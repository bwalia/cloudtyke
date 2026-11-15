// Package network implements the Microsoft.Network control plane used by
// landing zones and AKS: virtual networks, subnets, peerings, network
// security groups, route tables, public IP addresses and private DNS zones.
// Address spaces are validated like Azure (CIDR notation, subnets inside the
// VNet and not overlapping, peered VNets not overlapping) and resources that
// are still referenced cannot be deleted.
package network

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/arm"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

// APIVersions accepted by the Microsoft.Network types.
var APIVersions = []string{"2024-07-01", "2024-05-01", "2024-03-01", "2024-01-01", "2023-11-01", "2023-09-01", "2023-06-01", "2023-05-01", "2023-04-01", "2023-02-01", "2022-07-01"}

var dnsVersions = []string{"2024-06-01", "2020-06-01", "2018-09-01"}

const ns = "Microsoft.Network"

var (
	subnets = arm.Inline{Prop: "subnets", ChildType: "subnets", Validate: validateSubnet}
	rules   = arm.Inline{Prop: "securityRules", ChildType: "securityRules", Validate: validateRule}
	routes  = arm.Inline{Prop: "routes", ChildType: "routes", Validate: validateRoute}
)

// Register wires the RP.
func Register(s *arm.Server) {
	reg := func(types []string, versions []string, notes string, h arm.Handler) {
		s.Registry.Register(&arm.TypeRegistration{Namespace: ns, Types: types, APIVersions: versions, Scopes: []arm.ScopeKind{arm.KindResourceGroup}, Phase: 2, Notes: notes, Handler: h})
	}

	registerAVNM(s)

	reg([]string{"virtualNetworks"}, APIVersions, "address space validation, inline subnets synced with child resources, peerings listed, delete blocked while subnets are in use",
		arm.Generic(arm.GenericOptions{Tracked: true, Async: true,
			Prepare: func(q *arm.Request, body, existing map[string]any) error { return prepareVNet(q, body) },
			Present: func(q *arm.Request, b map[string]any) map[string]any { return presentVNet(q.S, b) },
			OnDelete: func(q *arm.Request, existing map[string]any) error {
				for _, sn := range q.S.Children(strOf(existing["id"]) + "/subnets") {
					if users := referencedBy(q.S, strOf(sn["id"])); len(users) > 0 {
						return &arm.APIError{Status: 400, Code: "InUseSubnetCannotBeDeleted", Message: arm.Errorf("Subnet %s is in use by %s and cannot be deleted. In order to delete the subnet, delete all the resources within the subnet. See aka.ms/deletesubnet.", sn["name"], strings.Join(users, ", "))}
					}
				}
				return nil
			},
		}))

	reg([]string{"virtualNetworks", "subnets"}, APIVersions, "CIDR inside the VNet, no overlap with sibling subnets, NSG / route table association, delegations, service endpoints",
		arm.Generic(arm.GenericOptions{Async: true,
			Prepare: func(q *arm.Request, body, existing map[string]any) error {
				parent := q.ID.ParentPath(ns, q.Reg.Types)
				vnet, _ := q.S.LoadResource(parent)
				vp, _ := vnet["properties"].(map[string]any)
				return validateSubnet(q, parent, vp, body, subnets.Siblings(q.S, parent, strOf(body["name"])))
			},
			Present: func(q *arm.Request, b map[string]any) map[string]any { return presentSubnet(q.S, b) },
			OnDelete: func(q *arm.Request, existing map[string]any) error {
				if users := referencedBy(q.S, strOf(existing["id"])); len(users) > 0 {
					return &arm.APIError{Status: 400, Code: "InUseSubnetCannotBeDeleted", Message: arm.Errorf("Subnet %s is in use by %s and cannot be deleted. In order to delete the subnet, delete all the resources within the subnet. See aka.ms/deletesubnet.", existing["name"], strings.Join(users, ", "))}
				}
				return nil
			},
		}))

	reg([]string{"virtualNetworks", "virtualNetworkPeerings"}, APIVersions, "peeringState Initiated until the remote side peers back, then Connected (Disconnected if the remote VNet goes away); overlapping address spaces rejected",
		arm.Generic(arm.GenericOptions{Async: true,
			Prepare: preparePeering,
			Present: func(q *arm.Request, b map[string]any) map[string]any { return presentPeering(q.S, b) },
		}))

	reg([]string{"networkSecurityGroups"}, APIVersions, "inline securityRules synced with child resources, the six default rules, priority/direction conflicts, delete blocked while associated",
		arm.Generic(arm.GenericOptions{Tracked: true, Async: true,
			Prepare: func(q *arm.Request, body, existing map[string]any) error {
				stripReadOnly("networkSecurityGroups", props(body))
				return rules.Sync(q, q.ID.Path(ns, q.Reg.Types), ns+"/networkSecurityGroups", props(body))
			},
			Present: func(q *arm.Request, b map[string]any) map[string]any { return presentNSG(q.S, b) },
			OnDelete: func(q *arm.Request, existing map[string]any) error {
				if users := referencedBy(q.S, strOf(existing["id"])); len(users) > 0 {
					return &arm.APIError{Status: 400, Code: "InUseNetworkSecurityGroupCannotBeDeleted", Message: arm.Errorf("Network security group %s cannot be deleted because it is in use by the following resources: %s. In order to delete the Network security group, remove the association with the resource(s).", existing["id"], strings.Join(users, ", "))}
				}
				return nil
			},
		}))

	reg([]string{"networkSecurityGroups", "securityRules"}, APIVersions, "priority 100-4096 unique per direction, Allow/Deny, prefixes and port ranges",
		arm.Generic(arm.GenericOptions{Async: true,
			Prepare: func(q *arm.Request, body, existing map[string]any) error {
				parent := q.ID.ParentPath(ns, q.Reg.Types)
				return validateRule(q, parent, nil, body, rules.Siblings(q.S, parent, strOf(body["name"])))
			},
		}))

	reg([]string{"routeTables"}, APIVersions, "inline routes synced with child resources, BGP propagation flag, delete blocked while associated",
		arm.Generic(arm.GenericOptions{Tracked: true, Async: true,
			Prepare: func(q *arm.Request, body, existing map[string]any) error {
				p := props(body)
				stripReadOnly("routeTables", p)
				if _, ok := p["disableBgpRoutePropagation"]; !ok {
					p["disableBgpRoutePropagation"] = false
				}
				return routes.Sync(q, q.ID.Path(ns, q.Reg.Types), ns+"/routeTables", p)
			},
			Present: func(q *arm.Request, b map[string]any) map[string]any {
				b = arm.DeepCopy(b)
				p := props(b)
				routes.Present(q.S, strOf(b["id"]), p, nil)
				p["subnets"] = subnetRefs(q.S, strOf(b["id"]), "routeTable")
				return b
			},
			OnDelete: func(q *arm.Request, existing map[string]any) error {
				if users := referencedBy(q.S, strOf(existing["id"])); len(users) > 0 {
					return &arm.APIError{Status: 400, Code: "InUseRouteTableCannotBeDeleted", Message: arm.Errorf("Route table %s is in use and cannot be deleted. In order to delete the route table, remove all subnet associations: %s.", existing["id"], strings.Join(users, ", "))}
				}
				return nil
			},
		}))

	reg([]string{"routeTables", "routes"}, APIVersions, "next hop types; VirtualAppliance requires nextHopIpAddress",
		arm.Generic(arm.GenericOptions{Async: true,
			Prepare: func(q *arm.Request, body, existing map[string]any) error {
				parent := q.ID.ParentPath(ns, q.Reg.Types)
				return validateRoute(q, parent, nil, body, routes.Siblings(q.S, parent, strOf(body["name"])))
			},
		}))

	reg([]string{"publicIPAddresses"}, APIVersions, "Standard/Basic SKU, static/dynamic allocation, stable Azure-looking addresses, DNS labels, zones",
		arm.Generic(arm.GenericOptions{Tracked: true, Async: true, Prepare: preparePublicIP}))

	// Private DNS (global)
	regDNS := func(types []string, notes string, o arm.GenericOptions) {
		s.Registry.Register(&arm.TypeRegistration{Namespace: ns, Types: types, APIVersions: dnsVersions, Scopes: []arm.ScopeKind{arm.KindResourceGroup}, Phase: 2, Notes: notes, Handler: arm.Generic(o)})
	}
	regDNS([]string{"privateDnsZones"}, "location global, SOA record created automatically, record/link counters, delete blocked while VNet links exist", arm.GenericOptions{Tracked: true, Async: true,
		Prepare: func(q *arm.Request, body, existing map[string]any) error {
			if !strings.EqualFold(strOf(body["location"]), "global") {
				return arm.BadRequest("InvalidLocation", "The location of a private DNS zone must be 'global'.")
			}
			name := q.ID.Name()
			if !strings.Contains(name, ".") || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
				return arm.BadRequest("InvalidZoneName", arm.Errorf("The zone name '%s' is not valid: it must have at least two labels.", name))
			}
			p := props(body)
			if existing != nil {
				p["internalId"] = props(existing)["internalId"]
			} else {
				p["internalId"] = "ImzvbmU7" + strings.ReplaceAll(entra.NewGUID(), "-", "")[:16]
			}
			return nil
		},
		AfterWrite: func(q *arm.Request, body map[string]any) {
			soa := strOf(body["id"]) + "/SOA/@"
			if _, ok := q.S.LoadResource(soa); !ok {
				_ = q.S.SaveResource(soaRecord(strOf(body["id"]), q.ID.Name()))
			}
		},
		Present: func(q *arm.Request, b map[string]any) map[string]any { return presentZone(q.S, b) },
		OnDelete: func(q *arm.Request, existing map[string]any) error {
			if links := q.S.Children(strOf(existing["id"]) + "/virtualNetworkLinks"); len(links) > 0 {
				return &arm.APIError{Status: 409, Code: "CannotDeleteResource", Message: arm.Errorf("Can not delete resource before nested resources are deleted. The zone has %d virtual network link(s).", len(links))}
			}
			return nil
		},
	})
	regDNS([]string{"privateDnsZones", "virtualNetworkLinks"}, "link a zone to a VNet (must exist), auto-registration flag, one registration zone per VNet", arm.GenericOptions{Tracked: true, Async: true, Prepare: prepareLink})
	for _, rt := range []string{"A", "AAAA", "CNAME", "TXT", "SOA"} {
		rt := rt
		regDNS([]string{"privateDnsZones", rt}, rt+" record sets with TTL and fqdn", arm.GenericOptions{Async: false,
			Prepare: func(q *arm.Request, body, existing map[string]any) error { return prepareRecord(q, body, rt) },
		})
	}
}

