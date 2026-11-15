// Package resources implements Microsoft.Resources: tenants, subscriptions,
// locations, resource provider registration, resource groups, generic
// resource listing and the resource-group delete operation results.
package resources

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/arm"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/hierarchy"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

// RP is the Microsoft.Resources resource provider.
type RP struct{ s *arm.Server }

// Register wires the RP into the ARM server.
func Register(s *arm.Server) {
	rp := &RP{s: s}
	s.Routes.Tenants = rp.tenants
	s.Routes.Subscriptions = rp.subscriptions
	s.Routes.Subscription = rp.subscription
	s.Routes.Locations = rp.locations
	s.Routes.Providers = rp.providers
	s.Routes.Provider = rp.provider
	s.Routes.ResourceGroups = rp.resourceGroups
	s.Routes.ResourceGroup = rp.resourceGroup
	s.Routes.Resources = rp.resources
	s.Routes.OperationResults = rp.operationResults
	s.Routes.ResourceGroupScope = rp.rgCanonical

	versions := []string{"2024-11-01", "2024-07-01", "2024-03-01", "2023-07-01", "2022-09-01", "2021-04-01", "2020-06-01", "2019-10-01"}
	for _, t := range []struct{ typ, notes string }{
		{"resourceGroups", "create/update/patch/delete (async)/list/HEAD, tag filters, cascade delete"},
		{"subscriptions", "get/list (filtered by RBAC visibility); seeded via seed.yaml"},
		{"tenants", "list"},
		{"locations", "curated region list"},
		{"providers", "list/get/register/unregister; every namespace reports Registered"},
		{"resources", "list at subscription and resource group scope ($filter resourceType, tagName/tagValue)"},
	} {
		s.Registry.RegisterBuiltin(&arm.TypeRegistration{Namespace: "Microsoft.Resources", Types: []string{t.typ}, APIVersions: versions, Phase: 1, Notes: t.notes})
	}
}

// ------------------------------------------------------------ tenants / subs

func (rp *RP) tenants(q *arm.Request) {
	var items []map[string]any
	for _, t := range rp.s.Dir.Tenants() {
		if !strings.EqualFold(t.ID, q.Identity.TenantID) {
			continue
		}
		items = append(items, map[string]any{
			"id": "/tenants/" + t.ID, "tenantId": t.ID, "tenantCategory": "Home", "displayName": t.DisplayName,
			"defaultDomain": t.DefaultDomain, "domains": t.Domains, "tenantType": "AAD", "country": "United Kingdom", "countryCode": "GB",
		})
	}
	q.List(items)
}

func subscriptionBody(sub hierarchy.Subscription) map[string]any {
	b := map[string]any{
		"id":                  "/subscriptions/" + sub.ID,
		"authorizationSource": "RoleBased",
		"managedByTenants":    []any{},
		"subscriptionId":      sub.ID,
		"tenantId":            sub.TenantID,
		"displayName":         sub.DisplayName,
		"state":               sub.State,
		"subscriptionPolicies": map[string]any{
			"locationPlacementId": "Public_2014-09-01",
			"quotaId":             "PayAsYouGo_2014-09-01",
			"spendingLimit":       "Off",
		},
	}
	if len(sub.Tags) > 0 {
		b["tags"] = sub.Tags
	}
	return b
}

func (rp *RP) subscriptions(q *arm.Request) {
	if q.R.Method != http.MethodGet {
		q.Error(405, "MethodNotAllowed", "Only GET is supported on /subscriptions.")
		return
	}
	var items []map[string]any
	for _, sub := range rp.s.H.Subscriptions() {
		if !strings.EqualFold(sub.TenantID, q.Identity.TenantID) {
			continue
		}
		if rp.s.RBAC.HasAnyAccess(q.Identity, "/subscriptions/"+sub.ID) {
			items = append(items, subscriptionBody(sub))
		}
	}
	q.List(items)
}

func (rp *RP) subscription(q *arm.Request) {
	sub, _ := rp.s.H.GetSubscription(q.Seg[1])
	if q.R.Method != http.MethodGet {
		q.Error(405, "MethodNotAllowed", "Subscriptions are managed through seed.yaml in CloudTyke (Microsoft.Subscription aliases arrive in a later phase).")
		return
	}
	if !rp.s.RBAC.HasAnyAccess(q.Identity, "/subscriptions/"+sub.ID) {
		q.Authorize("Microsoft.Resources/subscriptions/read", "/subscriptions/"+sub.ID)
		return
	}
	q.JSON(200, subscriptionBody(sub))
}

