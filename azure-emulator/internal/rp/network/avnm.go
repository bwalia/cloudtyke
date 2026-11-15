package network

// Azure Virtual Network Manager (AVNM): manage connectivity and security
// for many VNets across subscriptions and regions from one place.
//
//   networkManagers                        scope (subscriptions / management groups) + features
//     networkGroups / staticMembers        which VNets a configuration applies to
//     connectivityConfigurations           HubAndSpoke (peers every member with the hub) or Mesh,
//                                          optionally "directly connected" spokes and global mesh
//     securityAdminConfigurations
//       ruleCollections / rules            org-wide rules evaluated before NSGs:
//                                          Deny, Allow (NSG still applies) or AlwaysAllow (skips NSGs)
//   POST networkManagers/{nm}/commit       deploy configurations to regions (goal state per region)
//   POST .../listDeploymentStatus          per region / type status, as the portal shows
//
// Nothing happens until a commit: like Azure, configurations are inert until
// deployed to a region, and only member VNets in a deployed region are affected.
// Hub-and-spoke creates real peerings named ANM_*; connected groups program
// ConnectedGroup routes (no peering resources), visible in effective routes.

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/arm"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

const nmType = ns + "/networkManagers"

func registerAVNM(s *arm.Server) {
	reg := func(types []string, notes string, o arm.GenericOptions) {
		s.Registry.Register(&arm.TypeRegistration{Namespace: ns, Types: types, APIVersions: APIVersions, Scopes: []arm.ScopeKind{arm.KindResourceGroup}, Phase: 2, Notes: notes, Handler: arm.Generic(o)})
	}
	hidden := func(q *arm.Request, b map[string]any) map[string]any {
		b = arm.DeepCopy(b)
		p := props(b)
		for k := range p {
			if strings.HasPrefix(k, "_") {
				delete(p, k)
			}
		}
		return b
	}

	reg([]string{"networkManagers"}, "Virtual Network Manager: subscription / management group scopes, scope accesses (Connectivity, SecurityAdmin, Routing), commit to regions, deployment status, active configurations",
		arm.GenericOptions{Tracked: true, Async: true, Present: hidden,
			Prepare: func(q *arm.Request, body, existing map[string]any) error {
				p := props(body)
				sc, _ := p["networkManagerScopes"].(map[string]any)
				subs, mgs := strList(sc["subscriptions"]), strList(sc["managementGroups"])
				if len(subs)+len(mgs) == 0 {
					return arm.BadRequest("InvalidNetworkManagerScope", "A network manager needs at least one subscription or management group in properties.networkManagerScopes.")
				}
				for _, sid := range subs {
					parts := strings.Split(strings.Trim(sid, "/"), "/")
					if len(parts) != 2 || !strings.EqualFold(parts[0], "subscriptions") {
						return arm.BadRequest("InvalidNetworkManagerScope", arm.Errorf("Scope '%s' must look like /subscriptions/{subscriptionId}.", sid))
					}
					if _, ok := q.S.H.GetSubscription(parts[1]); !ok {
						return arm.BadRequest("InvalidNetworkManagerScope", arm.Errorf("Subscription '%s' in the network manager scope was not found.", parts[1]))
					}
				}
				for _, a := range strList(p["networkManagerScopeAccesses"]) {
					if a != "Connectivity" && a != "SecurityAdmin" && a != "Routing" {
						return arm.BadRequest("InvalidScopeAccess", arm.Errorf("Scope access '%s' is not valid. Allowed: Connectivity, SecurityAdmin, Routing.", a))
					}
				}
				if existing != nil {
					for k, v := range props(existing) {
						if strings.HasPrefix(k, "_") {
							p[k] = v
						}
					}
				}
				return nil
			},
			OnDelete: func(q *arm.Request, existing map[string]any) error {
				if d, _ := props(existing)["_deployments"].(map[string]any); len(d) > 0 {
					return arm.BadRequest("CannotDeleteNetworkManagerWithActiveDeployment", "The network manager has active deployments. Commit an empty set of configurations to every region first (destroy the deployments).")
				}
				return nil
			},
			Actions: map[string]arm.Handler{
				"commit":                               commit,
				"listDeploymentStatus":                 listDeploymentStatus,
				"listActiveConnectivityConfigurations": func(q *arm.Request) { listActive(q, "Connectivity") },
				"listActiveSecurityAdminRules":         func(q *arm.Request) { listActive(q, "SecurityAdmin") },
			},
		})

	reg([]string{"networkManagers", "networkGroups"}, "groups of VNets that configurations apply to (static membership)",
		arm.GenericOptions{Prepare: func(q *arm.Request, body, existing map[string]any) error {
			p := props(body)
			if mt := strOf(p["memberType"]); mt == "" {
				p["memberType"] = "VirtualNetwork"
			} else if mt != "VirtualNetwork" && mt != "Subnet" {
				return arm.BadRequest("InvalidMemberType", "memberType must be VirtualNetwork or Subnet.")
			}
			return nil
		}, OnDelete: func(q *arm.Request, existing map[string]any) error {
			if users := referencedBy(q.S, strOf(existing["id"])); len(users) > 0 {
				return arm.BadRequest("NetworkGroupInUse", arm.Errorf("Network group %s is used by %s. Remove it from those configurations first.", existing["name"], strings.Join(users, ", ")))
			}
			return nil
		}})

	reg([]string{"networkManagers", "networkGroups", "staticMembers"}, "add a VNet to a group: must exist and be inside the network manager's scope; membership changes re-apply deployed configurations",
		arm.GenericOptions{
			Prepare: func(q *arm.Request, body, existing map[string]any) error {
				p := props(body)
				vid := strOf(p["resourceId"])
				v, ok := q.S.LoadResource(vid)
				if !ok || !strings.EqualFold(strOf(v["type"]), ns+"/virtualNetworks") {
					return arm.BadRequest("InvalidResourceId", arm.Errorf("The virtual network '%s' was not found.", vid))
				}
				nm, ok := q.S.LoadResource(managerOf(q.ID.Path(ns, q.Reg.Types)))
				if !ok || !inScope(nm, vid) {
					return arm.BadRequest("ResourceNotInNetworkManagerScope", arm.Errorf("Virtual network %s is not in the scope of network manager %s. Add its subscription to properties.networkManagerScopes.", v["name"], nm["name"]))
				}
				p["region"] = v["location"]
				return nil
			},
			AfterWrite: func(q *arm.Request, body map[string]any) { reconcile(q.S, managerOf(strOf(body["id"]))) },
		})

	reg([]string{"networkManagers", "connectivityConfigurations"}, "HubAndSpoke or Mesh topology, directly connected groups, global mesh, hub gateway use; overlapping address spaces fail the deployment",
		arm.GenericOptions{Prepare: prepareConnectivity, OnDelete: notDeployed})

	reg([]string{"networkManagers", "securityAdminConfigurations"}, "security admin configuration: org-wide rules evaluated before NSGs",
		arm.GenericOptions{OnDelete: notDeployed})
	reg([]string{"networkManagers", "securityAdminConfigurations", "ruleCollections"}, "rule collections target network groups",
		arm.GenericOptions{Prepare: func(q *arm.Request, body, existing map[string]any) error {
			nm := managerOf(q.ID.Path(ns, q.Reg.Types))
			groups := entraObjects(props(body)["appliesToGroups"])
			if len(groups) == 0 {
				return arm.BadRequest("InvalidRuleCollection", "appliesToGroups must list at least one network group.")
			}
			for _, g := range groups {
				if err := groupInManager(q, nm, strOf(g["networkGroupId"])); err != nil {
					return err
				}
			}
			return nil
		}})
	reg([]string{"networkManagers", "securityAdminConfigurations", "ruleCollections", "rules"}, "admin rules: Allow, Deny or AlwaysAllow; priority 1-4096 unique per direction; IP prefixes or service tags",
		arm.GenericOptions{Prepare: prepareAdminRule})
}

