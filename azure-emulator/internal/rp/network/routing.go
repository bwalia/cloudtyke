package network

// Routing and packet-path analysis for learning labs, modelled on how Azure
// actually routes traffic between subnets:
//
//   - every subnet gets system routes: its VNet's address space (VnetLocal),
//     the address space of each Connected peering (VNetPeering), 0.0.0.0/0
//     (Internet) and the private ranges 10/8, 172.16/12, 192.168/16 and
//     100.64/10 with next hop None, so traffic to a network you are not
//     connected to is dropped;
//   - user-defined routes from the subnet's route table override system
//     routes with the same prefix, and the longest prefix always wins;
//   - peering is not transitive: a peered VNet's own peerings are not routes;
//   - traffic forwarded by an appliance (NVA / firewall) only enters a peered
//     VNet when that VNet's peering allows forwarded traffic;
//   - NSGs are evaluated outbound at the source subnet and inbound at the
//     destination subnet, lowest priority number first, with the default rules.
//
// Trace follows a packet hop by hop and explains every decision.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/arm"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

type netModel struct {
	vnets     map[string]*vnetM // by lower-case id
	subnets   []*subnetM
	connected map[string][]string    // Virtual Network Manager connected groups
	admin     map[string][]adminRule // active security admin rules by VNet
}

type vnetM struct {
	id, name string
	prefixes []netip.Prefix
	subnets  []*subnetM
	peerings []*peerM
}

type subnetM struct {
	id, name string
	prefixes []netip.Prefix
	nsg, rt  string
	vnet     *vnetM
}

type peerM struct {
	name, remote string
	vnetAccess   bool
	forwarded    bool
}

// Route is one effective route of a subnet.
type Route struct {
	Source        string `json:"source"` // Default (system) or User
	State         string `json:"state"`  // Active or Invalid (overridden)
	AddressPrefix string `json:"addressPrefix"`
	NextHopType   string `json:"nextHopType"`
	NextHopIP     string `json:"nextHopIpAddress,omitempty"`
	Name          string `json:"name,omitempty"` // UDR name / peering
	prefix        netip.Prefix
	remote        string
}

func loadModel(s *arm.Server) *netModel {
	m := &netModel{vnets: map[string]*vnetM{}}
	m.connected, m.admin = avnmState(s)
	type rawPeer struct {
		vnet string
		doc  map[string]any
	}
	var subs []map[string]any
	var peers []rawPeer
	_ = s.Store.Scan(store.BucketARM, "/subscriptions/", func(k string, v []byte) bool {
		if !strings.Contains(k, "/providers/microsoft.network/virtualnetworks/") {
			return true
		}
		var d map[string]any
		if json.Unmarshal(v, &d) != nil {
			return true
		}
		switch strings.ToLower(strOf(d["type"])) {
		case "microsoft.network/virtualnetworks":
			pfx, _ := parsePrefixes(vnetPrefixes(props(d)))
			m.vnets[strings.ToLower(strOf(d["id"]))] = &vnetM{id: strOf(d["id"]), name: strOf(d["name"]), prefixes: pfx}
		case "microsoft.network/virtualnetworks/subnets":
			subs = append(subs, d)
		case "microsoft.network/virtualnetworks/virtualnetworkpeerings":
			id := strOf(d["id"])
			peers = append(peers, rawPeer{vnet: strings.ToLower(id[:strings.Index(strings.ToLower(id), "/virtualnetworkpeerings/")]), doc: d})
		}
		return true
	})
	for _, d := range subs {
		id := strOf(d["id"])
		v := m.vnets[strings.ToLower(id[:strings.Index(strings.ToLower(id), "/subnets/")])]
		if v == nil {
			continue
		}
		p := props(d)
		pfx, _ := parsePrefixes(subnetPrefixList(p))
		sn := &subnetM{id: id, name: strOf(d["name"]), prefixes: pfx, nsg: refID(p["networkSecurityGroup"]), rt: refID(p["routeTable"]), vnet: v}
		v.subnets = append(v.subnets, sn)
		m.subnets = append(m.subnets, sn)
	}
	for _, rp := range peers {
		v := m.vnets[rp.vnet]
		if v == nil {
			continue
		}
		p := props(rp.doc)
		v.peerings = append(v.peerings, &peerM{name: strOf(rp.doc["name"]), remote: strings.ToLower(refID(p["remoteVirtualNetwork"])),
			vnetAccess: p["allowVirtualNetworkAccess"] != false, forwarded: p["allowForwardedTraffic"] == true})
	}
	return m
}