func (rp *RP) locations(q *arm.Request) {
	if !q.Authorize("Microsoft.Resources/subscriptions/locations/read", "/subscriptions/"+q.Seg[1]) {
		return
	}
	var items []map[string]any
	for _, l := range Locations {
		items = append(items, map[string]any{
			"id":                  "/subscriptions/" + q.Seg[1] + "/locations/" + l.Name,
			"name":                l.Name,
			"type":                "Region",
			"displayName":         l.DisplayName,
			"regionalDisplayName": l.Geography + " - " + l.DisplayName, // approximate
			"metadata": map[string]any{
				"regionType": "Physical", "regionCategory": "Recommended", "geography": l.Geography, "geographyGroup": l.Geography,
				"longitude": l.Longitude, "latitude": l.Latitude, "physicalLocation": l.DisplayName,
				"pairedRegion": []any{map[string]any{"name": l.Paired, "id": "/subscriptions/" + q.Seg[1] + "/locations/" + l.Paired}},
			},
			"availabilityZoneMappings": zoneMappings(l.Name),
		})
	}
	q.List(items)
}

func zoneMappings(loc string) []any {
	return []any{
		map[string]any{"logicalZone": "1", "physicalZone": loc + "-az1"},
		map[string]any{"logicalZone": "2", "physicalZone": loc + "-az2"},
		map[string]any{"logicalZone": "3", "physicalZone": loc + "-az3"},
	}
}

// ------------------------------------------------------------ providers

func (rp *RP) providerBody(sub, ns string) map[string]any {
	canon := ns
	for _, n := range rp.allNamespaces() {
		if strings.EqualFold(n, ns) {
			canon = n
		}
	}
	state := "Registered"
	if sub != "" {
		var st string
		if ok, _ := store.GetJSON(rp.s.Store, store.BucketMeta, "providers/"+strings.ToLower(sub)+"/"+strings.ToLower(canon), &st); ok {
			state = st
		}
	}
	rts := []map[string]any{}
	for _, t := range rp.s.Registry.Namespaces()[canon] {
		locs := []string{}
		for _, l := range Locations {
			locs = append(locs, l.DisplayName)
		}
		rts = append(rts, map[string]any{
			"resourceType": strings.Join(t.Types, "/"), "locations": locs, "apiVersions": t.APIVersions,
			"defaultApiVersion": first(t.APIVersions), "capabilities": "None",
		})
	}
	b := map[string]any{"namespace": canon, "registrationState": state, "registrationPolicy": "RegistrationRequired", "resourceTypes": rts}
	if sub != "" {
		b["id"] = "/subscriptions/" + sub + "/providers/" + canon
	} else {
		b["id"] = "/providers/" + canon
	}
	return b
}

func first(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	return xs[0]
}

func (rp *RP) allNamespaces() []string {
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		if !seen[strings.ToLower(n)] {
			seen[strings.ToLower(n)] = true
			out = append(out, n)
		}
	}
	for ns := range rp.s.Registry.Namespaces() {
		add(ns)
	}
	for _, ns := range KnownNamespaces {
		add(ns)
	}
	sort.Strings(out)
	return out
}

func (rp *RP) providers(q *arm.Request) {
	sub := q.SubscriptionID()
	if sub != "" && !q.Authorize("Microsoft.Resources/subscriptions/providers/read", "/subscriptions/"+sub) {
		return
	}
	var items []map[string]any
	for _, ns := range rp.allNamespaces() {
		items = append(items, rp.providerBody(sub, ns))
	}
	q.List(items)
}

