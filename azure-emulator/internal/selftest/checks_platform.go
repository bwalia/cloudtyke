package selftest

import (
	"encoding/base64"
	"strings"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
)

func entraGUID() string { return entra.NewGUID() }

// Checks for the phase 2 and 4 providers: networking, managed identities,
// Log Analytics, Container Registry and AKS.

const (
	apiNet = "2024-05-01"
	apiDNS = "2020-06-01"
	apiMI  = "2023-01-31"
	apiOI  = "2023-09-01"
	apiACR = "2023-07-01"
	apiAKS = "2025-02-01"
)

// newRG creates a sandbox resource group and returns its id.
func newRG(c *C, tok string) string {
	id := "/subscriptions/" + SubSandbox + "/resourceGroups/" + c.Name("rg")
	c.Status(c.Do(put, c.ARM(id, apiRes), tok, map[string]any{"location": "uksouth"}), 201, "", "create a resource group")
	return id
}

func newVNet(c *C, tok, rg, name, cidr string) string {
	id := rg + "/providers/Microsoft.Network/virtualNetworks/" + name
	c.Status(c.Do(put, c.ARM(id, apiNet), tok, map[string]any{"location": "uksouth", "properties": map[string]any{"addressSpace": map[string]any{"addressPrefixes": []any{cidr}}}}), 201, "", "create VNet "+name+" "+cidr)
	return id
}

func newZone(c *C, tok, rg string) string {
	id := rg + "/providers/Microsoft.Network/privateDnsZones/" + c.Name("z") + ".internal"
	c.Status(c.Do(put, c.ARM(id, apiDNS), tok, map[string]any{"location": "global"}), 201, "", "create private DNS zone")
	return id
}

func newUAI(c *C, tok, rg, name string) Resp {
	return c.Status(c.Do(put, c.ARM(rg+"/providers/Microsoft.ManagedIdentity/userAssignedIdentities/"+name, apiMI), tok, map[string]any{"location": "uksouth"}), 201, "", "create user-assigned identity "+name)
}

func recordCheck(rt string, body map[string]any, field string) Check {
	return Check{rt + " record lifecycle with fqdn and TTL", func(c *C) {
		tok := c.Boot("management")
		zone := newZone(c, tok, newRG(c, tok))
		name := "@"
		if rt != "SOA" {
			name = "www"
		}
		u := c.ARM(zone+"/"+rt+"/"+name, apiDNS)
		r := c.StatusIn(c.Do(put, u, tok, map[string]any{"properties": body}), "write "+rt+" record", 200, 201)
		c.Expect(strings.HasSuffix(r.Str("properties.fqdn"), ".internal."), "record has a fully qualified name, got %s", r.Str("properties.fqdn"))
		g := c.Status(c.Do(get, u, tok, nil), 200, "", "read "+rt+" record")
		c.Expect(g.Raw != nil && strings.Contains(string(g.Raw), field), "record keeps its %s data", field)
		if rt != "SOA" {
			c.Status(c.Do(del, u, tok, nil), 200, "", "delete record")
			c.Status(c.Do(get, u, tok, nil), 404, "ResourceNotFound", "record is gone")
		}
	}}
}