// peerTo returns v's peering to remote and whether it is Connected (remote peers back).
func (m *netModel) peerTo(v *vnetM, remote string) (*peerM, bool) {
	for _, p := range v.peerings {
		if p.remote == strings.ToLower(remote) {
			r := m.vnets[p.remote]
			if r == nil {
				return p, false
			}
			for _, back := range r.peerings {
				if back.remote == strings.ToLower(v.id) {
					return p, true
				}
			}
			return p, false
		}
	}
	return nil, false
}

func (m *netModel) subnetFor(ip netip.Addr) *subnetM {
	for _, s := range m.subnets {
		for _, p := range s.prefixes {
			if p.Contains(ip) {
				return s
			}
		}
	}
	return nil
}

func (m *netModel) subnetByID(id string) *subnetM {
	for _, s := range m.subnets {
		if strings.EqualFold(s.id, id) {
			return s
		}
	}
	return nil
}

var reservedNone = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10"}

// effectiveRoutes returns a subnet's routes the way Azure computes them.
func (m *netModel) effectiveRoutes(s *arm.Server, sn *subnetM) []Route {
	var sys []Route
	for _, p := range sn.vnet.prefixes {
		sys = append(sys, Route{Source: "Default", AddressPrefix: p.String(), NextHopType: "VnetLocal", prefix: p})
	}
	for _, pr := range sn.vnet.peerings {
		if _, ok := m.peerTo(sn.vnet, pr.remote); !ok {
			continue // only Connected peerings program routes
		}
		for _, p := range m.vnets[pr.remote].prefixes {
			sys = append(sys, Route{Source: "Default", AddressPrefix: p.String(), NextHopType: "VNetPeering", Name: pr.name, prefix: p, remote: pr.remote})
		}
	}
	peered := map[string]bool{}
	for _, pr := range sn.vnet.peerings {
		peered[pr.remote] = true
	}
	for _, other := range m.connected[strings.ToLower(sn.vnet.id)] {
		v := m.vnets[other]
		if v == nil || peered[other] {
			continue
		}
		for _, p := range v.prefixes {
			sys = append(sys, Route{Source: "Default", AddressPrefix: p.String(), NextHopType: "ConnectedGroup", Name: v.name, prefix: p, remote: other})
		}
	}
	def := netip.MustParsePrefix("0.0.0.0/0")
	sys = append(sys, Route{Source: "Default", AddressPrefix: "0.0.0.0/0", NextHopType: "Internet", prefix: def})
	for _, r := range reservedNone {
		p := netip.MustParsePrefix(r)
		sys = append(sys, Route{Source: "Default", AddressPrefix: r, NextHopType: "None", prefix: p})
	}
	var user []Route
	if sn.rt != "" {
		for _, r := range s.Children(sn.rt + "/routes") {
			rp := props(r)
			p, err := netip.ParsePrefix(strOf(rp["addressPrefix"]))
			if err != nil {
				continue
			}
			user = append(user, Route{Source: "User", AddressPrefix: p.String(), NextHopType: strOf(rp["nextHopType"]), NextHopIP: strOf(rp["nextHopIpAddress"]), Name: strOf(r["name"]), prefix: p})
		}
	}
	// a user route replaces a system route with the same prefix
	userPfx := map[netip.Prefix]bool{}
	for _, u := range user {
		userPfx[u.prefix] = true
	}
	var out []Route
	for _, r := range sys {
		r.State = "Active"
		if userPfx[r.prefix] {
			r.State = "Invalid"
		}
		out = append(out, r)
	}
	for _, u := range user {
		u.State = "Active"
		out = append(out, u)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].prefix.Bits() != out[j].prefix.Bits() {
			return out[i].prefix.Bits() > out[j].prefix.Bits()
		}
		return out[i].Source == "User" && out[j].Source != "User"
	})
	return out
}

