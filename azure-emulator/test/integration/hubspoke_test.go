package integration

import (
	"testing"
)

// TestHubSpokeRouting walks the hub-and-spoke lab: peering is not
// transitive, a direct peering or a hub appliance (UDR + forwarded traffic)
// fixes it, and NSGs still have the last word.
func TestHubSpokeRouting(t *testing.T) {
	e := start(t)
	arm := e.url("management", "")
	tok := e.token(bootID, bootSec, arm+"/")
	const api = "?api-version=2024-05-01"
	rg := "/subscriptions/" + sandbox + "/resourceGroups/rg-lab"
	put := func(path string, body map[string]any) {
		t.Helper()
		if code, _, b := e.do("PUT", arm+path+api, tok, body); code >= 300 {
			t.Fatalf("PUT %s: %d %v", path, code, b)
		}
	}
	del := func(path string) {
		t.Helper()
		if code, _, b := e.do("DELETE", arm+path+api, tok, nil); code >= 300 {
			t.Fatalf("DELETE %s: %d %v", path, code, b)
		}
	}
	vnet := func(name, cidr string, subnets map[string]string) string {
		id := rg + "/providers/Microsoft.Network/virtualNetworks/" + name
		var sn []any
		for n, c := range subnets {
			sn = append(sn, map[string]any{"name": n, "properties": map[string]any{"addressPrefix": c}})
		}
		put(id, map[string]any{"location": "uksouth", "properties": map[string]any{"addressSpace": map[string]any{"addressPrefixes": []any{cidr}}, "subnets": sn}})
		return id
	}
	peer := func(from, to, name string, fwd bool) {
		put(from+"/virtualNetworkPeerings/"+name, map[string]any{"properties": map[string]any{"remoteVirtualNetwork": map[string]any{"id": to}, "allowForwardedTraffic": fwd}})
	}
	trace := func(from, to string, port string) (string, map[string]any) {
		t.Helper()
		code, _, b := e.do("GET", e.url("management", "/_cloudtyke/network/trace?from="+from+"&to="+to+"&port="+port), "", nil)
		if code != 200 {
			t.Fatalf("trace: %d %v", code, b)
		}
		r, _ := b["result"].(string)
		return r, b
	}
	expect := func(step, from, to, port, want string) {
		t.Helper()
		got, b := trace(from, to, port)
		if got != want {
			t.Fatalf("%s: %s -> %s:%s want %s, got %s (%v)", step, from, to, port, want, got, b["summary"])
		}
	}

	put(rg[:len(rg)], map[string]any{"location": "uksouth"})
	hub := vnet("vnet-hub", "10.0.0.0/16", map[string]string{"AzureFirewallSubnet": "10.0.0.0/26", "snet-shared": "10.0.1.0/24"})
	a := vnet("vnet-spoke-a", "10.10.0.0/16", map[string]string{"snet-app": "10.10.1.0/24"})
	b := vnet("vnet-spoke-b", "10.20.0.0/16", map[string]string{"snet-app": "10.20.1.0/24"})

	// 1. no peering at all: private ranges have next hop None
	expect("isolated", "10.10.1.4", "10.20.1.4", "443", "Dropped")
	expect("same vnet", "10.10.1.4", "10.10.1.5", "443", "Delivered")

	// 2. hub-and-spoke peerings: spokes reach the hub ...
	peer(a, hub, "a-to-hub", true)
	expect("half-peered", "10.10.1.4", "10.0.1.4", "443", "Dropped") // Initiated only
	peer(hub, a, "hub-to-a", true)
	peer(b, hub, "b-to-hub", true)
	peer(hub, b, "hub-to-b", true)
	expect("spoke to hub", "10.10.1.4", "10.0.1.4", "443", "Delivered")
	// ... but not each other: peering is not transitive
	expect("spoke to spoke via hub peering only", "10.10.1.4", "10.20.1.4", "443", "Dropped")

	// 3a. fix 1: direct spoke-to-spoke peering
	peer(a, b, "a-to-b", false)
	peer(b, a, "b-to-a", false)
	expect("direct peering", "10.10.1.4", "10.20.1.4", "443", "Delivered")
	del(a + "/virtualNetworkPeerings/a-to-b")
	del(b + "/virtualNetworkPeerings/b-to-a")
	expect("direct peering removed", "10.10.1.4", "10.20.1.4", "443", "Dropped")

	// 3b. fix 2: route spoke traffic through the hub firewall (10.0.0.4)
	rt := func(name, prefix string) string {
		id := rg + "/providers/Microsoft.Network/routeTables/" + name
		put(id, map[string]any{"location": "uksouth", "properties": map[string]any{"routes": []any{map[string]any{"name": "to-other-spoke", "properties": map[string]any{
			"addressPrefix": prefix, "nextHopType": "VirtualAppliance", "nextHopIpAddress": "10.0.0.4"}}}}})
		return id
	}
	rtA, rtB := rt("rt-a", "10.20.0.0/16"), rt("rt-b", "10.10.0.0/16")
	put(a+"/subnets/snet-app", map[string]any{"properties": map[string]any{"addressPrefix": "10.10.1.0/24", "routeTable": map[string]any{"id": rtA}}})
	put(b+"/subnets/snet-app", map[string]any{"properties": map[string]any{"addressPrefix": "10.20.1.0/24", "routeTable": map[string]any{"id": rtB}}})
	expect("via hub firewall", "10.10.1.4", "10.20.1.4", "443", "Delivered")
	expect("return path via hub firewall", "10.20.1.4", "10.10.1.4", "443", "Delivered")

	// forwarded traffic must be allowed on the receiving spoke's peering
	peer(b, hub, "b-to-hub", false)
	_, res := trace("10.10.1.4", "10.20.1.4", "443")
	if res["result"] != "Dropped" {
		t.Fatalf("without allowForwardedTraffic the packet must be dropped, got %v", res["summary"])
	}
	peer(b, hub, "b-to-hub", true)

	// 4. NSGs still decide: deny 443 into spoke B's app subnet
	nsg := rg + "/providers/Microsoft.Network/networkSecurityGroups/nsg-b"
	put(nsg, map[string]any{"location": "uksouth", "properties": map[string]any{"securityRules": []any{map[string]any{"name": "deny-spoke-a-https",
		"properties": map[string]any{"priority": 200, "direction": "Inbound", "access": "Deny", "protocol": "Tcp", "sourcePortRange": "*", "destinationPortRange": "443",
			"sourceAddressPrefix": "10.10.0.0/16", "destinationAddressPrefix": "*"}}}}})
	put(b+"/subnets/snet-app", map[string]any{"properties": map[string]any{"addressPrefix": "10.20.1.0/24", "routeTable": map[string]any{"id": rtB}, "networkSecurityGroup": map[string]any{"id": nsg}}})
	expect("nsg deny 443", "10.10.1.4", "10.20.1.4", "443", "Blocked")
	expect("nsg allows 8080 (VirtualNetwork tag covers UDR prefixes)", "10.10.1.4", "10.20.1.4", "8080", "Delivered")

	// effective routes show the UDR overriding nothing and the system routes
	code, _, routes := e.do("GET", e.url("management", "/_cloudtyke/network/routes?ip=10.10.1.4"), "", nil)
	if code != 200 {
		t.Fatalf("routes: %d %v", code, routes)
	}
	seenUDR, seenNone := false, false
	for _, r := range routes["value"].([]any) {
		m := r.(map[string]any)
		if m["source"] == "User" && m["nextHopType"] == "VirtualAppliance" {
			seenUDR = true
		}
		if m["addressPrefix"] == "10.0.0.0/8" && m["nextHopType"] == "None" {
			seenNone = true
		}
	}
	if !seenUDR || !seenNone {
		t.Fatalf("effective routes should list the UDR and the 10.0.0.0/8 None route: %v", routes["value"])
	}
	_ = hub
}