func entraObjects(v any) []map[string]any { return entra.ObjectList(v) }

// managerOf returns the network manager id for any id below it.
func managerOf(id string) string {
	l := strings.ToLower(id)
	i := strings.Index(l, "/networkmanagers/")
	if i < 0 {
		return ""
	}
	end := i + len("/networkManagers/")
	rest := id[end:]
	if j := strings.Index(rest, "/"); j >= 0 {
		rest = rest[:j]
	}
	return id[:end] + rest
}

func inScope(nm map[string]any, vnetID string) bool {
	sc, _ := props(nm)["networkManagerScopes"].(map[string]any)
	if len(strList(sc["managementGroups"])) > 0 {
		return true // management group scopes cover their subscriptions
	}
	sub := "/subscriptions/" + arm.ScopeSubscription(vnetID)
	for _, s := range strList(sc["subscriptions"]) {
		if strings.EqualFold(strings.TrimSuffix(s, "/"), sub) {
			return true
		}
	}
	return false
}

func groupInManager(q *arm.Request, nm, gid string) error {
	if !strings.EqualFold(managerOf(gid), nm) {
		return arm.BadRequest("InvalidNetworkGroup", arm.Errorf("Network group %s does not belong to this network manager.", gid))
	}
	if _, ok := q.S.LoadResource(gid); !ok {
		return arm.BadRequest("InvalidNetworkGroup", arm.Errorf("Network group %s was not found.", gid))
	}
	return nil
}