// lookup picks the active route with the longest matching prefix.
func lookup(routes []Route, ip netip.Addr) (Route, bool) {
	for _, r := range routes {
		if r.State == "Active" && r.prefix.Contains(ip) {
			return r, true
		}
	}
	return Route{}, false
}

// ------------------------------------------------------------ NSG evaluation

// RuleHit is the NSG rule that decided a packet.
type RuleHit struct {
	NSG       string `json:"nsg"`
	Rule      string `json:"rule"`
	Priority  int    `json:"priority"`
	Access    string `json:"access"`
	Direction string `json:"direction"`
	Default   bool   `json:"default"`
}

func (m *netModel) vnetTag(s *arm.Server, sn *subnetM) []netip.Prefix {
	out := append([]netip.Prefix{}, sn.vnet.prefixes...)
	for _, pr := range sn.vnet.peerings {
		if _, ok := m.peerTo(sn.vnet, pr.remote); ok {
			out = append(out, m.vnets[pr.remote].prefixes...)
		}
	}
	// Virtual Network Manager connected groups are reachable like peered VNets
	for _, other := range m.connected[strings.ToLower(sn.vnet.id)] {
		if v := m.vnets[other]; v != nil {
			out = append(out, v.prefixes...)
		}
	}
	// Azure's VirtualNetwork tag also covers prefixes used in the subnet's UDRs
	for _, r := range m.effectiveRoutes(s, sn) {
		if r.Source == "User" && r.prefix.Bits() > 0 {
			out = append(out, r.prefix)
		}
	}
	return out
}

func matchAddr(s *arm.Server, m *netModel, sn *subnetM, spec string, ip netip.Addr) bool {
	switch strings.ToLower(spec) {
	case "*", "any", "0.0.0.0/0":
		return true
	case "virtualnetwork":
		for _, p := range m.vnetTag(s, sn) {
			if p.Contains(ip) {
				return true
			}
		}
		return false
	case "internet":
		for _, r := range reservedNone {
			if netip.MustParsePrefix(r).Contains(ip) {
				return false
			}
		}
		return true
	case "azureloadbalancer":
		return ip == netip.MustParseAddr("168.63.129.16")
	}
	if p, err := netip.ParsePrefix(spec); err == nil {
		return p.Contains(ip)
	}
	if a, err := netip.ParseAddr(spec); err == nil {
		return a == ip
	}
	return false
}

func matchPort(spec string, port int) bool {
	if spec == "*" || spec == "" {
		return true
	}
	lo, hi, ok := strings.Cut(spec, "-")
	a, err := strconv.Atoi(strings.TrimSpace(lo))
	if err != nil {
		return false
	}
	if !ok {
		return a == port
	}
	b, err := strconv.Atoi(strings.TrimSpace(hi))
	return err == nil && port >= a && port <= b
}

func listOr(p map[string]any, many, one string) []string {
	if l := strList(p[many]); len(l) > 0 {
		return l
	}
	if v := strOf(p[one]); v != "" {
		return []string{v}
	}
	return []string{"*"}
}