// ------------------------------------------------------------ helpers

func strOf(v any) string { s, _ := v.(string); return s }

func props(b map[string]any) map[string]any {
	p, _ := b["properties"].(map[string]any)
	if p == nil {
		p = map[string]any{}
		b["properties"] = p
	}
	return p
}

func strList(v any) []string {
	var out []string
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	case []string:
		out = t
	}
	return out
}

// readOnly lists computed properties Azure ignores on PUT. Clients (Terraform)
// often echo them back from a GET; storing them would create false references.
var readOnly = map[string][]string{
	"virtualNetworks":       {"virtualNetworkPeerings"},
	"subnets":               {"ipConfigurations", "ipConfigurationProfiles", "privateEndpoints", "resourceNavigationLinks", "serviceAssociationLinks", "purpose"},
	"networkSecurityGroups": {"defaultSecurityRules", "subnets", "networkInterfaces", "flowLogs"},
	"routeTables":           {"subnets"},
	"publicIPAddresses":     {"ipConfiguration"},
}

func stripReadOnly(kind string, p map[string]any) {
	for _, k := range readOnly[kind] {
		delete(p, k)
	}
}

func refID(v any) string {
	m, _ := v.(map[string]any)
	return strOf(m["id"])
}

// referencedBy lists resources (outside id's own subtree) whose stored JSON
// refers to id: subnets referencing an NSG, AKS pools referencing a subnet, ...
func referencedBy(s *arm.Server, id string) []string {
	needle := strings.ToLower(id)
	own := arm.StoreKey(id)
	var users []string
	_ = s.Store.Scan(store.BucketARM, "", func(k string, v []byte) bool {
		if k == own || strings.HasPrefix(k, own+"/") || strings.Contains(k, "/providers/microsoft.authorization/") {
			return true
		}
		if strings.Contains(strings.ToLower(string(v)), `"`+needle+`"`) {
			var m map[string]any
			if json.Unmarshal(v, &m) == nil {
				users = append(users, strOf(m["id"]))
			}
		}
		return true
	})
	sort.Strings(users)
	return users
}