func (rp *RP) provider(q *arm.Request) {
	sub := q.SubscriptionID()
	var ns string
	if sub == "" {
		ns = q.Seg[1]
	} else {
		ns = q.Seg[3]
	}
	if len(q.Seg) == 5 { // register / unregister
		if !q.Authorize(ns+"/register/action", "/subscriptions/"+sub) {
			return
		}
		state := "Registered"
		if strings.EqualFold(q.Seg[4], "unregister") {
			state = "Unregistered"
		}
		_ = store.PutJSON(rp.s.Store, store.BucketMeta, "providers/"+strings.ToLower(sub)+"/"+strings.ToLower(ns), state)
		q.JSON(200, rp.providerBody(sub, ns))
		return
	}
	if q.R.Method != http.MethodGet {
		q.Error(405, "MethodNotAllowed", "Only GET is supported on a resource provider.")
		return
	}
	if sub != "" && !q.Authorize("Microsoft.Resources/subscriptions/providers/read", "/subscriptions/"+sub) {
		return
	}
	known := false
	for _, n := range rp.allNamespaces() {
		known = known || strings.EqualFold(n, ns)
	}
	if !known {
		q.Error(404, "InvalidResourceNamespace", arm.Errorf("The resource namespace '%s' is invalid.", ns))
		return
	}
	q.JSON(200, rp.providerBody(sub, ns))
}

// ------------------------------------------------------------ resource groups

var rgNameRe = regexp.MustCompile(`^[-\w._()]{1,90}$`)

func rgKey(sub, rg string) string {
	return strings.ToLower("/subscriptions/" + sub + "/resourcegroups/" + rg)
}

func (rp *RP) loadRG(sub, rg string) (map[string]any, bool) {
	var m map[string]any
	ok, err := store.GetJSON(rp.s.Store, store.BucketARM, rgKey(sub, rg), &m)
	return m, ok && err == nil
}

// rgCanonical returns the canonical resource group ID if it exists.
func (rp *RP) rgCanonical(sub, rg string) (string, bool) {
	m, ok := rp.loadRG(sub, rg)
	if !ok {
		return "", false
	}
	id, _ := m["id"].(string)
	return id, true
}

func (rp *RP) resourceGroups(q *arm.Request) {
	sub := q.Seg[1]
	if q.R.Method != http.MethodGet {
		q.Error(405, "MethodNotAllowed", "Only GET is supported on the resource group collection.")
		return
	}
	if !q.Authorize("Microsoft.Resources/subscriptions/resourceGroups/read", "/subscriptions/"+sub) {
		return
	}
	prefix := strings.ToLower("/subscriptions/" + sub + "/resourcegroups/")
	tagName, tagValue := tagFilter(q.R.URL.Query().Get("$filter"))
	var items []map[string]any
	_ = rp.s.Store.Scan(store.BucketARM, prefix, func(k string, v []byte) bool {
		if strings.Contains(strings.TrimPrefix(k, prefix), "/") {
			return true
		}
		var m map[string]any
		if json.Unmarshal(v, &m) == nil && matchTag(m, tagName, tagValue) {
			items = append(items, m)
		}
		return true
	})
	q.List(items)
}