func (m *netModel) evalNSG(s *arm.Server, sn *subnetM, dir, proto string, src, dst netip.Addr, port int) (*RuleHit, bool) {
	if sn == nil || sn.nsg == "" {
		return nil, true
	}
	nsg, ok := s.LoadResource(sn.nsg)
	if !ok {
		return nil, true
	}
	type rule struct {
		name    string
		p       map[string]any
		prio    int
		builtin bool
	}
	var rules []rule
	for _, r := range s.Children(sn.nsg + "/securityRules") {
		p := props(r)
		n, _ := p["priority"].(float64)
		rules = append(rules, rule{strOf(r["name"]), p, int(n), false})
	}
	for _, d := range defaultRules {
		rules = append(rules, rule{d.name, map[string]any{"direction": d.dir, "access": d.access, "protocol": "*", "sourceAddressPrefix": d.src, "destinationAddressPrefix": d.dst, "destinationPortRange": "*"}, d.prio, true})
	}
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].prio < rules[j].prio })
	for _, r := range rules {
		if !strings.EqualFold(strOf(r.p["direction"]), dir) {
			continue
		}
		if pr := strOf(r.p["protocol"]); pr != "*" && !strings.EqualFold(pr, proto) {
			continue
		}
		hit := false
		for _, a := range listOr(r.p, "sourceAddressPrefixes", "sourceAddressPrefix") {
			if matchAddr(s, m, sn, a, src) {
				hit = true
			}
		}
		if !hit {
			continue
		}
		hit = false
		for _, a := range listOr(r.p, "destinationAddressPrefixes", "destinationAddressPrefix") {
			if matchAddr(s, m, sn, a, dst) {
				hit = true
			}
		}
		if !hit {
			continue
		}
		if !strings.EqualFold(proto, "Icmp") {
			hit = false
			for _, pr := range listOr(r.p, "destinationPortRanges", "destinationPortRange") {
				if matchPort(pr, port) {
					hit = true
				}
			}
			if !hit {
				continue
			}
		}
		h := &RuleHit{NSG: strOf(nsg["name"]), Rule: r.name, Priority: r.prio, Access: strOf(r.p["access"]), Direction: dir, Default: r.builtin}
		return h, strings.EqualFold(h.Access, "Allow")
	}
	return nil, true
}

// security applies Virtual Network Manager security admin rules, then the
// subnet NSG (unless an admin rule said AlwaysAllow). It returns the hops
// to record and whether the packet may pass.
func (m *netModel) security(s *arm.Server, sn *subnetM, dir, proto string, src, dst netip.Addr, port int) ([]Hop, bool) {
	var hops []Hop
	for _, r := range m.admin[strings.ToLower(sn.vnet.id)] {
		if !strings.EqualFold(strOf(r.p["direction"]), dir) {
			continue
		}
		if pr := strOf(r.p["protocol"]); pr != "Any" && !strings.EqualFold(pr, proto) {
			continue
		}
		match := func(key string, ip netip.Addr) bool {
			list := entraObjects(r.p[key])
			if len(list) == 0 {
				return true
			}
			for _, a := range list {
				if matchAddr(s, m, sn, strOf(a["addressPrefix"]), ip) {
					return true
				}
			}
			return false
		}
		if !match("sources", src) || !match("destinations", dst) {
			continue
		}
		if ports := strList(r.p["destinationPortRanges"]); len(ports) > 0 && !strings.EqualFold(proto, "Icmp") {
			ok := false
			for _, pr := range ports {
				if matchPort(pr, port) {
					ok = true
				}
			}
			if !ok {
				continue
			}
		}
		access := strOf(r.p["access"])
		hit := &RuleHit{NSG: "admin: " + r.manager + "/" + r.collection, Rule: r.name, Priority: r.prio, Access: access, Direction: dir}
		switch access {
		case "Deny":
			return append(hops, Hop{Where: where(sn), Action: "Security admin " + strings.ToLower(dir), Detail: fmt.Sprintf("Denied by security admin rule %s (priority %d) from network manager %s. Admin rules are evaluated before NSGs, so no NSG can allow this.", r.name, r.prio, r.manager), Rule: hit, Verdict: "blocked"}), false
		case "AlwaysAllow":
			return append(hops, Hop{Where: where(sn), Action: "Security admin " + strings.ToLower(dir), Detail: fmt.Sprintf("AlwaysAllow by security admin rule %s (priority %d) from network manager %s: the NSG is skipped, even if it would deny.", r.name, r.prio, r.manager), Rule: hit, Verdict: "ok"}), true
		default:
			hops = append(hops, Hop{Where: where(sn), Action: "Security admin " + strings.ToLower(dir), Detail: fmt.Sprintf("Allowed by security admin rule %s (priority %d); the subnet NSG still decides.", r.name, r.prio), Rule: hit, Verdict: "ok"})
		}
		break
	}
	if hit, ok := m.evalNSG(s, sn, dir, proto, src, dst, port); !ok {
		return append(hops, Hop{Where: where(sn), Action: "NSG " + strings.ToLower(dir), Detail: fmt.Sprintf("Denied by rule %s (priority %d) of %s.", hit.Rule, hit.Priority, hit.NSG), Rule: hit, Verdict: "blocked"}), false
	} else if hit != nil {
		hops = append(hops, Hop{Where: where(sn), Action: "NSG " + strings.ToLower(dir), Detail: fmt.Sprintf("Allowed by rule %s (priority %d) of %s.", hit.Rule, hit.Priority, hit.NSG), Rule: hit, Verdict: "ok"})
	}
	return hops, true
}

