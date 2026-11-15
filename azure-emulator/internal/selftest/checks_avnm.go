package selftest

import "net/url"

// Checks for Azure Virtual Network Manager.

type avnmEnv struct {
	tok, rg, nm, group, hub, s1, s2 string
}

// newAVNM builds a network manager with a hub, two spokes in a group.
func newAVNM(c *C) avnmEnv {
	tok := c.Boot("management")
	rg := newRG(c, tok)
	e := avnmEnv{tok: tok, rg: rg}
	e.hub = newVNet(c, tok, rg, "hub", "10.0.0.0/16")
	e.s1 = newVNet(c, tok, rg, "s1", "10.1.0.0/16")
	e.s2 = newVNet(c, tok, rg, "s2", "10.2.0.0/16")
	for _, v := range []struct{ id, cidr string }{{e.hub, "10.0.0.0/24"}, {e.s1, "10.1.0.0/24"}, {e.s2, "10.2.0.0/24"}} {
		c.Status(c.Do(put, c.ARM(v.id+"/subnets/default", apiNet), tok, map[string]any{"properties": map[string]any{"addressPrefix": v.cidr}}), 201, "", "add subnet "+v.cidr)
	}
	e.nm = rg + "/providers/Microsoft.Network/networkManagers/nm"
	c.Status(c.Do(put, c.ARM(e.nm, apiNet), tok, map[string]any{"location": "uksouth", "properties": map[string]any{
		"networkManagerScopes": map[string]any{"subscriptions": []any{"/subscriptions/" + SubSandbox}}, "networkManagerScopeAccesses": []any{"Connectivity", "SecurityAdmin"}}}), 201, "", "create network manager")
	e.group = e.nm + "/networkGroups/spokes"
	c.Status(c.Do(put, c.ARM(e.group, apiNet), tok, map[string]any{"properties": map[string]any{}}), 201, "", "create network group")
	for i, v := range []string{e.s1, e.s2} {
		c.Status(c.Do(put, c.ARM(e.group+"/staticMembers/m"+string(rune('1'+i)), apiNet), tok, map[string]any{"properties": map[string]any{"resourceId": v}}), 201, "", "add a spoke to the group")
	}
	return e
}

func (e avnmEnv) hubSpoke(c *C, connectivity string) string {
	id := e.nm + "/connectivityConfigurations/hub-spoke"
	c.Status(c.Do(put, c.ARM(id, apiNet), e.tok, map[string]any{"properties": map[string]any{"connectivityTopology": "HubAndSpoke",
		"hubs":            []any{map[string]any{"resourceId": e.hub, "resourceType": "Microsoft.Network/virtualNetworks"}},
		"appliesToGroups": []any{map[string]any{"networkGroupId": e.group, "groupConnectivity": connectivity}}}}), 201, "", "create hub-and-spoke configuration")
	return id
}

func (e avnmEnv) commit(c *C, typ string, ids ...string) {
	cfg := make([]any, len(ids))
	for i, id := range ids {
		cfg[i] = id
	}
	c.Status(c.Do(post, c.ARM(e.nm+"/commit", apiNet), e.tok, map[string]any{"targetLocations": []any{"uksouth"}, "configurationIds": cfg, "commitType": typ}), 202, "", "commit "+typ+" to uksouth")
}

// traceFrom traces from a host in the given VNet's "default" subnet.
func traceFrom(c *C, vnet, from, to, port string) string {
	u := c.URL("management", "/_cloudtyke/network/trace?from="+from+"&to="+to+"&port="+port+"&fromSubnet="+url.QueryEscape(vnet+"/subnets/default"))
	r := c.Status(c.Do(get, u, "", nil), 200, "", "trace "+from+" -> "+to+":"+port)
	return r.Str("result")
}