func subnetRefs(s *arm.Server, id, field string) []any {
	out := []any{}
	for _, u := range referencedBy(s, id) {
		if m, ok := s.LoadResource(u); ok && strings.EqualFold(strOf(m["type"]), ns+"/virtualNetworks/subnets") && strings.EqualFold(refID(props(m)[field]), id) {
			out = append(out, map[string]any{"id": u})
		}
	}
	return out
}

func badCIDR(id, s string, err error) error {
	return arm.BadRequest("InvalidCIDRNotation", arm.Errorf("The address prefix %s in resource %s has an invalid CIDR notation: %v.", s, id, err))
}

// ------------------------------------------------------------ virtual networks

func vnetPrefixes(p map[string]any) []string {
	as, _ := p["addressSpace"].(map[string]any)
	return strList(as["addressPrefixes"])
}

func prepareVNet(q *arm.Request, body map[string]any) error {
	id := q.ID.Path(ns, q.Reg.Types)
	p := props(body)
	stripReadOnly("virtualNetworks", p)
	if list, ok := p["subnets"].([]any); ok {
		for _, it := range list {
			if m, _ := it.(map[string]any); m != nil {
				stripReadOnly("subnets", props(m))
			}
		}
	}
	list := vnetPrefixes(p)
	if len(list) == 0 {
		return arm.BadRequest("InvalidAddressSpace", arm.Errorf("Virtual network %s must have at least one address prefix in properties.addressSpace.addressPrefixes.", q.ID.Name()))
	}
	pfx, err := parsePrefixes(list)
	if err != nil {
		return badCIDR(id, strings.Join(list, ","), err)
	}
	for i := range pfx {
		for j := i + 1; j < len(pfx); j++ {
			if overlaps(pfx[i], pfx[j]) {
				return arm.BadRequest("NetcfgInvalidAddressSpace", arm.Errorf("Address space %s overlaps with address space %s in virtual network %s.", pfx[i], pfx[j], q.ID.Name()))
			}
		}
	}
	// existing subnets must stay inside a changed address space
	if _, inline := p["subnets"]; !inline {
		for _, sn := range q.S.Children(id + "/subnets") {
			for _, sp := range subnetPrefixList(props(sn)) {
				if pp, err := netip.ParsePrefix(sp); err == nil && !withinAny(pp, pfx) {
					return arm.BadRequest("NetcfgSubnetRangeOutsideVnet", arm.Errorf("Subnet '%s' is not valid because its IP address range is outside the IP address range of virtual network '%s'.", sn["name"], q.ID.Name()))
				}
			}
		}
	}
	// peered VNets must not overlap the new space
	for _, pr := range q.S.Children(id + "/virtualNetworkPeerings") {
		if remote, ok := q.S.LoadResource(refID(props(pr)["remoteVirtualNetwork"])); ok {
			rp, _ := parsePrefixes(vnetPrefixes(props(remote)))
			if a, b, bad := anyOverlap(pfx, rp); bad {
				return arm.BadRequest("VnetAddressSpacesOverlap", arm.Errorf("Cannot update virtual network %s: address space %s overlaps with %s of peered virtual network %s.", q.ID.Name(), a, b, remote["name"]))
			}
		}
	}
	if _, ok := p["enableDdosProtection"]; !ok {
		p["enableDdosProtection"] = false
	}
	if p["resourceGuid"] == nil {
		if old, ok := q.S.LoadResource(id); ok {
			p["resourceGuid"] = props(old)["resourceGuid"]
		}
		if p["resourceGuid"] == nil {
			p["resourceGuid"] = entra.NewGUID()
		}
	}
	return subnets.Sync(q, id, ns+"/virtualNetworks", p)
}