func ruleLabel(h *RuleHit) string {
	if h == nil {
		return "a rule"
	}
	if strings.HasPrefix(h.NSG, "admin: ") {
		return "security admin rule " + h.Rule
	}
	return "NSG rule " + h.Rule
}

// ------------------------------------------------------------ trace

// Hop is one step of a trace.
type Hop struct {
	Step     int      `json:"step"`
	Where    string   `json:"where"` // vnet/subnet
	Action   string   `json:"action"`
	Detail   string   `json:"detail"`
	Route    *Route   `json:"route,omitempty"`
	Rule     *RuleHit `json:"rule,omitempty"`
	Verdict  string   `json:"verdict"` // ok, blocked, dropped, egress
	Location string   `json:"location,omitempty"`
}

// TraceResult explains whether a packet gets from src to dst.
type TraceResult struct {
	Source      map[string]any `json:"source"`
	Destination map[string]any `json:"destination"`
	Result      string         `json:"result"` // Delivered, Blocked, Dropped, Internet, Unknown
	Summary     string         `json:"summary"`
	Hops        []Hop          `json:"hops"`
}

func where(sn *subnetM) string { return sn.vnet.name + "/" + sn.name }

// Trace follows a packet from srcIP to dstIP:port. srcSubnet (optional)
// pins the source subnet when the same address exists in several VNets.
func Trace(s *arm.Server, srcIP, dstIP netip.Addr, proto string, port int, srcSubnet string) TraceResult {
	m := loadModel(s)
	res := TraceResult{Source: map[string]any{"ip": srcIP.String()}, Destination: map[string]any{"ip": dstIP.String(), "protocol": proto, "port": port}}
	add := func(h Hop) { h.Step = len(res.Hops) + 1; res.Hops = append(res.Hops, h) }
	finish := func(result, summary string) TraceResult { res.Result, res.Summary = result, summary; return res }

	var src *subnetM
	if srcSubnet != "" {
		src = m.subnetByID(srcSubnet)
		if src == nil {
			return finish("Unknown", "Source subnet "+srcSubnet+" was not found.")
		}
	} else {
		var matches []*subnetM
		for _, sn := range m.subnets {
			for _, p := range sn.prefixes {
				if p.Contains(srcIP) {
					matches = append(matches, sn)
				}
			}
		}
		if len(matches) == 0 {
			return finish("Unknown", fmt.Sprintf("%s is not inside any subnet you can see.", srcIP))
		}
		if len(matches) > 1 {
			var names []string
			for _, mt := range matches {
				names = append(names, where(mt))
			}
			return finish("Unknown", fmt.Sprintf("%s exists in %d subnets (%s): unconnected VNets may reuse addresses. Pass fromSubnet=<subnet id> to choose.", srcIP, len(matches), strings.Join(names, ", ")))
		}
		src = matches[0]
	}
	res.Source["subnet"], res.Source["vnet"] = src.name, src.vnet.name
	if d := m.subnetFor(dstIP); d != nil {
		res.Destination["subnet"], res.Destination["vnet"] = d.name, d.vnet.name
	}
	add(Hop{Where: where(src), Action: "Start", Detail: fmt.Sprintf("Packet leaves %s in subnet %s (%s) for %s %s:%d.", srcIP, src.name, src.vnet.name, proto, dstIP, port), Verdict: "ok"})

	// no route at all (next hop None) is reported before NSGs, like Network Watcher
	if first, ok := lookup(m.effectiveRoutes(s, src), dstIP); ok && first.NextHopType == "None" {
		goto route
	}
	if hops, ok := m.security(s, src, "Outbound", proto, srcIP, dstIP, port); !ok {
		for _, h := range hops {
			add(h)
		}
		last := hops[len(hops)-1]
		return finish("Blocked", fmt.Sprintf("Blocked leaving %s by %s.", src.name, ruleLabel(last.Rule)))
	} else {
		for _, h := range hops {
			add(h)
		}
	}

route:
	cur := src
	forwarded := false
	seen := map[string]bool{}
	for i := 0; i < 8; i++ {
		r, ok := lookup(m.effectiveRoutes(s, cur), dstIP)
		if !ok {
			return finish("Dropped", "No route matched.")
		}
		rc := r
		switch r.NextHopType {
		case "VnetLocal":
			add(Hop{Where: where(cur), Action: "Route", Detail: fmt.Sprintf("%s matches %s (system route for the VNet's own address space): deliver inside %s.", dstIP, r.AddressPrefix, cur.vnet.name), Route: &rc, Verdict: "ok"})
			return m.deliver(s, &res, add, finish, cur.vnet, srcIP, dstIP, proto, port)
		case "VNetPeering":
			remote := m.vnets[r.remote]
			add(Hop{Where: where(cur), Action: "Route", Detail: fmt.Sprintf("%s matches %s, programmed by peering %s: send to %s.", dstIP, r.AddressPrefix, r.Name, remote.name), Route: &rc, Verdict: "ok"})
			if h, ok := m.crossPeering(cur.vnet, remote, forwarded); !ok {
				add(h)
				return finish("Dropped", h.Detail)
			}
			return m.deliver(s, &res, add, finish, remote, srcIP, dstIP, proto, port)
		case "ConnectedGroup":
			remote := m.vnets[r.remote]
			add(Hop{Where: where(cur), Action: "Route", Detail: fmt.Sprintf("%s matches %s with next hop ConnectedGroup: Virtual Network Manager connects %s and %s directly (no peering resource).", dstIP, r.AddressPrefix, cur.vnet.name, remote.name), Route: &rc, Verdict: "ok"})
			return m.deliver(s, &res, add, finish, remote, srcIP, dstIP, proto, port)
		case "VirtualAppliance":
			nva, _ := netip.ParseAddr(r.NextHopIP)
			add(Hop{Where: where(cur), Action: "Route", Detail: fmt.Sprintf("%s matches user route %s (%s): next hop is the appliance at %s.", dstIP, r.Name, r.AddressPrefix, r.NextHopIP), Route: &rc, Verdict: "ok"})
			if seen[r.NextHopIP+"|"+cur.id] {
				add(Hop{Where: where(cur), Action: "Loop", Detail: "The appliance's subnet routes the packet straight back to the same appliance: routing loop.", Verdict: "dropped"})
				return finish("Dropped", "Routing loop between user-defined routes.")
			}
			seen[r.NextHopIP+"|"+cur.id] = true
			nsn := m.subnetFor(nva)
			if nsn == nil {
				add(Hop{Where: where(cur), Action: "Next hop", Detail: fmt.Sprintf("No subnet contains %s, so the next hop is unreachable and Azure drops the packet.", nva), Verdict: "dropped"})
				return finish("Dropped", "The UDR points at an appliance IP that does not exist.")
			}
			if nsn.vnet != cur.vnet {
				if rr, ok := lookup(m.effectiveRoutes(s, cur), nva); !ok || rr.NextHopType != "VNetPeering" {
					add(Hop{Where: where(cur), Action: "Next hop", Detail: fmt.Sprintf("The appliance %s is in %s, but %s has no Connected peering to it.", nva, nsn.vnet.name, cur.vnet.name), Verdict: "dropped"})
					return finish("Dropped", "The appliance is in a VNet this subnet is not peered with.")
				}
				if h, ok := m.crossPeering(cur.vnet, nsn.vnet, forwarded); !ok {
					add(h)
					return finish("Dropped", h.Detail)
				}
			}
			add(Hop{Where: where(nsn), Action: "Appliance", Detail: fmt.Sprintf("The appliance at %s (%s) inspects and forwards the packet (IP forwarding on). From here it is forwarded traffic.", nva, where(nsn)), Verdict: "ok", Location: nva.String()})
			cur, forwarded = nsn, true
		case "Internet":
			add(Hop{Where: where(cur), Action: "Route", Detail: fmt.Sprintf("%s only matches the default route %s: it leaves Azure to the internet.", dstIP, r.AddressPrefix), Route: &rc, Verdict: "egress"})
			return finish("Internet", "Sent to the internet through the default route.")
		case "None":
			why := "a user route with next hop None (black hole)"
			if r.Source == "Default" {
				why = "Azure's built-in route for private address ranges, used when no VNet, peering or user route covers the destination"
			}
			add(Hop{Where: where(cur), Action: "Route", Detail: fmt.Sprintf("%s matches %s with next hop None: %s. The packet is dropped.", dstIP, r.AddressPrefix, why), Route: &rc, Verdict: "dropped"})
			return finish("Dropped", fmt.Sprintf("No path: %s from %s matches %s (next hop None).", dstIP, where(cur), r.AddressPrefix))
		case "VirtualNetworkGateway":
			add(Hop{Where: where(cur), Action: "Route", Detail: "Next hop is a VPN / ExpressRoute gateway, which CloudTyke does not emulate yet.", Route: &rc, Verdict: "egress"})
			return finish("Unknown", "Sent to a virtual network gateway (not emulated).")
		default:
			return finish("Unknown", "Unsupported next hop type "+r.NextHopType)
		}
	}
	return finish("Dropped", "Too many hops.")
}