func (rp *RP) resourceGroup(q *arm.Request) {
	sub, name := q.Seg[1], q.Seg[3]
	canonicalID := "/subscriptions/" + sub + "/resourceGroups/" + name
	existing, exists := rp.loadRG(sub, name)
	if exists {
		canonicalID, _ = existing["id"].(string)
	}
	scope := canonicalID
	switch q.R.Method {
	case http.MethodHead:
		if !q.Authorize("Microsoft.Resources/subscriptions/resourceGroups/read", scope) {
			return
		}
		if exists {
			q.Empty(204)
		} else {
			q.Empty(404)
		}
	case http.MethodGet:
		if !q.Authorize("Microsoft.Resources/subscriptions/resourceGroups/read", scope) {
			return
		}
		if !exists {
			q.Error(404, "ResourceGroupNotFound", arm.Errorf("Resource group '%s' could not be found.", name))
			return
		}
		q.JSON(200, existing)
	case http.MethodPut:
		if !q.Authorize("Microsoft.Resources/subscriptions/resourceGroups/write", scope) {
			return
		}
		if !rgNameRe.MatchString(name) || strings.HasSuffix(name, ".") {
			q.Error(400, "InvalidResourceGroupName", arm.Errorf("The provided resource group name '%s' is invalid. Resource group names can only include alphanumeric, underscore, parentheses, hyphen, period (except at end), and Unicode characters that match the allowed characters. The name must be 1-90 characters.", name))
			return
		}
		in, ok := q.BodyOrError()
		if !ok {
			return
		}
		loc := arm.NormalizeLocation(strOf(in["location"]))
		if loc == "" {
			q.Error(400, "LocationRequired", "The location property is required for this definition.")
			return
		}
		if !IsLocation(loc) {
			q.Error(400, "LocationNotAvailableForResourceGroup", arm.Errorf("The provided location '%s' is not available for resource group. List of available regions is '%s'.", strOf(in["location"]), strings.Join(LocationNames(), ",")))
			return
		}
		if exists && !strings.EqualFold(strOf(existing["location"]), loc) {
			q.Error(409, "InvalidResourceGroupLocation", arm.Errorf("Invalid resource group location '%s'. The Resource group already exists in location '%s'.", loc, existing["location"]))
			return
		}
		for _, hook := range rp.s.PreWrite {
			if !hook(q) {
				return
			}
		}
		body := map[string]any{
			"id":         canonicalID,
			"name":       nameOf(existing, name),
			"type":       "Microsoft.Resources/resourceGroups",
			"location":   loc,
			"properties": map[string]any{"provisioningState": "Succeeded"},
		}
		if t, ok := in["tags"].(map[string]any); ok && len(t) > 0 {
			body["tags"] = t
		}
		if mb := strOf(in["managedBy"]); mb != "" {
			body["managedBy"] = mb
		}
		if err := store.PutJSON(rp.s.Store, store.BucketARM, rgKey(sub, name), body); err != nil {
			q.Error(500, "InternalServerError", err.Error())
			return
		}
		if exists {
			q.JSON(200, body)
		} else {
			q.JSON(201, body)
		}
	case http.MethodPatch:
		if !q.Authorize("Microsoft.Resources/subscriptions/resourceGroups/write", scope) {
			return
		}
		if !exists {
			q.Error(404, "ResourceGroupNotFound", arm.Errorf("Resource group '%s' could not be found.", name))
			return
		}
		in, ok := q.BodyOrError()
		if !ok {
			return
		}
		if t, ok := in["tags"].(map[string]any); ok {
			if len(t) == 0 {
				delete(existing, "tags")
			} else {
				existing["tags"] = t
			}
		}
		if mb, ok := in["managedBy"].(string); ok {
			existing["managedBy"] = mb
		}
		_ = store.PutJSON(rp.s.Store, store.BucketARM, rgKey(sub, name), existing)
		q.JSON(200, existing)
	case http.MethodDelete:
		if !q.Authorize("Microsoft.Resources/subscriptions/resourceGroups/delete", scope) {
			return
		}
		if !exists {
			q.Error(404, "ResourceGroupNotFound", arm.Errorf("Resource group '%s' could not be found.", name))
			return
		}
		for _, hook := range rp.s.PreWrite {
			if !hook(q) {
				return
			}
		}
		rp.s.DeleteTree(canonicalID)
		op := rp.s.Ops.Start(sub, canonicalID, "Succeeded", nil)
		q.W.Header().Set("Location", q.URL("/subscriptions/"+sub+"/operationresults/"+op.ID, map[string][]string{"api-version": {q.APIVersion}}))
		q.W.Header().Set("Retry-After", "1")
		q.Empty(202)
	default:
		q.Error(405, "MethodNotAllowed", "The requested method is not allowed.")
	}
}

func nameOf(existing map[string]any, def string) string {
	if existing != nil {
		if n, _ := existing["name"].(string); n != "" {
			return n
		}
	}
	return def
}

func strOf(v any) string { s, _ := v.(string); return s }

func (rp *RP) operationResults(q *arm.Request) {
	q.Scope = "/subscriptions/" + q.Seg[1]
	op, ok := rp.s.Ops.Get(q.Seg[3])
	if !ok {
		q.Error(404, "OperationNotFound", arm.Errorf("The operation '%s' could not be found.", q.Seg[3]))
		return
	}
	if !op.Done() {
		q.W.Header().Set("Location", q.URL(q.R.URL.Path, q.R.URL.Query()))
		q.W.Header().Set("Retry-After", "1")
		q.Empty(202)
		return
	}
	if op.Status != "Succeeded" {
		q.JSON(200, map[string]any{"error": op.Error})
		return
	}
	q.Empty(200)
}

// ------------------------------------------------------------ resources list