func presentVNet(s *arm.Server, b map[string]any) map[string]any {
	b = arm.DeepCopy(b)
	p := props(b)
	subnets.Present(s, strOf(b["id"]), p, func(m map[string]any) map[string]any { return presentSubnet(s, m) })
	peers := []any{}
	for _, pr := range s.Children(strOf(b["id"]) + "/virtualNetworkPeerings") {
		pp := presentPeering(s, pr)
		delete(pp, "systemData")
		peers = append(peers, pp)
	}
	p["virtualNetworkPeerings"] = peers
	return b
}

// ------------------------------------------------------------ subnets

func subnetPrefixList(p map[string]any) []string {
	if l := strList(p["addressPrefixes"]); len(l) > 0 {
		return l
	}
	if s := strOf(p["addressPrefix"]); s != "" {
		return []string{s}
	}
	return nil
}

func validateSubnet(q *arm.Request, vnetID string, vnetProps, child map[string]any, siblings []map[string]any) error {
	p := props(child)
	stripReadOnly("subnets", p)
	name := strOf(child["name"])
	list := subnetPrefixList(p)
	if len(list) == 0 {
		return arm.BadRequest("NetcfgInvalidSubnet", arm.Errorf("Subnet '%s' must specify properties.addressPrefix or properties.addressPrefixes.", name))
	}
	pfx, err := parsePrefixes(list)
	if err != nil {
		return badCIDR(strOf(child["id"]), strings.Join(list, ","), err)
	}
	vnetName := vnetID[strings.LastIndex(vnetID, "/")+1:]
	space, _ := parsePrefixes(vnetPrefixes(vnetProps))
	for _, x := range pfx {
		if !withinAny(x, space) {
			return arm.BadRequest("NetcfgSubnetRangeOutsideVnet", arm.Errorf("Subnet '%s' is not valid because its IP address range is outside the IP address range of virtual network '%s'.", name, vnetName))
		}
		if x.Bits() > 29 {
			return arm.BadRequest("NetcfgInvalidSubnet", arm.Errorf("Subnet '%s' is not valid: the smallest supported subnet is /29, got %s.", name, x))
		}
	}
	for _, sib := range siblings {
		sp, _ := parsePrefixes(subnetPrefixList(props(sib)))
		if _, _, bad := anyOverlap(pfx, sp); bad {
			return arm.BadRequest("NetcfgSubnetRangesOverlap", arm.Errorf("Subnet '%s' is not valid because its IP address range overlaps with that of an existing subnet '%s' in virtual network '%s'.", name, sib["name"], vnetName))
		}
	}
	for _, f := range []struct{ field, typ string }{{"networkSecurityGroup", "networkSecurityGroups"}, {"routeTable", "routeTables"}, {"natGateway", "natGateways"}} {
		if ref := refID(p[f.field]); ref != "" && f.typ != "natGateways" {
			if _, ok := q.S.LoadResource(ref); !ok {
				return &arm.APIError{Status: 404, Code: "InvalidResourceReference", Message: arm.Errorf("Resource %s referenced by resource %s was not found. Please make sure that the referenced resource exists, and that both resources are in the same region.", ref, child["id"])}
			}
		}
	}
	if _, ok := p["privateEndpointNetworkPolicies"]; !ok {
		p["privateEndpointNetworkPolicies"] = "Disabled"
	}
	if _, ok := p["privateLinkServiceNetworkPolicies"]; !ok {
		p["privateLinkServiceNetworkPolicies"] = "Enabled"
	}
	if _, ok := p["delegations"]; !ok {
		p["delegations"] = []any{}
	}
	if d, ok := p["delegations"].([]any); ok {
		for i, x := range d {
			m, _ := x.(map[string]any)
			if m == nil {
				continue
			}
			if strOf(m["id"]) == "" {
				m["id"] = strOf(child["id"]) + "/delegations/" + strOf(m["name"])
			}
			m["type"] = ns + "/virtualNetworks/subnets/delegations"
			dp, _ := m["properties"].(map[string]any)
			if dp != nil {
				dp["provisioningState"] = "Succeeded"
				if strOf(dp["serviceName"]) != "" && dp["actions"] == nil {
					dp["actions"] = []any{ns + "/virtualNetworks/subnets/join/action"}
				}
			}
			d[i] = m
		}
	}
	return nil
}