func init() {
	// ------------------------------------------------------------ networking
	Checks["Microsoft.Network/virtualNetworks"] = Check{"address space validation, inline subnets, list across resource groups", func(c *C) {
		tok := c.Boot("management")
		rg := newRG(c, tok)
		bad := rg + "/providers/Microsoft.Network/virtualNetworks/bad"
		c.Status(c.Do(put, c.ARM(bad, apiNet), tok, map[string]any{"location": "uksouth", "properties": map[string]any{"addressSpace": map[string]any{"addressPrefixes": []any{"10.0.0.1/16"}}}}), 400, "InvalidCIDRNotation", "host bits in the address space")
		id := rg + "/providers/Microsoft.Network/virtualNetworks/vnet-a"
		v := c.Status(c.Do(put, c.ARM(id, apiNet), tok, map[string]any{"location": "uksouth", "properties": map[string]any{
			"addressSpace": map[string]any{"addressPrefixes": []any{"10.1.0.0/16"}},
			"subnets":      []any{map[string]any{"name": "app", "properties": map[string]any{"addressPrefix": "10.1.1.0/24"}}},
		}}), 201, "", "create VNet with an inline subnet")
		c.Expect(len(v.Body["properties"].(map[string]any)["subnets"].([]any)) == 1, "VNet returns its subnet inline")
		c.Status(c.Do(get, c.ARM(id+"/subnets/app", apiNet), tok, nil), 200, "", "inline subnet is also a child resource")
		l := c.Status(c.Do(get, c.ARM("/subscriptions/"+SubSandbox+"/providers/Microsoft.Network/virtualNetworks", apiNet), tok, nil), 200, "", "list VNets in the subscription").Value()
		c.Expect(HasID(l, "name", "vnet-a"), "subscription-wide list includes the VNet")
		c.Status(c.Do(put, c.ARM(id, apiNet), tok, map[string]any{"location": "uksouth", "properties": map[string]any{"addressSpace": map[string]any{"addressPrefixes": []any{"10.9.0.0/16"}}}}), 400, "NetcfgSubnetRangeOutsideVnet", "shrinking the space below a subnet")
		c.Status(c.Do(del, c.ARM(id, apiNet), tok, nil), 202, "", "delete VNet")
	}}
	Checks["Microsoft.Network/virtualNetworks/subnets"] = Check{"CIDR inside the VNet, no overlaps, NSG association blocks NSG delete", func(c *C) {
		tok := c.Boot("management")
		rg := newRG(c, tok)
		vnet := newVNet(c, tok, rg, "vnet-s", "10.2.0.0/16")
		sn := func(name, cidr string, extra map[string]any) Resp {
			p := map[string]any{"addressPrefix": cidr}
			for k, v := range extra {
				p[k] = v
			}
			return c.Do(put, c.ARM(vnet+"/subnets/"+name, apiNet), tok, map[string]any{"properties": p})
		}
		c.Status(sn("outside", "10.3.0.0/24", nil), 400, "NetcfgSubnetRangeOutsideVnet", "subnet outside the VNet")
		c.Status(sn("a", "10.2.1.0/24", nil), 201, "", "create subnet a")
		c.Status(sn("b", "10.2.1.128/25", nil), 400, "NetcfgSubnetRangesOverlap", "overlapping subnet")
		nsg := rg + "/providers/Microsoft.Network/networkSecurityGroups/nsg-s"
		c.Status(c.Do(put, c.ARM(nsg, apiNet), tok, map[string]any{"location": "uksouth"}), 201, "", "create NSG")
		a := c.Status(sn("a", "10.2.1.0/24", map[string]any{"networkSecurityGroup": map[string]any{"id": nsg}}), 200, "", "associate the NSG")
		c.Expect(strings.EqualFold(a.Str("properties.networkSecurityGroup.id"), nsg), "subnet references the NSG")
		c.Status(c.Do(del, c.ARM(nsg, apiNet), tok, nil), 400, "InUseNetworkSecurityGroupCannotBeDeleted", "NSG in use cannot be deleted")
		c.Status(c.Do(del, c.ARM(vnet+"/subnets/a", apiNet), tok, nil), 202, "", "delete subnet")
		c.Status(c.Do(del, c.ARM(nsg, apiNet), tok, nil), 202, "", "NSG can be deleted once free")
	}}
	Checks["Microsoft.Network/virtualNetworks/virtualNetworkPeerings"] = Check{"Initiated until peered back, then Connected; overlap rejected", func(c *C) {
		tok := c.Boot("management")
		rg := newRG(c, tok)
		hub := newVNet(c, tok, rg, "hub", "10.0.0.0/16")
		spoke := newVNet(c, tok, rg, "spoke", "10.10.0.0/16")
		clash := newVNet(c, tok, rg, "clash", "10.0.128.0/17")
		peer := func(from, to, name string) Resp {
			return c.Do(put, c.ARM(from+"/virtualNetworkPeerings/"+name, apiNet), tok, map[string]any{"properties": map[string]any{"remoteVirtualNetwork": map[string]any{"id": to}, "allowForwardedTraffic": true}})
		}
		c.Status(peer(hub, clash, "to-clash"), 400, "VnetAddressSpacesOverlap", "peering overlapping VNets")
		p := c.Status(peer(spoke, hub, "to-hub"), 201, "", "peer spoke to hub")
		c.Expect(p.Str("properties.peeringState") == "Initiated", "one-sided peering is Initiated, got %s", p.Str("properties.peeringState"))
		c.Status(peer(hub, spoke, "to-spoke"), 201, "", "peer hub back to spoke")
		g := c.Status(c.Do(get, c.ARM(spoke+"/virtualNetworkPeerings/to-hub", apiNet), tok, nil), 200, "", "read spoke peering")
		c.Expect(g.Str("properties.peeringState") == "Connected", "both sides peered: Connected, got %s", g.Str("properties.peeringState"))
		c.Status(c.Do(del, c.ARM(hub, apiNet), tok, nil), 202, "", "delete the hub")
		d := c.Status(c.Do(get, c.ARM(spoke+"/virtualNetworkPeerings/to-hub", apiNet), tok, nil), 200, "", "read spoke peering")
		c.Expect(d.Str("properties.peeringState") == "Disconnected", "remote gone: Disconnected, got %s", d.Str("properties.peeringState"))
	}}
	Checks["Microsoft.Network/networkSecurityGroups"] = Check{"inline rules, six default rules, priority conflicts", func(c *C) {
		tok := c.Boot("management")
		rg := newRG(c, tok)
		id := rg + "/providers/Microsoft.Network/networkSecurityGroups/nsg-web"
		rule := func(name string, prio int) map[string]any {
			return map[string]any{"name": name, "properties": map[string]any{"priority": prio, "direction": "Inbound", "access": "Allow", "protocol": "Tcp",
				"sourcePortRange": "*", "destinationPortRange": "443", "sourceAddressPrefix": "Internet", "destinationAddressPrefix": "*"}}
		}
		c.Status(c.Do(put, c.ARM(id, apiNet), tok, map[string]any{"location": "uksouth", "properties": map[string]any{"securityRules": []any{rule("a", 100), rule("b", 100)}}}), 400, "SecurityRuleConflict", "two rules with the same priority")
		n := c.Status(c.Do(put, c.ARM(id, apiNet), tok, map[string]any{"location": "uksouth", "properties": map[string]any{"securityRules": []any{rule("https", 100)}}}), 201, "", "create NSG with a rule")
		p := n.Body["properties"].(map[string]any)
		c.Expect(len(p["defaultSecurityRules"].([]any)) == 6, "NSG has the six default rules")
		c.Expect(len(p["securityRules"].([]any)) == 1, "custom rule returned inline")
	}}
	Checks["Microsoft.Network/networkSecurityGroups/securityRules"] = Check{"child rules: priority range and direction", func(c *C) {
		tok := c.Boot("management")
		rg := newRG(c, tok)
		nsg := rg + "/providers/Microsoft.Network/networkSecurityGroups/nsg-r"
		c.Status(c.Do(put, c.ARM(nsg, apiNet), tok, map[string]any{"location": "uksouth"}), 201, "", "create NSG")
		body := func(prio int) map[string]any {
			return map[string]any{"properties": map[string]any{"priority": prio, "direction": "Outbound", "access": "Deny", "protocol": "*", "sourcePortRange": "*", "destinationPortRange": "*", "sourceAddressPrefix": "*", "destinationAddressPrefix": "Internet"}}
		}
		c.Status(c.Do(put, c.ARM(nsg+"/securityRules/deny", apiNet), tok, body(5000)), 400, "SecurityRuleInvalidPriority", "priority above 4096")
		c.Status(c.Do(put, c.ARM(nsg+"/securityRules/deny", apiNet), tok, body(4000)), 201, "", "create rule")
		g := c.Status(c.Do(get, c.ARM(nsg, apiNet), tok, nil), 200, "", "read NSG")
		c.Expect(strings.Contains(string(g.Raw), `"deny"`), "child rule appears in the NSG")
	}}
	Checks["Microsoft.Network/routeTables"] = Check{"routes inline, association blocks delete", func(c *C) {
		tok := c.Boot("management")
		rg := newRG(c, tok)
		rt := rg + "/providers/Microsoft.Network/routeTables/rt"
		c.Status(c.Do(put, c.ARM(rt, apiNet), tok, map[string]any{"location": "uksouth", "properties": map[string]any{"disableBgpRoutePropagation": true,
			"routes": []any{map[string]any{"name": "default", "properties": map[string]any{"addressPrefix": "0.0.0.0/0", "nextHopType": "VirtualAppliance", "nextHopIpAddress": "10.0.0.4"}}}}}), 201, "", "create route table")
		vnet := newVNet(c, tok, rg, "v", "10.5.0.0/16")
		c.Status(c.Do(put, c.ARM(vnet+"/subnets/app", apiNet), tok, map[string]any{"properties": map[string]any{"addressPrefix": "10.5.1.0/24", "routeTable": map[string]any{"id": rt}}}), 201, "", "associate with a subnet")
		g := c.Status(c.Do(get, c.ARM(rt, apiNet), tok, nil), 200, "", "read route table")
		c.Expect(strings.Contains(strings.ToLower(string(g.Raw)), "/subnets/app"), "route table lists the associated subnet")
		c.Status(c.Do(del, c.ARM(rt, apiNet), tok, nil), 400, "InUseRouteTableCannotBeDeleted", "delete while associated")
	}}
	Checks["Microsoft.Network/routeTables/routes"] = Check{"VirtualAppliance needs a next hop IP", func(c *C) {
		tok := c.Boot("management")
		rg := newRG(c, tok)
		rt := rg + "/providers/Microsoft.Network/routeTables/rt2"
		c.Status(c.Do(put, c.ARM(rt, apiNet), tok, map[string]any{"location": "uksouth"}), 201, "", "create route table")
		c.Status(c.Do(put, c.ARM(rt+"/routes/fw", apiNet), tok, map[string]any{"properties": map[string]any{"addressPrefix": "0.0.0.0/0", "nextHopType": "VirtualAppliance"}}), 400, "InvalidRequestFormat", "missing nextHopIpAddress")
		c.Status(c.Do(put, c.ARM(rt+"/routes/fw", apiNet), tok, map[string]any{"properties": map[string]any{"addressPrefix": "0.0.0.0/0", "nextHopType": "VirtualAppliance", "nextHopIpAddress": "10.0.0.4"}}), 201, "", "create route")
	}}
	Checks["Microsoft.Network/publicIPAddresses"] = Check{"Standard must be Static; stable address", func(c *C) {
		tok := c.Boot("management")
		rg := newRG(c, tok)
		id := rg + "/providers/Microsoft.Network/publicIPAddresses/pip"
		c.Status(c.Do(put, c.ARM(id, apiNet), tok, map[string]any{"location": "uksouth", "sku": map[string]any{"name": "Standard"}, "properties": map[string]any{"publicIPAllocationMethod": "Dynamic"}}), 400, "StandardSkuPublicIPAddressesCanNotBeDynamic", "Standard + Dynamic")
		p := c.Status(c.Do(put, c.ARM(id, apiNet), tok, map[string]any{"location": "uksouth", "sku": map[string]any{"name": "Standard"}, "properties": map[string]any{"publicIPAllocationMethod": "Static"}}), 201, "", "create static IP")
		ip := p.Str("properties.ipAddress")
		c.Expect(ip != "" && strings.Count(ip, ".") == 3, "static IP gets an address, got %s", ip)
		again := c.Status(c.Do(put, c.ARM(id, apiNet), tok, map[string]any{"location": "uksouth", "sku": map[string]any{"name": "Standard"}, "properties": map[string]any{"publicIPAllocationMethod": "Static"}}), 200, "", "update")
		c.Expect(again.Str("properties.ipAddress") == ip, "address is stable across updates")
	}}
	Checks["Microsoft.Network/privateDnsZones"] = Check{"global zone, automatic SOA, link counters, delete blocked by links", func(c *C) {
		tok := c.Boot("management")
		rg := newRG(c, tok)
		c.Status(c.Do(put, c.ARM(rg+"/providers/Microsoft.Network/privateDnsZones/x.internal", apiDNS), tok, map[string]any{"location": "uksouth"}), 400, "InvalidLocation", "zone outside global")
		zone := newZone(c, tok, rg)
		c.Status(c.Do(get, c.ARM(zone+"/SOA/@", apiDNS), tok, nil), 200, "", "SOA record exists")
		vnet := newVNet(c, tok, rg, "v", "10.6.0.0/16")
		c.Status(c.Do(put, c.ARM(zone+"/virtualNetworkLinks/l", apiDNS), tok, map[string]any{"location": "global", "properties": map[string]any{"virtualNetwork": map[string]any{"id": vnet}}}), 201, "", "link a VNet")
		z := c.Status(c.Do(get, c.ARM(zone, apiDNS), tok, nil), 200, "", "read zone")
		c.Expect(dig(z.Body, "properties.numberOfVirtualNetworkLinks") == float64(1), "zone counts its links")
		c.Status(c.Do(del, c.ARM(zone, apiDNS), tok, nil), 409, "CannotDeleteResource", "delete zone with links")
	}}
	Checks["Microsoft.Network/privateDnsZones/virtualNetworkLinks"] = Check{"VNet must exist, one link per VNet", func(c *C) {
		tok := c.Boot("management")
		rg := newRG(c, tok)
		zone := newZone(c, tok, rg)
		c.Status(c.Do(put, c.ARM(zone+"/virtualNetworkLinks/l", apiDNS), tok, map[string]any{"location": "global", "properties": map[string]any{"virtualNetwork": map[string]any{"id": rg + "/providers/Microsoft.Network/virtualNetworks/nope"}}}), 400, "BadRequest", "link to a missing VNet")
		vnet := newVNet(c, tok, rg, "v", "10.7.0.0/16")
		l := c.Status(c.Do(put, c.ARM(zone+"/virtualNetworkLinks/l1", apiDNS), tok, map[string]any{"location": "global", "properties": map[string]any{"virtualNetwork": map[string]any{"id": vnet}, "registrationEnabled": true}}), 201, "", "link")
		c.Expect(l.Str("properties.virtualNetworkLinkState") == "Completed", "link state is Completed")
		c.Status(c.Do(put, c.ARM(zone+"/virtualNetworkLinks/l2", apiDNS), tok, map[string]any{"location": "global", "properties": map[string]any{"virtualNetwork": map[string]any{"id": vnet}}}), 409, "Conflict", "second link to the same VNet")
	}}
	Checks["Microsoft.Network/privateDnsZones/A"] = recordCheck("A", map[string]any{"ttl": 300, "aRecords": []any{map[string]any{"ipv4Address": "10.0.3.10"}}}, "10.0.3.10")
	Checks["Microsoft.Network/privateDnsZones/AAAA"] = recordCheck("AAAA", map[string]any{"ttl": 300, "aaaaRecords": []any{map[string]any{"ipv6Address": "fd00::10"}}}, "fd00::10")
	Checks["Microsoft.Network/privateDnsZones/CNAME"] = recordCheck("CNAME", map[string]any{"ttl": 300, "cnameRecord": map[string]any{"cname": "app.internal"}}, "app.internal")
	Checks["Microsoft.Network/privateDnsZones/TXT"] = recordCheck("TXT", map[string]any{"ttl": 300, "txtRecords": []any{map[string]any{"value": []any{"v=cloudtyke"}}}}, "v=cloudtyke")
	Checks["Microsoft.Network/privateDnsZones/SOA"] = recordCheck("SOA", map[string]any{"ttl": 3600, "soaRecord": map[string]any{"email": "dns-admin.contoso.com", "refreshTime": 1800}}, "dns-admin")

	// ------------------------------------------------------------ managed identities
	Checks["Microsoft.ManagedIdentity/userAssignedIdentities"] = Check{"backed by a service principal that can be granted roles", func(c *C) {
		tok := c.Boot("management")
		rg := newRG(c, tok)
		u := newUAI(c, tok, rg, "id-app")
		pid, cid := u.Str("properties.principalId"), u.Str("properties.clientId")
		c.Expect(pid != "" && cid != "", "identity has principalId and clientId")
		again := c.Status(c.Do(put, c.ARM(rg+"/providers/Microsoft.ManagedIdentity/userAssignedIdentities/id-app", apiMI), tok, map[string]any{"location": "uksouth", "tags": map[string]any{"a": "b"}}), 200, "", "update")
		c.Expect(again.Str("properties.principalId") == pid, "principalId is stable")
		sp := c.Status(c.Do(get, c.Graph("/servicePrincipals/"+pid), c.Boot("graph"), nil), 200, "", "service principal in Graph")
		c.Expect(sp.Str("servicePrincipalType") == "ManagedIdentity", "service principal type is ManagedIdentity")
		c.Status(c.Do(put, c.ARM(rg+"/providers/Microsoft.Authorization/roleAssignments/"+entraGUID(), apiAuth), tok, map[string]any{"properties": map[string]any{
			"roleDefinitionId": "/subscriptions/" + SubSandbox + "/providers/Microsoft.Authorization/roleDefinitions/" + RoleReader, "principalId": pid, "principalType": "ServicePrincipal"}}), 201, "", "grant Reader to the identity")
		c.Status(c.Do(del, c.ARM(rg+"/providers/Microsoft.ManagedIdentity/userAssignedIdentities/id-app", apiMI), tok, nil), 200, "", "delete identity")
		c.Status(c.Do(get, c.Graph("/servicePrincipals/"+pid), c.Boot("graph"), nil), 404, "Request_ResourceNotFound", "service principal removed with it")
	}}
	Checks["Microsoft.ManagedIdentity/userAssignedIdentities/federatedIdentityCredentials"] = Check{"https issuer, unique issuer+subject", func(c *C) {
		tok := c.Boot("management")
		rg := newRG(c, tok)
		newUAI(c, tok, rg, "id-wi")
		base := rg + "/providers/Microsoft.ManagedIdentity/userAssignedIdentities/id-wi/federatedIdentityCredentials/"
		fic := func(issuer string) map[string]any {
			return map[string]any{"properties": map[string]any{"issuer": issuer, "subject": "system:serviceaccount:orders:api", "audiences": []any{"api://AzureADTokenExchange"}}}
		}
		c.Status(c.Do(put, c.ARM(base+"bad", apiMI), tok, fic("http://insecure.example")), 400, "BadRequest", "plain-http issuer")
		c.Status(c.Do(put, c.ARM(base+"aks", apiMI), tok, fic("https://oidc.example/issuer/")), 201, "", "create federated credential")
		c.Status(c.Do(put, c.ARM(base+"dupe", apiMI), tok, fic("https://oidc.example/issuer")), 409, "Conflict", "same issuer and subject twice")
	}}

	// ------------------------------------------------------------ Log Analytics
	Checks["Microsoft.OperationalInsights/workspaces"] = Check{"customerId, retention bounds, shared keys", func(c *C) {
		tok := c.Boot("management")
		rg := newRG(c, tok)
		id := rg + "/providers/Microsoft.OperationalInsights/workspaces/" + c.Name("log")
		c.Status(c.Do(put, c.ARM(id, apiOI), tok, map[string]any{"location": "uksouth", "properties": map[string]any{"retentionInDays": 7}}), 400, "InvalidParameter", "retention below 30 days")
		w := c.Status(c.Do(put, c.ARM(id, apiOI), tok, map[string]any{"location": "uksouth", "properties": map[string]any{"sku": map[string]any{"name": "PerGB2018"}, "retentionInDays": 90}}), 201, "", "create workspace")
		c.Expect(w.Str("properties.customerId") != "", "workspace has a customerId")
		k := c.Status(c.Do(post, c.ARM(id+"/sharedKeys", apiOI), tok, nil), 200, "", "get shared keys")
		c.Expect(k.Str("primarySharedKey") != "", "primary shared key returned")
		g := c.Status(c.Do(get, c.ARM(id, apiOI), tok, nil), 200, "", "read workspace")
		c.Expect(!strings.Contains(string(g.Raw), k.Str("primarySharedKey")), "keys are never returned by GET")
	}}
	Checks["Microsoft.OperationalInsights/deletedWorkspaces"] = Check{"soft-deleted workspace list", func(c *C) {
		l := c.Status(c.Do(get, c.ARM("/subscriptions/"+SubSandbox+"/providers/Microsoft.OperationalInsights/deletedWorkspaces", apiOI), c.Boot("management"), nil), 200, "", "list deleted workspaces")
		c.Expect(l.Body["value"] != nil, "returns a value list")
	}}

	// ------------------------------------------------------------ Container Registry
	acr := func(c *C, tok, rg, sku string) (string, string) {
		name := strings.ReplaceAll(c.Name("acr"), "-", "")
		id := rg + "/providers/Microsoft.ContainerRegistry/registries/" + name
		c.Status(c.Do(put, c.ARM(id, apiACR), tok, map[string]any{"location": "uksouth", "sku": map[string]any{"name": sku}, "properties": map[string]any{"adminUserEnabled": true}}), 201, "", "create "+sku+" registry")
		return id, name
	}
	Checks["Microsoft.ContainerRegistry/registries"] = Check{"names, SKU rules, login server, admin credentials", func(c *C) {
		tok := c.Boot("management")
		rg := newRG(c, tok)
		c.Status(c.Do(put, c.ARM(rg+"/providers/Microsoft.ContainerRegistry/registries/bad-name", apiACR), tok, map[string]any{"location": "uksouth", "sku": map[string]any{"name": "Basic"}}), 400, "InvalidRegistryName", "name with a hyphen")
		c.Status(c.Do(put, c.ARM(rg+"/providers/Microsoft.ContainerRegistry/registries/"+strings.ReplaceAll(c.Name("acr"), "-", ""), apiACR), tok, map[string]any{"location": "uksouth", "sku": map[string]any{"name": "Basic"}, "properties": map[string]any{"publicNetworkAccess": "Disabled"}}), 400, "PublicNetworkAccessNotSupported", "private registry on Basic")
		id, name := acr(c, tok, rg, "Standard")
		g := c.Status(c.Do(get, c.ARM(id, apiACR), tok, nil), 200, "", "read registry")
		c.Expect(strings.HasPrefix(g.Str("properties.loginServer"), strings.ToLower(name)+".azurecr."), "login server is <name>.azurecr.<domain>, got %s", g.Str("properties.loginServer"))
		cr := c.Status(c.Do(post, c.ARM(id+"/listCredentials", apiACR), tok, nil), 200, "", "list admin credentials")
		c.Expect(cr.Str("username") == name, "admin user name is the registry name")
	}}
	Checks["Microsoft.ContainerRegistry/checkNameAvailability"] = Check{"registry names are global", func(c *C) {
		tok := c.Boot("management")
		_, name := acr(c, tok, newRG(c, tok), "Basic")
		u := c.ARM("/subscriptions/"+SubSandbox+"/providers/Microsoft.ContainerRegistry/checkNameAvailability", apiACR)
		r := c.Status(c.Do(post, u, tok, map[string]any{"name": name, "type": "Microsoft.ContainerRegistry/registries"}), 200, "", "check a taken name")
		c.Expect(r.Body["nameAvailable"] == false && r.Str("reason") == "AlreadyExists", "taken name is not available")
		f := c.Status(c.Do(post, u, tok, map[string]any{"name": "freeregistry12345", "type": "Microsoft.ContainerRegistry/registries"}), 200, "", "check a free name")
		c.Expect(f.Body["nameAvailable"] == true, "free name is available")
	}}
	Checks["Microsoft.ContainerRegistry/registries/replications"] = Check{"geo-replication is Premium-only", func(c *C) {
		tok := c.Boot("management")
		rg := newRG(c, tok)
		std, _ := acr(c, tok, rg, "Standard")
		c.Status(c.Do(put, c.ARM(std+"/replications/westeurope", apiACR), tok, map[string]any{"location": "westeurope"}), 400, "ReplicationNotSupported", "replicate a Standard registry")
		prem, _ := acr(c, tok, rg, "Premium")
		c.Status(c.Do(put, c.ARM(prem+"/replications/westeurope", apiACR), tok, map[string]any{"location": "westeurope"}), 201, "", "replicate a Premium registry")
	}}
	for _, child := range []string{"webhooks", "scopeMaps", "tokens"} {
		child := child
		Checks["Microsoft.ContainerRegistry/registries/"+child] = Check{child + " lifecycle", func(c *C) {
			tok := c.Boot("management")
			id, _ := acr(c, tok, newRG(c, tok), "Premium")
			body := map[string]any{"properties": map[string]any{}}
			if child == "webhooks" {
				body["location"] = "uksouth"
				body["properties"] = map[string]any{"serviceUri": "https://hooks.example/acr", "actions": []any{"push"}}
			}
			c.Status(c.Do(put, c.ARM(id+"/"+child+"/one", apiACR), tok, body), 201, "", "create "+child)
			l := c.Status(c.Do(get, c.ARM(id+"/"+child, apiACR), tok, nil), 200, "", "list "+child).Value()
			c.Expect(len(l) == 1, "lists the new item")
		}}
	}

	// ------------------------------------------------------------ AKS
	Checks["Microsoft.ContainerService/managedClusters"] = Check{"create, node resource group, kubelet identity, credentials, kubectl, upgrade rules, delete cleanup", func(c *C) {
		tok := c.Boot("management")
		rg := newRG(c, tok)
		id := rg + "/providers/Microsoft.ContainerService/managedClusters/aks-st"
		cluster := func(version string) map[string]any {
			return map[string]any{"location": "uksouth", "identity": map[string]any{"type": "SystemAssigned"}, "properties": map[string]any{
				"kubernetesVersion": version, "dnsPrefix": "aksst", "networkProfile": map[string]any{"networkPlugin": "kubenet"},
				"agentPoolProfiles": []any{map[string]any{"name": "system", "count": 2, "vmSize": "Standard_D4s_v5", "mode": "System"}}}}
		}
		c.Status(c.Do(put, c.ARM(id, apiAKS), tok, cluster("1.20")), 400, "InvalidParameter", "unsupported Kubernetes version")
		k := c.Status(c.Do(put, c.ARM(id, apiAKS), tok, cluster("1.33")), 201, "", "create cluster")
		c.Expect(strings.HasPrefix(k.Str("properties.currentKubernetesVersion"), "1.33."), "minor version resolves to a patch, got %s", k.Str("properties.currentKubernetesVersion"))
		nodeRG := k.Str("properties.nodeResourceGroup")
		c.Expect(strings.HasPrefix(nodeRG, "MC_"), "node resource group is MC_*, got %s", nodeRG)
		c.Status(c.Do(get, c.ARM("/subscriptions/"+SubSandbox+"/resourceGroups/"+nodeRG, apiRes), tok, nil), 200, "", "node resource group exists")
		g := c.Status(c.Do(get, c.ARM(id, apiAKS), tok, nil), 200, "", "read cluster")
		c.Expect(g.Str("properties.identityProfile.kubeletidentity.objectId") != "", "kubelet identity created")
		cr := c.Status(c.Do(post, c.ARM(id+"/listClusterAdminCredential", apiAKS), tok, nil), 200, "", "admin credentials")
		kcs, _ := cr.Body["kubeconfigs"].([]any)
		c.Expect(len(kcs) == 1, "one kubeconfig returned")
		kc, _ := kcs[0].(map[string]any)
		val, _ := kc["value"].(string)
		cfg, _ := base64.StdEncoding.DecodeString(val)
		fqdn := g.Str("properties.fqdn")
		c.Expect(strings.Contains(string(cfg), "server: https://"+fqdn), "kubeconfig points at the cluster fqdn")
		token := ""
		for _, line := range strings.Split(string(cfg), "\n") {
			if t := strings.TrimSpace(line); strings.HasPrefix(t, "token: ") {
				token = strings.TrimPrefix(t, "token: ")
			}
		}
		nodes := c.Status(c.Do(get, "https://"+fqdn+"/api/v1/nodes", token, nil), 200, "", "kubectl get nodes with the kubeconfig token")
		c.Expect(len(nodes.Body["items"].([]any)) == 2, "two nodes for a two-node pool")
		c.Status(c.Do(get, "https://"+fqdn+"/api/v1/nodes", "wrong-token", nil), 401, "", "wrong token is rejected")
		c.Status(c.Do(put, c.ARM(id, apiAKS), tok, cluster("1.35")), 400, "UpgradeFailed", "skip a minor version")
		c.Status(c.Do(put, c.ARM(id, apiAKS), tok, cluster("1.34")), 200, "", "upgrade one minor version")
		c.Status(c.Do(del, c.ARM(id, apiAKS), tok, nil), 202, "", "delete cluster")
		c.Status(c.Do(get, c.ARM("/subscriptions/"+SubSandbox+"/resourceGroups/"+nodeRG, apiRes), tok, nil), 404, "ResourceGroupNotFound", "node resource group removed")
	}}
	aks := func(c *C, tok, rg string, extra map[string]any) string {
		id := rg + "/providers/Microsoft.ContainerService/managedClusters/" + c.Name("aks")
		p := map[string]any{"dnsPrefix": "app", "agentPoolProfiles": []any{map[string]any{"name": "system", "count": 1, "vmSize": "Standard_D2s_v5", "mode": "System"}}}
		for k, v := range extra {
			p[k] = v
		}
		c.Status(c.Do(put, c.ARM(id, apiAKS), tok, map[string]any{"location": "uksouth", "identity": map[string]any{"type": "SystemAssigned"}, "properties": p}), 201, "", "create cluster")
		return id
	}
	Checks["Microsoft.ContainerService/managedClusters/agentPools"] = Check{"user pools, autoscaler bounds, last system pool protected", func(c *C) {
		tok := c.Boot("management")
		id := aks(c, tok, newRG(c, tok), nil)
		pool := func(name string, p map[string]any) Resp {
			return c.Do(put, c.ARM(id+"/agentPools/"+name, apiAKS), tok, map[string]any{"properties": p})
		}
		c.Status(pool("Apps", map[string]any{"count": 1, "vmSize": "Standard_D4s_v5"}), 400, "InvalidParameter", "upper-case pool name")
		c.Status(pool("apps", map[string]any{"count": 5, "enableAutoScaling": true, "minCount": 1, "maxCount": 3, "vmSize": "Standard_D4s_v5"}), 400, "InvalidParameter", "count above maxCount")
		u := c.Status(pool("apps", map[string]any{"count": 2, "vmSize": "Standard_D4s_v5", "mode": "User", "nodeLabels": map[string]any{"workload": "apps"}}), 201, "", "add a user pool")
		c.Expect(u.Str("properties.mode") == "User", "pool mode is User")
		c.Status(c.Do(del, c.ARM(id+"/agentPools/system", apiAKS), tok, nil), 400, "OperationNotAllowed", "delete the last system pool")
		c.Status(c.Do(del, c.ARM(id+"/agentPools/apps", apiAKS), tok, nil), 202, "", "delete the user pool")
	}}
	Checks["Microsoft.ContainerService/managedClusters/maintenanceConfigurations"] = Check{"planned maintenance windows", func(c *C) {
		tok := c.Boot("management")
		id := aks(c, tok, newRG(c, tok), nil)
		c.Status(c.Do(put, c.ARM(id+"/maintenanceConfigurations/weekends", apiAKS), tok, map[string]any{"properties": map[string]any{}}), 400, "InvalidParameter", "unknown configuration name")
		c.Status(c.Do(put, c.ARM(id+"/maintenanceConfigurations/aksManagedAutoUpgradeSchedule", apiAKS), tok, map[string]any{"properties": map[string]any{
			"maintenanceWindow": map[string]any{"schedule": map[string]any{"weekly": map[string]any{"intervalWeeks": 1, "dayOfWeek": "Sunday"}}, "durationHours": 4, "startTime": "02:00", "utcOffset": "+00:00"}}}), 201, "", "auto-upgrade window")
	}}
	Checks["Microsoft.ContainerService/managedClusters/upgradeProfiles"] = Check{"available upgrades for control plane and pools", func(c *C) {
		tok := c.Boot("management")
		id := aks(c, tok, newRG(c, tok), map[string]any{"kubernetesVersion": "1.33"})
		u := c.Status(c.Do(get, c.ARM(id+"/upgradeProfiles/default", apiAKS), tok, nil), 200, "", "read upgrade profile")
		c.Expect(strings.Contains(string(u.Raw), `"1.34.`), "1.33 can upgrade to 1.34")
		c.Expect(!strings.Contains(string(u.Raw), `"1.35.`), "1.33 cannot jump to 1.35")
	}}
	Checks["Microsoft.ContainerService/managedClusters/agentPools/upgradeProfiles"] = Check{"node pool upgrade profile", func(c *C) {
		tok := c.Boot("management")
		id := aks(c, tok, newRG(c, tok), nil)
		u := c.Status(c.Do(get, c.ARM(id+"/agentPools/system/upgradeProfiles/default", apiAKS), tok, nil), 200, "", "read pool upgrade profile")
		c.Expect(u.Str("properties.latestNodeImageVersion") != "", "latest node image reported")
	}}
	Checks["Microsoft.ContainerService/locations/kubernetesVersions"] = Check{"az aks get-versions", func(c *C) {
		v := c.Status(c.Do(get, c.ARM("/subscriptions/"+SubSandbox+"/providers/Microsoft.ContainerService/locations/uksouth/kubernetesVersions", apiAKS), c.Boot("management"), nil), 200, "", "list versions")
		c.Expect(strings.Contains(string(v.Raw), `"isDefault":true`), "one version is the default")
	}}
	Checks["Microsoft.ContainerService/locations/orchestrators"] = Check{"legacy orchestrator list", func(c *C) {
		v := c.Status(c.Do(get, c.ARM("/subscriptions/"+SubSandbox+"/providers/Microsoft.ContainerService/locations/uksouth/orchestrators", apiAKS), c.Boot("management"), nil), 200, "", "list orchestrators")
		c.Expect(strings.Contains(string(v.Raw), "Kubernetes"), "lists Kubernetes versions")
	}}
}