// crossPeering checks the peering rules for traffic moving from a to b.
func (m *netModel) crossPeering(a, b *vnetM, forwarded bool) (Hop, bool) {
	out, ok := m.peerTo(a, b.id)
	if !ok || out == nil {
		return Hop{Where: a.name, Action: "Peering", Detail: fmt.Sprintf("%s and %s are not peered in both directions.", a.name, b.name), Verdict: "dropped"}, false
	}
	in, _ := m.peerTo(b, a.id)
	if !out.vnetAccess || (in != nil && !in.vnetAccess) {
		return Hop{Where: a.name, Action: "Peering", Detail: fmt.Sprintf("allowVirtualNetworkAccess is off on the %s / %s peering.", a.name, b.name), Verdict: "dropped"}, false
	}
	if forwarded && (in == nil || !in.forwarded) {
		name := "back to " + a.name
		if in != nil {
			name = in.name
		}
		return Hop{Where: b.name, Action: "Peering", Detail: fmt.Sprintf("The packet was forwarded by an appliance, but %s's peering %s does not allow forwarded traffic (allowForwardedTraffic=false), so %s drops it.", b.name, name, b.name), Verdict: "dropped"}, false
	}
	detail := fmt.Sprintf("Crosses peering %s -> %s (Connected).", a.name, b.name)
	if forwarded {
		detail += fmt.Sprintf(" %s accepts forwarded traffic on peering %s.", b.name, in.name)
	}
	return Hop{Where: a.name + " -> " + b.name, Action: "Peering", Detail: detail, Verdict: "ok"}, true
}