func presentSubnet(s *arm.Server, b map[string]any) map[string]any {
	b = arm.DeepCopy(b)
	p := props(b)
	var cfgs []any
	for _, u := range referencedBy(s, strOf(b["id"])) {
		if !strings.EqualFold(u, strOf(b["id"])) {
			cfgs = append(cfgs, map[string]any{"id": u})
		}
	}
	if len(cfgs) > 0 {
		p["ipConfigurations"] = cfgs
	}
	return b
}

// ------------------------------------------------------------ peerings

func preparePeering(q *arm.Request, body, existing map[string]any) error {
	p := props(body)
	local := q.ID.ParentPath(ns, q.Reg.Types)
	rid := refID(p["remoteVirtualNetwork"])
	if rid == "" {
		return arm.BadRequest("InvalidRequestFormat", "properties.remoteVirtualNetwork.id is required.")
	}
	if strings.EqualFold(rid, local) {
		return arm.BadRequest("CannotPeerVirtualNetworkWithItself", "A virtual network cannot be peered with itself.")
	}
	remote, ok := q.S.LoadResource(rid)
	if !ok {
		return &arm.APIError{Status: 404, Code: "ReferencedResourceNotProvisioned", Message: arm.Errorf("Cannot proceed with operation because resource %s used by resource %s is not in Succeeded state. Resource is in Failed state and the last operation that updated/is updating the resource is PutVirtualNetworkOperation.", rid, body["id"])}
	}
	lv, _ := q.S.LoadResource(local)
	lp, _ := parsePrefixes(vnetPrefixes(props(lv)))
	rp, _ := parsePrefixes(vnetPrefixes(props(remote)))
	if a, b, bad := anyOverlap(lp, rp); bad {
		return arm.BadRequest("VnetAddressSpacesOverlap", arm.Errorf("Cannot create or update peering %s. Virtual networks %s and %s cannot be peered because their address spaces overlap. Overlapping address prefixes: %s, %s", q.ID.Name(), lv["name"], remote["name"], a, b))
	}
	for k, def := range map[string]any{"allowVirtualNetworkAccess": true, "allowForwardedTraffic": false, "allowGatewayTransit": false, "useRemoteGateways": false, "doNotVerifyRemoteGateways": false} {
		if _, ok := p[k]; !ok {
			p[k] = def
		}
	}
	if p["useRemoteGateways"] == true {
		return arm.BadRequest("RemotePeeringIsDisconnected", "useRemoteGateways requires a virtual network gateway in the remote VNet, which CloudTyke does not emulate yet.")
	}
	if existing == nil {
		p["resourceGuid"] = entra.NewGUID()
	} else {
		p["resourceGuid"] = props(existing)["resourceGuid"]
	}
	delete(p, "peeringState")
	delete(p, "peeringSyncLevel")
	return nil
}