func boolish(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true")
	}
	return false
}

func notDeployed(q *arm.Request, existing map[string]any) error {
	nm, _ := q.S.LoadResource(managerOf(strOf(existing["id"])))
	deps, _ := props(nm)["_deployments"].(map[string]any)
	for key, d := range deps {
		for _, cid := range strList(d.(map[string]any)["configurationIds"]) {
			if strings.EqualFold(cid, strOf(existing["id"])) {
				return arm.BadRequest("ConfigurationIsDeployed", arm.Errorf("Configuration %s is deployed (%s). Remove it from that deployment first.", existing["name"], key))
			}
		}
	}
	return nil
}

func prepareConnectivity(q *arm.Request, body, existing map[string]any) error {
	p := props(body)
	nm := managerOf(q.ID.Path(ns, q.Reg.Types))
	topo := strOf(p["connectivityTopology"])
	if topo != "HubAndSpoke" && topo != "Mesh" {
		return arm.BadRequest("InvalidConnectivityTopology", "connectivityTopology must be HubAndSpoke or Mesh.")
	}
	hubs := entraObjects(p["hubs"])
	if topo == "HubAndSpoke" {
		if len(hubs) != 1 {
			return arm.BadRequest("InvalidHub", "A HubAndSpoke configuration needs exactly one hub.")
		}
		if h, ok := q.S.LoadResource(strOf(hubs[0]["resourceId"])); !ok || !strings.EqualFold(strOf(h["type"]), ns+"/virtualNetworks") {
			return arm.BadRequest("InvalidHub", arm.Errorf("The hub virtual network %s was not found.", hubs[0]["resourceId"]))
		}
		hubs[0]["resourceType"] = ns + "/virtualNetworks"
	} else if len(hubs) > 0 {
		return arm.BadRequest("InvalidHub", "A Mesh configuration has no hub.")
	}
	groups := entraObjects(p["appliesToGroups"])
	if len(groups) == 0 {
		return arm.BadRequest("InvalidAppliesToGroups", "appliesToGroups must list at least one network group.")
	}
	for _, g := range groups {
		if err := groupInManager(q, nm, strOf(g["networkGroupId"])); err != nil {
			return err
		}
		gc := strOf(g["groupConnectivity"])
		if gc == "" {
			gc = "None"
		}
		if gc != "None" && gc != "DirectlyConnected" {
			return arm.BadRequest("InvalidGroupConnectivity", "groupConnectivity must be None or DirectlyConnected.")
		}
		if topo == "Mesh" {
			gc = "DirectlyConnected"
		}
		g["groupConnectivity"] = gc
		if g["isGlobal"] == nil {
			g["isGlobal"] = "False"
		}
		if g["useHubGateway"] == nil {
			g["useHubGateway"] = "False"
		}
	}
	for _, k := range []string{"isGlobal", "deleteExistingPeering"} {
		if p[k] == nil {
			p[k] = "False"
		}
	}
	return nil
}