func (m *netModel) deliver(s *arm.Server, res *TraceResult, add func(Hop), finish func(string, string) TraceResult, v *vnetM, src, dst netip.Addr, proto string, port int) TraceResult {
	var d *subnetM
	for _, sn := range v.subnets {
		for _, p := range sn.prefixes {
			if p.Contains(dst) {
				d = sn
			}
		}
	}
	if d == nil {
		add(Hop{Where: v.name, Action: "Deliver", Detail: fmt.Sprintf("No subnet in %s contains %s.", v.name, dst), Verdict: "dropped"})
		return finish("Dropped", "The destination address is not in any subnet.")
	}
	hops, ok := m.security(s, d, "Inbound", proto, src, dst, port)
	for _, h := range hops {
		add(h)
	}
	if !ok {
		return finish("Blocked", fmt.Sprintf("Reached %s but %s denied it.", where(d), ruleLabel(hops[len(hops)-1].Rule)))
	}
	add(Hop{Where: where(d), Action: "Delivered", Detail: fmt.Sprintf("Delivered to %s in subnet %s.", dst, d.name), Verdict: "ok"})
	return finish("Delivered", fmt.Sprintf("%s reaches %s:%d in %s.", src, dst, port, where(d)))
}

// ------------------------------------------------------------ HTTP (admin API)