func presentPeering(s *arm.Server, b map[string]any) map[string]any {
	b = arm.DeepCopy(b)
	p := props(b)
	id := strOf(b["id"])
	local := id[:strings.Index(strings.ToLower(id), "/virtualnetworkpeerings/")]
	state := "Disconnected"
	if remote, ok := s.LoadResource(refID(p["remoteVirtualNetwork"])); ok {
		state = "Initiated"
		for _, back := range s.Children(strOf(remote["id"]) + "/virtualNetworkPeerings") {
			if strings.EqualFold(refID(props(back)["remoteVirtualNetwork"]), local) {
				state = "Connected"
			}
		}
		as := map[string]any{"addressPrefixes": toAny(vnetPrefixes(props(remote)))}
		p["remoteAddressSpace"] = as
		p["remoteVirtualNetworkAddressSpace"] = as
	}
	p["peeringState"] = state
	if state == "Connected" {
		p["peeringSyncLevel"] = "FullyInSync"
	} else {
		p["peeringSyncLevel"] = "RemoteNotInSync"
	}
	return b
}

func toAny(l []string) []any {
	out := make([]any, len(l))
	for i, s := range l {
		out[i] = s
	}
	return out
}

// ------------------------------------------------------------ NSGs

var defaultRules = []struct {
	name, desc, dir, access, src, dst string
	prio                              int
}{
	{"AllowVnetInBound", "Allow inbound traffic from all VMs in VNET", "Inbound", "Allow", "VirtualNetwork", "VirtualNetwork", 65000},
	{"AllowAzureLoadBalancerInBound", "Allow inbound traffic from azure load balancer", "Inbound", "Allow", "AzureLoadBalancer", "*", 65001},
	{"DenyAllInBound", "Deny all inbound traffic", "Inbound", "Deny", "*", "*", 65500},
	{"AllowVnetOutBound", "Allow outbound traffic from all VMs to all VMs in VNET", "Outbound", "Allow", "VirtualNetwork", "VirtualNetwork", 65000},
	{"AllowInternetOutBound", "Allow outbound traffic from all VMs to Internet", "Outbound", "Allow", "*", "Internet", 65001},
	{"DenyAllOutBound", "Deny all outbound traffic", "Outbound", "Deny", "*", "*", 65500},
}

func presentNSG(s *arm.Server, b map[string]any) map[string]any {
	b = arm.DeepCopy(b)
	id := strOf(b["id"])
	p := props(b)
	rules.Present(s, id, p, nil)
	var defs []any
	for _, r := range defaultRules {
		defs = append(defs, map[string]any{
			"id": id + "/defaultSecurityRules/" + r.name, "name": r.name, "type": ns + "/networkSecurityGroups/defaultSecurityRules",
			"properties": map[string]any{"provisioningState": "Succeeded", "description": r.desc, "protocol": "*", "sourcePortRange": "*", "destinationPortRange": "*",
				"sourceAddressPrefix": r.src, "destinationAddressPrefix": r.dst, "access": r.access, "priority": r.prio, "direction": r.dir,
				"sourcePortRanges": []any{}, "destinationPortRanges": []any{}, "sourceAddressPrefixes": []any{}, "destinationAddressPrefixes": []any{}},
		})
	}
	p["defaultSecurityRules"] = defs
	if refs := subnetRefs(s, id, "networkSecurityGroup"); len(refs) > 0 {
		p["subnets"] = refs
	}
	if p["resourceGuid"] == nil {
		p["resourceGuid"] = entra.NewGUID()
	}
	return b
}