func prepareAdminRule(q *arm.Request, body, existing map[string]any) error {
	if k := strOf(body["kind"]); k != "" && k != "Custom" {
		return arm.BadRequest("InvalidRuleKind", "Only kind Custom admin rules are supported.")
	}
	body["kind"] = "Custom"
	p := props(body)
	name := q.ID.Name()
	switch strOf(p["access"]) {
	case "Allow", "Deny", "AlwaysAllow":
	default:
		return arm.BadRequest("InvalidAdminRule", arm.Errorf("Rule %s: access must be Allow, Deny or AlwaysAllow.", name))
	}
	dir := strOf(p["direction"])
	if dir != "Inbound" && dir != "Outbound" {
		return arm.BadRequest("InvalidAdminRule", arm.Errorf("Rule %s: direction must be Inbound or Outbound.", name))
	}
	prio, _ := p["priority"].(float64)
	if prio < 1 || prio > 4096 {
		return arm.BadRequest("InvalidAdminRule", arm.Errorf("Rule %s: priority must be between 1 and 4096.", name))
	}
	switch strOf(p["protocol"]) {
	case "Tcp", "Udp", "Icmp", "Esp", "Ah", "Any":
	default:
		return arm.BadRequest("InvalidAdminRule", arm.Errorf("Rule %s: protocol must be Tcp, Udp, Icmp, Esp, Ah or Any.", name))
	}
	for _, k := range []string{"sources", "destinations"} {
		for _, a := range entraObjects(p[k]) {
			t := strOf(a["addressPrefixType"])
			if t != "IPPrefix" && t != "ServiceTag" {
				return arm.BadRequest("InvalidAdminRule", arm.Errorf("Rule %s: addressPrefixType must be IPPrefix or ServiceTag.", name))
			}
			if t == "IPPrefix" {
				if _, err := netip.ParsePrefix(strOf(a["addressPrefix"])); err != nil {
					if _, err2 := netip.ParseAddr(strOf(a["addressPrefix"])); err2 != nil && strOf(a["addressPrefix"]) != "*" {
						return arm.BadRequest("InvalidAdminRule", arm.Errorf("Rule %s: '%v' is not a valid IP prefix.", name, a["addressPrefix"]))
					}
				}
			}
		}
	}
	collection := q.ID.ParentPath(ns, q.Reg.Types)
	for _, sib := range q.S.Children(collection + "/rules") {
		sp := props(sib)
		if !strings.EqualFold(strOf(sib["name"]), name) && sp["priority"] == p["priority"] && strOf(sp["direction"]) == dir {
			return arm.BadRequest("AdminRuleConflict", arm.Errorf("Rule %s conflicts with rule %s: same priority %v and direction %s in the collection.", name, sib["name"], p["priority"], dir))
		}
	}
	return nil
}

// ------------------------------------------------------------ commit and deployment status

func depKey(region, typ string) string { return strings.ToLower(region) + "|" + typ }

