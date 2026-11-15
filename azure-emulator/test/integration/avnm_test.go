package integration

import "testing"

// TestVirtualNetworkManager covers groups, hub-and-spoke and mesh
// connectivity, per-region commits and security admin rules.
func TestVirtualNetworkManager(t *testing.T) {
	e := start(t)
	arm := e.url("management", "")
	tok := e.token(bootID, bootSec, arm+"/")
	const api = "?api-version=2024-05-01"
	rg := "/subscriptions/" + sandbox + "/resourceGroups/rg-avnm"
	put := func(path string, body map[string]any) map[string]any {
		t.Helper()
		code, _, b := e.do("PUT", arm+path+api, tok, body)
		if code >= 300 {
			t.Fatalf("PUT %s: %d %v", path, code, b)
		}
		return b
	}
	post := func(path string, body map[string]any) (int, map[string]any) {
		t.Helper()
		code, _, b := e.do("POST", arm+path+api, tok, body)
		return code, b
	}
	trace := func(from, to, port, want string) {
		t.Helper()
		code, _, b := e.do("GET", e.url("management", "/_cloudtyke/network/trace?from="+from+"&to="+to+"&port="+port), "", nil)
		if code != 200 || b["result"] != want {
			t.Fatalf("%s -> %s:%s want %s, got %v: %v", from, to, port, want, b["result"], b["summary"])
		}
	}
	vnet := func(name, loc, cidr, sub string) string {
		id := rg + "/providers/Microsoft.Network/virtualNetworks/" + name
		put(id, map[string]any{"location": loc, "properties": map[string]any{"addressSpace": map[string]any{"addressPrefixes": []any{cidr}},
			"subnets": []any{map[string]any{"name": "snet", "properties": map[string]any{"addressPrefix": sub}}}}})
		return id
	}
	put(rg, map[string]any{"location": "uksouth"})
	hub := vnet("hub", "uksouth", "10.0.0.0/16", "10.0.1.0/24")
	s1 := vnet("s1", "uksouth", "10.1.0.0/16", "10.1.1.0/24")
	s2 := vnet("s2", "uksouth", "10.2.0.0/16", "10.2.1.0/24")
	w1 := vnet("w1", "westeurope", "10.101.0.0/16", "10.101.1.0/24")

	nm := rg + "/providers/Microsoft.Network/networkManagers/nm"
	put(nm, map[string]any{"location": "uksouth", "properties": map[string]any{"networkManagerScopes": map[string]any{"subscriptions": []any{"/subscriptions/" + sandbox}},
		"networkManagerScopeAccesses": []any{"Connectivity", "SecurityAdmin"}}})
	ng := nm + "/networkGroups/spokes"
	put(ng, map[string]any{"properties": map[string]any{}})
	for i, v := range []string{s1, s2, w1} {
		put(ng+"/staticMembers/m"+string(rune('1'+i)), map[string]any{"properties": map[string]any{"resourceId": v}})
	}
	cc := nm + "/connectivityConfigurations/hubspoke"
	put(cc, map[string]any{"properties": map[string]any{"connectivityTopology": "HubAndSpoke", "hubs": []any{map[string]any{"resourceId": hub, "resourceType": "Microsoft.Network/virtualNetworks"}},
		"appliesToGroups": []any{map[string]any{"networkGroupId": ng, "groupConnectivity": "DirectlyConnected"}}}})

	// nothing happens before a commit
	trace("10.1.1.4", "10.0.1.4", "443", "Dropped")
	if code, b := post(nm+"/commit", map[string]any{"targetLocations": []any{"uksouth"}, "configurationIds": []any{cc}, "commitType": "Connectivity"}); code != 202 {
		t.Fatalf("commit: %d %v", code, b)
	}
	trace("10.1.1.4", "10.0.1.4", "443", "Delivered") // ANM peering to the hub
	trace("10.1.1.4", "10.2.1.4", "443", "Delivered") // connected group in the region
	nsg := rg + "/providers/Microsoft.Network/networkSecurityGroups/nsg-s1"
	put(nsg, map[string]any{"location": "uksouth"})
	put(s1+"/subnets/snet", map[string]any{"properties": map[string]any{"addressPrefix": "10.1.1.0/24", "networkSecurityGroup": map[string]any{"id": nsg}}})
	trace("10.1.1.4", "10.2.1.4", "443", "Delivered") // default AllowVnetOutBound covers the connected group
	trace("10.1.1.4", "10.101.1.4", "443", "Dropped") // westeurope not deployed
	trace("10.101.1.4", "10.0.1.4", "443", "Dropped")

	code, st := post(nm+"/listDeploymentStatus", map[string]any{})
	if code != 200 || len(st["value"].([]any)) != 1 {
		t.Fatalf("deployment status: %d %v", code, st)
	}

	// security admin: deny SSH everywhere in the group
	sac := nm + "/securityAdminConfigurations/baseline"
	put(sac, map[string]any{"properties": map[string]any{}})
	put(sac+"/ruleCollections/rc", map[string]any{"properties": map[string]any{"appliesToGroups": []any{map[string]any{"networkGroupId": ng}}}})
	put(sac+"/ruleCollections/rc/rules/deny-ssh", map[string]any{"kind": "Custom", "properties": map[string]any{"access": "Deny", "direction": "Inbound", "priority": 100,
		"protocol": "Tcp", "destinationPortRanges": []any{"22"}, "sources": []any{map[string]any{"addressPrefix": "*", "addressPrefixType": "IPPrefix"}}}})
	trace("10.1.1.4", "10.2.1.4", "22", "Delivered")
	if code, b := post(nm+"/commit", map[string]any{"targetLocations": []any{"uksouth"}, "configurationIds": []any{sac}, "commitType": "SecurityAdmin"}); code != 202 {
		t.Fatalf("commit security: %d %v", code, b)
	}
	trace("10.1.1.4", "10.2.1.4", "22", "Blocked")
	trace("10.1.1.4", "10.2.1.4", "443", "Delivered")

	// editing a deployed configuration changes nothing until the next commit
	put(cc, map[string]any{"properties": map[string]any{"connectivityTopology": "HubAndSpoke", "hubs": []any{map[string]any{"resourceId": hub, "resourceType": "Microsoft.Network/virtualNetworks"}},
		"appliesToGroups": []any{map[string]any{"networkGroupId": ng, "groupConnectivity": "None"}}}})
	trace("10.1.1.4", "10.2.1.4", "443", "Delivered")
	post(nm+"/commit", map[string]any{"targetLocations": []any{"uksouth"}, "configurationIds": []any{cc}, "commitType": "Connectivity"})
	trace("10.1.1.4", "10.2.1.4", "443", "Dropped")

	// uncommit connectivity: peerings go away
	post(nm+"/commit", map[string]any{"targetLocations": []any{"uksouth"}, "configurationIds": []any{}, "commitType": "Connectivity"})
	trace("10.1.1.4", "10.0.1.4", "443", "Dropped")
	_ = w1
}