func validateRule(q *arm.Request, parent string, _ map[string]any, child map[string]any, siblings []map[string]any) error {
	p := props(child)
	name := strOf(child["name"])
	prio, ok := p["priority"].(float64)
	if !ok {
		if i, ok2 := p["priority"].(int); ok2 {
			prio, ok = float64(i), true
		}
	}
	if !ok || prio < 100 || prio > 4096 {
		return arm.BadRequest("SecurityRuleInvalidPriority", arm.Errorf("Security rule %s has invalid Priority. Value provided: %v Allowed range 100-4096.", name, p["priority"]))
	}
	dir := strOf(p["direction"])
	if dir != "Inbound" && dir != "Outbound" {
		return arm.BadRequest("SecurityRuleInvalidDirection", arm.Errorf("Security rule %s has invalid direction '%s'. Allowed values are Inbound and Outbound.", name, dir))
	}
	if a := strOf(p["access"]); a != "Allow" && a != "Deny" {
		return arm.BadRequest("SecurityRuleInvalidAccess", arm.Errorf("Security rule %s has invalid access '%s'. Allowed values are Allow and Deny.", name, a))
	}
	switch strOf(p["protocol"]) {
	case "Tcp", "Udp", "Icmp", "Esp", "Ah", "*":
	default:
		return arm.BadRequest("SecurityRuleInvalidProtocol", arm.Errorf("Security rule %s has invalid protocol '%v'.", name, p["protocol"]))
	}
	for _, sib := range siblings {
		sp := props(sib)
		if sp["priority"] == p["priority"] || fmt.Sprint(sp["priority"]) == fmt.Sprint(p["priority"]) {
			if strOf(sp["direction"]) == dir {
				return arm.BadRequest("SecurityRuleConflict", arm.Errorf("Security rule %s conflicts with rule %s. Rules cannot have the same Priority and Direction. To learn more, see aka.ms/nsgrules.", name, sib["name"]))
			}
		}
	}
	for _, k := range []string{"sourcePortRanges", "destinationPortRanges", "sourceAddressPrefixes", "destinationAddressPrefixes", "sourceApplicationSecurityGroups", "destinationApplicationSecurityGroups"} {
		if _, ok := p[k]; !ok {
			p[k] = []any{}
		}
	}
	return nil
}

// ------------------------------------------------------------ routes

func validateRoute(q *arm.Request, parent string, _ map[string]any, child map[string]any, _ []map[string]any) error {
	p := props(child)
	name := strOf(child["name"])
	if ap := strOf(p["addressPrefix"]); ap == "" {
		return arm.BadRequest("InvalidRequestFormat", arm.Errorf("Route %s must have properties.addressPrefix.", name))
	} else if _, err := netip.ParsePrefix(ap); err != nil && !isServiceTag(ap) {
		return badCIDR(strOf(child["id"]), ap, err)
	}
	switch strOf(p["nextHopType"]) {
	case "VirtualAppliance":
		ip := strOf(p["nextHopIpAddress"])
		if _, err := netip.ParseAddr(ip); err != nil {
			return arm.BadRequest("InvalidRequestFormat", arm.Errorf("Route %s has nextHopType VirtualAppliance and needs a valid nextHopIpAddress (got '%s').", name, ip))
		}
	case "VirtualNetworkGateway", "VnetLocal", "Internet", "None":
		if strOf(p["nextHopIpAddress"]) != "" {
			return arm.BadRequest("InvalidRequestFormat", arm.Errorf("Route %s: nextHopIpAddress is only allowed when nextHopType is VirtualAppliance.", name))
		}
	default:
		return arm.BadRequest("InvalidRequestFormat", arm.Errorf("Route %s has invalid nextHopType '%v'. Allowed: VirtualNetworkGateway, VnetLocal, Internet, VirtualAppliance, None.", name, p["nextHopType"]))
	}
	return nil
}

func isServiceTag(s string) bool { return !strings.ContainsAny(s, "./:") }

// ------------------------------------------------------------ public IPs

func preparePublicIP(q *arm.Request, body, existing map[string]any) error {
	p := props(body)
	stripReadOnly("publicIPAddresses", p)
	sku, _ := body["sku"].(map[string]any)
	if sku == nil {
		sku = map[string]any{"name": "Standard"}
		body["sku"] = sku
	}
	if strOf(sku["tier"]) == "" {
		sku["tier"] = "Regional"
	}
	alloc := strOf(p["publicIPAllocationMethod"])
	if alloc == "" {
		alloc = "Dynamic"
		if strOf(sku["name"]) == "Standard" {
			alloc = "Static"
		}
		p["publicIPAllocationMethod"] = alloc
	}
	if strOf(sku["name"]) == "Standard" && alloc != "Static" {
		return arm.BadRequest("StandardSkuPublicIPAddressesCanNotBeDynamic", arm.Errorf("Standard sku publicIp %s must have AllocationMethod set to Static.", body["id"]))
	}
	if strOf(p["publicIPAddressVersion"]) == "" {
		p["publicIPAddressVersion"] = "IPv4"
	}
	if p["idleTimeoutInMinutes"] == nil {
		p["idleTimeoutInMinutes"] = 4
	}
	if p["ipTags"] == nil {
		p["ipTags"] = []any{}
	}
	if alloc == "Static" {
		p["ipAddress"] = fakePublicIP(strOf(body["id"]))
	} else {
		delete(p, "ipAddress")
	}
	if d, _ := p["dnsSettings"].(map[string]any); d != nil && strOf(d["domainNameLabel"]) != "" {
		d["fqdn"] = strOf(d["domainNameLabel"]) + "." + strOf(body["location"]) + ".cloudapp.azure.com"
	}
	if existing != nil {
		p["resourceGuid"] = props(existing)["resourceGuid"]
	} else {
		p["resourceGuid"] = entra.NewGUID()
	}
	return nil
}