// LabHandler serves the network lab tools under /_cloudtyke/network/:
//
//	GET routes?subnet=<subnet id>  or  ?ip=<address>   effective routes
//	GET trace?from=<ip>&to=<ip>&port=443&protocol=Tcp[&fromSubnet=<id>]   hop-by-hop path
func LabHandler(s *arm.Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		write := func(code int, v any) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			_ = enc.Encode(v)
		}
		bad := func(msg string) { write(400, map[string]any{"error": msg}) }
		switch strings.TrimPrefix(r.URL.Path, "/_cloudtyke/network/") {
		case "routes":
			m := loadModel(s)
			var sn *subnetM
			if id := q.Get("subnet"); id != "" {
				sn = m.subnetByID(id)
			} else if ip, err := netip.ParseAddr(q.Get("ip")); err == nil {
				sn = m.subnetFor(ip)
			} else {
				bad("pass ?subnet=<subnet resource id> or ?ip=<address>")
				return
			}
			if sn == nil {
				write(404, map[string]any{"error": "subnet not found"})
				return
			}
			write(200, map[string]any{"subnet": sn.id, "vnet": sn.vnet.name, "name": sn.name, "value": m.effectiveRoutes(s, sn)})
		case "trace":
			from, err1 := netip.ParseAddr(q.Get("from"))
			to, err2 := netip.ParseAddr(q.Get("to"))
			if err1 != nil || err2 != nil {
				bad("pass ?from=<source ip>&to=<destination ip>[&port=443&protocol=Tcp]")
				return
			}
			port := 443
			if p, err := strconv.Atoi(q.Get("port")); err == nil {
				port = p
			}
			proto := q.Get("protocol")
			switch strings.ToLower(proto) {
			case "", "tcp":
				proto = "Tcp"
			case "udp":
				proto = "Udp"
			case "icmp":
				proto = "Icmp"
			default:
				bad("protocol must be Tcp, Udp or Icmp")
				return
			}
			write(200, Trace(s, from, to, proto, port, q.Get("fromSubnet")))
		default:
			write(404, map[string]any{"error": "use /_cloudtyke/network/routes or /_cloudtyke/network/trace"})
		}
	})
}