func init() {
	Checks["Microsoft.Network/networkManagers"] = Check{"scopes, scope accesses, commit to a region, deployment status", func(c *C) {
		tok := c.Boot("management")
		rg := newRG(c, tok)
		c.Status(c.Do(put, c.ARM(rg+"/providers/Microsoft.Network/networkManagers/bad", apiNet), tok, map[string]any{"location": "uksouth", "properties": map[string]any{}}), 400, "InvalidNetworkManagerScope", "network manager without a scope")
		e := newAVNM(c)
		cc := e.hubSpoke(c, "None")
		c.Status(c.Do(post, c.ARM(e.nm+"/commit", apiNet), e.tok, map[string]any{"targetLocations": []any{"uksouth"}, "configurationIds": []any{cc}, "commitType": "Routing"}), 400, "InvalidCommitType", "commit a type the manager is not scoped for")
		e.commit(c, "Connectivity", cc)
		st := c.Status(c.Do(post, c.ARM(e.nm+"/listDeploymentStatus", apiNet), e.tok, map[string]any{"regions": []any{"uksouth"}}), 200, "", "list deployment status").Value()
		c.Expect(len(st) == 1 && st[0]["deploymentStatus"] == "Deployed", "uksouth shows a Deployed connectivity deployment")
		c.Status(c.Do(del, c.ARM(e.nm, apiNet), e.tok, nil), 400, "CannotDeleteNetworkManagerWithActiveDeployment", "delete while deployed")
	}}
	Checks["Microsoft.Network/networkManagers/networkGroups"] = Check{"groups of VNets; in use by a configuration cannot be deleted", func(c *C) {
		e := newAVNM(c)
		e.hubSpoke(c, "None")
		c.Status(c.Do(del, c.ARM(e.group, apiNet), e.tok, nil), 400, "NetworkGroupInUse", "delete a group a configuration uses")
	}}
	Checks["Microsoft.Network/networkManagers/networkGroups/staticMembers"] = Check{"VNet must exist and be in scope", func(c *C) {
		e := newAVNM(c)
		c.Status(c.Do(put, c.ARM(e.group+"/staticMembers/ghost", apiNet), e.tok, map[string]any{"properties": map[string]any{"resourceId": e.rg + "/providers/Microsoft.Network/virtualNetworks/ghost"}}), 400, "InvalidResourceId", "member that does not exist")
		m := c.Status(c.Do(get, c.ARM(e.group+"/staticMembers/m1", apiNet), e.tok, nil), 200, "", "read member")
		c.Expect(m.Str("properties.region") == "uksouth", "member records its region")
	}}
	Checks["Microsoft.Network/networkManagers/connectivityConfigurations"] = Check{"hub-and-spoke peerings, connected group, nothing before commit", func(c *C) {
		e := newAVNM(c)
		cc := e.hubSpoke(c, "DirectlyConnected")
		c.Expect(traceFrom(c, e.s1, "10.1.0.4", "10.0.0.4", "443") == "Dropped", "spoke cannot reach the hub before a commit")
		e.commit(c, "Connectivity", cc)
		c.Expect(traceFrom(c, e.s1, "10.1.0.4", "10.0.0.4", "443") == "Delivered", "spoke reaches the hub through the ANM peering")
		c.Expect(traceFrom(c, e.s1, "10.1.0.4", "10.2.0.4", "443") == "Delivered", "spokes reach each other through the connected group")
		p := c.Status(c.Do(get, c.ARM(e.s1+"/virtualNetworkPeerings", apiNet), e.tok, nil), 200, "", "list spoke peerings").Value()
		c.Expect(len(p) == 1 && len(p[0]["name"].(string)) > 4 && p[0]["name"].(string)[:4] == "ANM_", "network manager created an ANM_ peering")
		e.commit(c, "Connectivity")
		c.Expect(traceFrom(c, e.s1, "10.1.0.4", "10.0.0.4", "443") == "Dropped", "an empty commit removes the connectivity")
	}}
	Checks["Microsoft.Network/networkManagers/securityAdminConfigurations"] = Check{"admin rules apply after commit, before NSGs", func(c *C) {
		e := newAVNM(c)
		e.commit(c, "Connectivity", e.hubSpoke(c, "DirectlyConnected"))
		sac := e.nm + "/securityAdminConfigurations/baseline"
		c.Status(c.Do(put, c.ARM(sac, apiNet), e.tok, map[string]any{"properties": map[string]any{}}), 201, "", "create security admin configuration")
		c.Status(c.Do(put, c.ARM(sac+"/ruleCollections/rc", apiNet), e.tok, map[string]any{"properties": map[string]any{"appliesToGroups": []any{map[string]any{"networkGroupId": e.group}}}}), 201, "", "create rule collection")
		c.Status(c.Do(put, c.ARM(sac+"/ruleCollections/rc/rules/deny-ssh", apiNet), e.tok, map[string]any{"kind": "Custom", "properties": map[string]any{"access": "Deny", "direction": "Inbound", "priority": 100,
			"protocol": "Tcp", "destinationPortRanges": []any{"22"}}}), 201, "", "deny SSH")
		c.Expect(traceFrom(c, e.s1, "10.1.0.4", "10.2.0.4", "22") == "Delivered", "rules do nothing before a commit")
		e.commit(c, "SecurityAdmin", sac)
		c.Expect(traceFrom(c, e.s1, "10.1.0.4", "10.2.0.4", "22") == "Blocked", "committed Deny rule blocks SSH")
		c.Status(c.Do(del, c.ARM(sac, apiNet), e.tok, nil), 400, "ConfigurationIsDeployed", "delete a deployed configuration")
	}}
	Checks["Microsoft.Network/networkManagers/securityAdminConfigurations/ruleCollections"] = Check{"collections must target groups of this manager", func(c *C) {
		e := newAVNM(c)
		sac := e.nm + "/securityAdminConfigurations/s"
		c.Status(c.Do(put, c.ARM(sac, apiNet), e.tok, map[string]any{"properties": map[string]any{}}), 201, "", "create configuration")
		c.Status(c.Do(put, c.ARM(sac+"/ruleCollections/empty", apiNet), e.tok, map[string]any{"properties": map[string]any{}}), 400, "InvalidRuleCollection", "collection without groups")
		c.Status(c.Do(put, c.ARM(sac+"/ruleCollections/rc", apiNet), e.tok, map[string]any{"properties": map[string]any{"appliesToGroups": []any{map[string]any{"networkGroupId": e.group}}}}), 201, "", "collection for the spokes group")
	}}
	Checks["Microsoft.Network/networkManagers/securityAdminConfigurations/ruleCollections/rules"] = Check{"access values, priority range and conflicts", func(c *C) {
		e := newAVNM(c)
		rc := e.nm + "/securityAdminConfigurations/s/ruleCollections/rc"
		c.Status(c.Do(put, c.ARM(e.nm+"/securityAdminConfigurations/s", apiNet), e.tok, map[string]any{"properties": map[string]any{}}), 201, "", "create configuration")
		c.Status(c.Do(put, c.ARM(rc, apiNet), e.tok, map[string]any{"properties": map[string]any{"appliesToGroups": []any{map[string]any{"networkGroupId": e.group}}}}), 201, "", "create collection")
		rule := func(access string, prio int) map[string]any {
			return map[string]any{"kind": "Custom", "properties": map[string]any{"access": access, "direction": "Inbound", "priority": prio, "protocol": "Tcp", "destinationPortRanges": []any{"3389"}}}
		}
		c.Status(c.Do(put, c.ARM(rc+"/rules/bad", apiNet), e.tok, rule("Maybe", 100)), 400, "InvalidAdminRule", "unknown access value")
		c.Status(c.Do(put, c.ARM(rc+"/rules/bad", apiNet), e.tok, rule("Deny", 5000)), 400, "InvalidAdminRule", "priority above 4096")
		c.Status(c.Do(put, c.ARM(rc+"/rules/deny-rdp", apiNet), e.tok, rule("Deny", 100)), 201, "", "create rule")
		c.Status(c.Do(put, c.ARM(rc+"/rules/also", apiNet), e.tok, rule("AlwaysAllow", 100)), 400, "AdminRuleConflict", "same priority and direction")
	}}
}