func commit(q *arm.Request) {
	path := q.ID.Path(ns, q.Reg.Types)
	nm, _ := q.S.LoadResource(path)
	in, ok := q.BodyOrError()
	if !ok {
		return
	}
	typ := strOf(in["commitType"])
	allowed := false
	for _, a := range strList(props(nm)["networkManagerScopeAccesses"]) {
		if a == typ {
			allowed = true
		}
	}
	if !allowed {
		q.Error(400, "InvalidCommitType", arm.Errorf("commitType '%s' is not enabled on this network manager (scope accesses: %s).", typ, strings.Join(strList(props(nm)["networkManagerScopeAccesses"]), ", ")))
		return
	}
	regions := strList(in["targetLocations"])
	if len(regions) == 0 {
		q.Error(400, "InvalidTargetLocations", "targetLocations must list at least one region.")
		return
	}
	ids := strList(in["configurationIds"])
	want := map[string]string{"Connectivity": "connectivityconfigurations", "SecurityAdmin": "securityadminconfigurations", "Routing": "routingconfigurations"}[typ]
	for _, id := range ids {
		if !strings.Contains(strings.ToLower(id), "/"+want+"/") || !strings.EqualFold(managerOf(id), path) {
			q.Error(400, "InvalidConfigurationId", arm.Errorf("Configuration %s is not a %s configuration of this network manager.", id, typ))
			return
		}
		if _, ok := q.S.LoadResource(id); !ok {
			q.Error(404, "ConfigurationNotFound", arm.Errorf("Configuration %s was not found.", id))
			return
		}
	}
	p := props(nm)
	deps, _ := p["_deployments"].(map[string]any)
	if deps == nil {
		deps = map[string]any{}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, r := range regions {
		r = arm.NormalizeLocation(r)
		if len(ids) == 0 {
			delete(deps, depKey(r, typ))
			continue
		}
		deps[depKey(r, typ)] = map[string]any{"region": r, "deploymentType": typ, "configurationIds": toAny(ids), "commitTime": now, "deploymentStatus": "Deployed", "errorMessage": "",
			"_snapshot": snapshot(q.S, ids, typ)}
	}
	p["_deployments"] = deps
	_ = q.S.SaveResource(nm)
	reconcile(q.S, path)
	op := q.S.Ops.Start(arm.ScopeSubscription(path), path, "Succeeded", nil)
	q.AsyncHeaders(op, ns, strOf(nm["location"]))
	q.JSON(202, map[string]any{"commitId": entra.NewGUID(), "targetLocations": toAny(regions), "configurationIds": toAny(ids), "commitType": typ})
}

// snapshot records what was committed: later edits to a configuration only
// take effect after the next commit, exactly like Azure.
func snapshot(s *arm.Server, ids []string, typ string) map[string]any {
	out := map[string]any{}
	for _, id := range ids {
		c, ok := s.LoadResource(id)
		if !ok {
			continue
		}
		snap := map[string]any{"properties": arm.DeepCopy(props(c))}
		if typ == "SecurityAdmin" {
			var cols []any
			for _, col := range s.Children(id + "/ruleCollections") {
				var rules []any
				for _, r := range s.Children(strOf(col["id"]) + "/rules") {
					rules = append(rules, map[string]any{"name": r["name"], "properties": arm.DeepCopy(props(r))})
				}
				cols = append(cols, map[string]any{"name": col["name"], "appliesToGroups": props(col)["appliesToGroups"], "rules": rules})
			}
			snap["ruleCollections"] = cols
		}
		out[strings.ToLower(id)] = snap
	}
	return out
}

// committed returns the snapshot of a configuration in a deployment.
func committed(d map[string]any, id string) (map[string]any, bool) {
	snaps, _ := d["_snapshot"].(map[string]any)
	m, ok := snaps[strings.ToLower(id)].(map[string]any)
	return m, ok
}

func listDeploymentStatus(q *arm.Request) {
	nm, _ := q.S.LoadResource(q.ID.Path(ns, q.Reg.Types))
	in, _ := q.Body()
	regions, types := strList(in["regions"]), strList(in["deploymentTypes"])
	match := func(v string, list []string) bool {
		if len(list) == 0 {
			return true
		}
		for _, x := range list {
			if strings.EqualFold(arm.NormalizeLocation(x), v) || strings.EqualFold(x, v) {
				return true
			}
		}
		return false
	}
	deps, _ := props(nm)["_deployments"].(map[string]any)
	out := []any{}
	keys := make([]string, 0, len(deps))
	for k := range deps {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		d := deps[k].(map[string]any)
		if match(strOf(d["region"]), regions) && match(strOf(d["deploymentType"]), types) {
			c := arm.DeepCopy(d)
			delete(c, "_snapshot")
			out = append(out, c)
		}
	}
	q.JSON(200, map[string]any{"value": out, "skipToken": ""})
}

func listActive(q *arm.Request, typ string) {
	nm, _ := q.S.LoadResource(q.ID.Path(ns, q.Reg.Types))
	in, _ := q.Body()
	regions := strList(in["regions"])
	deps, _ := props(nm)["_deployments"].(map[string]any)
	out := []any{}
	for _, dv := range deps {
		d := dv.(map[string]any)
		if strOf(d["deploymentType"]) != typ {
			continue
		}
		if len(regions) > 0 {
			ok := false
			for _, r := range regions {
				if strings.EqualFold(arm.NormalizeLocation(r), strOf(d["region"])) {
					ok = true
				}
			}
			if !ok {
				continue
			}
		}
		for _, id := range strList(d["configurationIds"]) {
			c, ok := committed(d, id)
			if !ok {
				continue
			}
			item := map[string]any{"id": id, "region": d["region"], "commitTime": d["commitTime"], "properties": c["properties"]}
			if typ == "SecurityAdmin" {
				var rules []any
				for _, col := range entraObjects(c["ruleCollections"]) {
					for _, r := range entraObjects(col["rules"]) {
						rules = append(rules, map[string]any{"name": r["name"], "kind": "Custom", "properties": r["properties"], "ruleCollectionAppliesToGroups": col["appliesToGroups"]})
					}
				}
				item["rules"] = rules
			}
			out = append(out, item)
		}
	}
	q.JSON(200, map[string]any{"value": out, "skipToken": ""})
}

// ------------------------------------------------------------ effects

// groupMembers returns the VNet ids in a network group.
func groupMembers(s *arm.Server, gid string) []string {
	var out []string
	for _, m := range s.Children(gid + "/staticMembers") {
		if vid := strOf(props(m)["resourceId"]); vid != "" {
			if _, ok := s.LoadResource(vid); ok {
				out = append(out, vid)
			}
		}
	}
	return out
}

func vnetLocation(s *arm.Server, id string) string {
	v, _ := s.LoadResource(id)
	return strOf(v["location"])
}

// reconcile makes the network match the deployed connectivity configurations:
// ANM_* peerings for hub-and-spoke, connected groups for meshes. It records
// failures (overlapping address spaces) on the deployment status.
func reconcile(s *arm.Server, nmID string) {
	nm, ok := s.LoadResource(nmID)
	if !ok {
		return
	}
	p := props(nm)
	deps, _ := p["_deployments"].(map[string]any)
	wantPeer := map[string]map[string]any{} // peering id -> body
	var connected []any
	for _, dv := range deps {
		d := dv.(map[string]any)
		if strOf(d["deploymentType"]) != "Connectivity" {
			continue
		}
		region := strOf(d["region"])
		d["deploymentStatus"], d["errorMessage"] = "Deployed", ""
		for _, cid := range strList(d["configurationIds"]) {
			c, ok := committed(d, cid)
			if !ok {
				continue
			}
			cp := props(c)
			topo := strOf(cp["connectivityTopology"])
			hub := ""
			if h := entraObjects(cp["hubs"]); len(h) > 0 {
				hub = strOf(h[0]["resourceId"])
			}
			for _, g := range entraObjects(cp["appliesToGroups"]) {
				global := boolish(g["isGlobal"]) || boolish(cp["isGlobal"])
				var members []string
				for _, vid := range groupMembers(s, strOf(g["networkGroupId"])) {
					if strings.EqualFold(vid, hub) {
						continue // the hub is never its own spoke
					}
					if strings.EqualFold(vnetLocation(s, vid), region) {
						members = append(members, vid)
					}
				}
				if topo == "HubAndSpoke" && hub != "" {
					for _, vid := range members {
						h := shortHashOf(cid + "|" + vid)
						wantPeer[strings.ToLower(vid+"/virtualNetworkPeerings/ANM_"+h)] = anmPeering(vid, hub, "ANM_"+h, cid, false, boolish(g["useHubGateway"]))
						wantPeer[strings.ToLower(hub+"/virtualNetworkPeerings/ANM_"+h)] = anmPeering(hub, vid, "ANM_"+h, cid, boolish(g["useHubGateway"]), false)
					}
				}
				if strOf(g["groupConnectivity"]) == "DirectlyConnected" && len(members) > 0 {
					if msg := overlapError(s, members); msg != "" && !overlapAllowed(cp) {
						d["deploymentStatus"], d["errorMessage"] = "Failed", msg
						continue
					}
					connected = append(connected, map[string]any{"configurationId": cid, "region": region, "global": global, "members": toAny(members)})
				}
			}
		}
	}
	// global meshes connect members across every region they are deployed to
	merged := map[string]map[string]any{}
	var groups []any
	for _, cg := range connected {
		m := cg.(map[string]any)
		if m["global"] == true {
			k := strOf(m["configurationId"])
			if merged[k] == nil {
				merged[k] = map[string]any{"configurationId": k, "region": "global", "members": []any{}}
				groups = append(groups, merged[k])
			}
			merged[k]["members"] = append(merged[k]["members"].([]any), m["members"].([]any)...)
		} else {
			groups = append(groups, m)
		}
	}
	p["_connectedGroups"] = groups
	_ = s.SaveResource(nm)

	// sync ANM peerings owned by this network manager
	var stale []string
	_ = s.Store.Scan(store.BucketARM, "/subscriptions/", func(k string, v []byte) bool {
		if !strings.Contains(k, "/virtualnetworkpeerings/anm_") {
			return true
		}
		var d map[string]any
		if json.Unmarshal(v, &d) == nil && strings.EqualFold(strOf(props(d)["_networkManager"]), nmID) && wantPeer[k] == nil {
			stale = append(stale, strOf(d["id"]))
		}
		return true
	})
	for _, id := range stale {
		s.DeleteTree(id)
	}
	for _, b := range wantPeer {
		props(b)["_networkManager"] = nmID
		_ = s.SaveResource(b)
	}
}

func anmPeering(local, remote, name, cid string, gwTransit, useRemoteGW bool) map[string]any {
	return map[string]any{
		"id": local + "/virtualNetworkPeerings/" + name, "name": name, "type": ns + "/virtualNetworks/virtualNetworkPeerings", "etag": arm.NewETag(),
		"properties": map[string]any{"provisioningState": "Succeeded", "remoteVirtualNetwork": map[string]any{"id": remote},
			"allowVirtualNetworkAccess": true, "allowForwardedTraffic": true, "allowGatewayTransit": gwTransit, "useRemoteGateways": useRemoteGW,
			"doNotVerifyRemoteGateways": false, "resourceGuid": entra.NewGUID(), "connectivityConfigurationId": cid},
	}
}

func shortHashOf(s string) string {
	h := 0
	for _, c := range strings.ToLower(s) {
		h = h*31 + int(c)
	}
	return fmt.Sprintf("%08x", uint32(h))
}

func overlapError(s *arm.Server, members []string) string {
	type sp struct {
		name string
		pfx  []netip.Prefix
	}
	var all []sp
	for _, vid := range members {
		v, _ := s.LoadResource(vid)
		pfx, _ := parsePrefixes(vnetPrefixes(props(v)))
		all = append(all, sp{strOf(v["name"]), pfx})
	}
	for i := range all {
		for j := i + 1; j < len(all); j++ {
			if a, b, bad := anyOverlap(all[i].pfx, all[j].pfx); bad {
				return fmt.Sprintf("Virtual networks %s (%s) and %s (%s) in the connected group have overlapping address spaces.", all[i].name, a, all[j].name, b)
			}
		}
	}
	return ""
}

// ------------------------------------------------------------ inputs for routing / trace

type adminRule struct {
	name, manager, collection string
	p                         map[string]any
	prio                      int
}

// avnmState reads every network manager's connected groups and the active
// security admin rules, indexed by VNet id.
func avnmState(s *arm.Server) (connected map[string][]string, admin map[string][]adminRule) {
	connected, admin = map[string][]string{}, map[string][]adminRule{}
	var managers []map[string]any
	_ = s.Store.Scan(store.BucketARM, "/subscriptions/", func(k string, v []byte) bool {
		if strings.HasSuffix(strings.ToLower(k[:strings.LastIndex(k, "/")]), "/providers/microsoft.network/networkmanagers") {
			var d map[string]any
			if json.Unmarshal(v, &d) == nil {
				managers = append(managers, d)
			}
		}
		return true
	})
	for _, nm := range managers {
		p := props(nm)
		for _, g := range entraObjects(p["_connectedGroups"]) {
			members := strList(g["members"])
			for _, a := range members {
				for _, b := range members {
					if !strings.EqualFold(a, b) {
						connected[strings.ToLower(a)] = append(connected[strings.ToLower(a)], strings.ToLower(b))
					}
				}
			}
		}
		deps, _ := p["_deployments"].(map[string]any)
		for _, dv := range deps {
			d := dv.(map[string]any)
			if strOf(d["deploymentType"]) != "SecurityAdmin" {
				continue
			}
			region := strOf(d["region"])
			for _, cid := range strList(d["configurationIds"]) {
				snap, ok := committed(d, cid)
				if !ok {
					continue
				}
				for _, col := range entraObjects(snap["ruleCollections"]) {
					var members []string
					for _, g := range entraObjects(col["appliesToGroups"]) {
						members = append(members, groupMembers(s, strOf(g["networkGroupId"]))...)
					}
					for _, r := range entraObjects(col["rules"]) {
						rp := props(r)
						pr, _ := rp["priority"].(float64)
						rule := adminRule{name: strOf(r["name"]), manager: strOf(nm["name"]), collection: strOf(col["name"]), p: rp, prio: int(pr)}
						for _, vid := range members {
							if strings.EqualFold(vnetLocation(s, vid), region) {
								admin[strings.ToLower(vid)] = append(admin[strings.ToLower(vid)], rule)
							}
						}
					}
				}
			}
		}
	}
	for k := range admin {
		sort.SliceStable(admin[k], func(i, j int) bool { return admin[k][i].prio < admin[k][j].prio })
	}
	return connected, admin
}

func overlapAllowed(cp map[string]any) bool {
	caps, _ := cp["connectivityCapabilities"].(map[string]any)
	return strings.EqualFold(strOf(caps["connectedGroupAddressOverlap"]), "Allowed")
}