// ------------------------------------------------------------ private DNS

func soaRecord(zoneID, zone string) map[string]any {
	return map[string]any{
		"id": zoneID + "/SOA/@", "name": "@", "type": ns + "/privateDnsZones/SOA", "etag": entra.NewGUID(),
		"properties": map[string]any{"ttl": 3600, "fqdn": zone + ".", "isAutoRegistered": false, "provisioningState": "Succeeded",
			"soaRecord": map[string]any{"email": "azureprivatedns-host.microsoft.com", "expireTime": 2419200, "host": "azureprivatedns.net",
				"minimumTtl": 10, "refreshTime": 3600, "retryTime": 300, "serialNumber": 1}},
	}
}

func presentZone(s *arm.Server, b map[string]any) map[string]any {
	b = arm.DeepCopy(b)
	p := props(b)
	id := strOf(b["id"])
	n := 0
	for _, rt := range []string{"A", "AAAA", "CNAME", "TXT", "SOA"} {
		n += len(s.Children(id + "/" + rt))
	}
	links := s.Children(id + "/virtualNetworkLinks")
	reg := 0
	for _, l := range links {
		if props(l)["registrationEnabled"] == true {
			reg++
		}
	}
	p["maxNumberOfRecordSets"] = 25000
	p["numberOfRecordSets"] = n
	p["maxNumberOfVirtualNetworkLinks"] = 1000
	p["numberOfVirtualNetworkLinks"] = len(links)
	p["maxNumberOfVirtualNetworkLinksWithRegistration"] = 100
	p["numberOfVirtualNetworkLinksWithRegistration"] = reg
	return b
}

func prepareLink(q *arm.Request, body, existing map[string]any) error {
	p := props(body)
	if !strings.EqualFold(strOf(body["location"]), "global") {
		return arm.BadRequest("InvalidLocation", "The location of a virtual network link must be 'global'.")
	}
	vid := refID(p["virtualNetwork"])
	if _, ok := q.S.LoadResource(vid); !ok {
		return &arm.APIError{Status: 400, Code: "BadRequest", Message: arm.Errorf("Virtual network '%s' was not found.", vid)}
	}
	if _, ok := p["registrationEnabled"]; !ok {
		p["registrationEnabled"] = false
	}
	zone := q.ID.ParentPath(ns, q.Reg.Types)
	for _, l := range q.S.Children(zone + "/virtualNetworkLinks") {
		if !strings.EqualFold(strOf(l["name"]), q.ID.Name()) && strings.EqualFold(refID(props(l)["virtualNetwork"]), vid) {
			return &arm.APIError{Status: 409, Code: "Conflict", Message: arm.Errorf("A virtual network cannot be linked to the same private DNS zone more than once. Existing link: %s.", l["name"])}
		}
	}
	p["virtualNetworkLinkState"] = "Completed"
	if strOf(p["resolutionPolicy"]) == "" {
		p["resolutionPolicy"] = "Default"
	}
	return nil
}

func prepareRecord(q *arm.Request, body map[string]any, rt string) error {
	p := props(body)
	zone := q.ID.Names[0]
	name := q.ID.Name()
	if rt == "SOA" && name != "@" {
		return arm.BadRequest("BadRequest", "The only SOA record set name allowed is '@'.")
	}
	if p["ttl"] == nil {
		p["ttl"] = 3600
	}
	if name == "@" {
		p["fqdn"] = zone + "."
	} else {
		p["fqdn"] = name + "." + zone + "."
	}
	p["isAutoRegistered"] = false
	if rt == "A" {
		for _, r := range entra.ObjectList(p["aRecords"]) {
			if _, err := netip.ParseAddr(strOf(r["ipv4Address"])); err != nil {
				return arm.BadRequest("BadRequest", arm.Errorf("'%v' is not a valid IPv4 address.", r["ipv4Address"]))
			}
		}
	}
	return nil
}