func (rp *RP) resources(q *arm.Request) {
	sub := q.Seg[1]
	var prefix, scope, action string
	if len(q.Seg) == 5 {
		rgID, ok := rp.rgCanonical(sub, q.Seg[3])
		if !ok {
			q.Error(404, "ResourceGroupNotFound", arm.Errorf("Resource group '%s' could not be found.", q.Seg[3]))
			return
		}
		scope, action = rgID, "Microsoft.Resources/subscriptions/resourceGroups/resources/read"
		prefix = strings.ToLower(rgID) + "/providers/"
	} else {
		scope, action = "/subscriptions/"+sub, "Microsoft.Resources/subscriptions/resources/read"
		prefix = strings.ToLower("/subscriptions/" + sub + "/resourcegroups/")
	}
	if !q.Authorize(action, scope) {
		return
	}
	filter := q.R.URL.Query().Get("$filter")
	typeFilter := ""
	if m := regexp.MustCompile(`(?i)resourceType\s+eq\s+'([^']+)'`).FindStringSubmatch(filter); m != nil {
		typeFilter = m[1]
	}
	tagName, tagValue := tagFilter(filter)
	var items []map[string]any
	_ = rp.s.Store.Scan(store.BucketARM, prefix, func(k string, v []byte) bool {
		if !isTopLevelResourceKey(k) {
			return true
		}
		var m map[string]any
		if json.Unmarshal(v, &m) != nil {
			return true
		}
		if typeFilter != "" && !strings.EqualFold(strOf(m["type"]), typeFilter) {
			return true
		}
		if !matchTag(m, tagName, tagValue) {
			return true
		}
		g := map[string]any{"id": m["id"], "name": m["name"], "type": m["type"]}
		for _, f := range []string{"location", "tags", "kind", "sku", "managedBy", "identity", "plan", "zones", "extendedLocation"} {
			if v, ok := m[f]; ok && v != nil {
				g[f] = v
			}
		}
		items = append(items, g)
		return true
	})
	q.List(items)
}

// isTopLevelResourceKey matches /subscriptions/x/resourcegroups/y/providers/ns/type/name
// and excludes extension resources such as role assignments.
func isTopLevelResourceKey(k string) bool {
	seg := hierarchy.Segments(k)
	if len(seg) != 8 || seg[4] != "providers" {
		return false
	}
	return seg[5] != "microsoft.authorization"
}

var tagNameRe = regexp.MustCompile(`(?i)tagName\s+eq\s+'([^']*)'`)
var tagValueRe = regexp.MustCompile(`(?i)tagValue\s+eq\s+'([^']*)'`)

func tagFilter(f string) (string, string) {
	var n, v string
	if m := tagNameRe.FindStringSubmatch(f); m != nil {
		n = m[1]
	}
	if m := tagValueRe.FindStringSubmatch(f); m != nil {
		v = m[1]
	}
	return n, v
}

func matchTag(m map[string]any, name, value string) bool {
	if name == "" {
		return true
	}
	tags, _ := m["tags"].(map[string]any)
	for k, v := range tags {
		if strings.EqualFold(k, name) {
			return value == "" || v == value
		}
	}
	return false
}

// PutManagedResourceGroup creates (or returns) a resource group owned by
// another resource, such as an AKS node resource group (MC_...).
func PutManagedResourceGroup(s *arm.Server, sub, name, location, managedBy string, tags map[string]any) (string, error) {
	var existing map[string]any
	if ok, err := store.GetJSON(s.Store, store.BucketARM, rgKey(sub, name), &existing); err == nil && ok {
		if mb := strOf(existing["managedBy"]); mb != "" && !strings.EqualFold(mb, managedBy) {
			return "", fmt.Errorf("resource group %s is already managed by %s", name, mb)
		}
		if mb := strOf(existing["managedBy"]); mb == "" {
			return "", fmt.Errorf("resource group %s already exists and is not managed by this resource", name)
		}
		return strOf(existing["id"]), nil
	}
	id := "/subscriptions/" + sub + "/resourceGroups/" + name
	body := map[string]any{"id": id, "name": name, "type": "Microsoft.Resources/resourceGroups", "location": location,
		"managedBy": managedBy, "properties": map[string]any{"provisioningState": "Succeeded"}}
	if len(tags) > 0 {
		body["tags"] = tags
	}
	return id, store.PutJSON(s.Store, store.BucketARM, rgKey(sub, name), body)
}
