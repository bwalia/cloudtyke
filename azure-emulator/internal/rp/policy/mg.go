package policy

import (
	"net/http"
	"strings"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/arm"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/hierarchy"
)

// registerManagementGroups serves the read side of
// Microsoft.Management/managementGroups (the hierarchy is seeded), enough for
// data "azurerm_management_group" and management-group-scoped assignments.
func registerManagementGroups(s *arm.Server) {
	s.Registry.Register(&arm.TypeRegistration{Namespace: "Microsoft.Management", Types: []string{"managementGroups"}, APIVersions: []string{"2023-04-01", "2021-04-01", "2020-05-01"},
		Scopes: []arm.ScopeKind{arm.KindTenant}, Phase: 3, Notes: "read: list, get with $expand=children and recurse (hierarchy from the seed)",
		Handler: func(q *arm.Request) {
			if q.R.Method != http.MethodGet {
				q.Error(405, "MethodNotAllowed", "Management groups are defined by the CloudTyke seed; only GET is supported.")
				return
			}
			if q.ID.IsCollection() {
				var items []map[string]any
				for _, m := range s.H.ManagementGroups() {
					items = append(items, mgBody(s, m, false, false))
				}
				q.List(items)
				return
			}
			m, ok := s.H.GetManagementGroup(q.ID.Name())
			if !ok {
				q.Error(404, "NotFound", arm.Errorf("'%s' not found", q.ID.Name()))
				return
			}
			expand := strings.Contains(strings.ToLower(q.R.URL.Query().Get("$expand")), "children")
			q.JSON(200, mgBody(s, m, expand, strings.EqualFold(q.R.URL.Query().Get("$recurse"), "true")))
		}})
}

func mgBody(s *arm.Server, m hierarchy.ManagementGroup, expand, recurse bool) map[string]any {
	p := map[string]any{"tenantId": m.TenantID, "displayName": m.DisplayName, "details": map[string]any{"version": 1, "updatedTime": "2020-01-01T00:00:00Z", "updatedBy": "cloudtyke-seed"}}
	if m.Parent != "" {
		if pm, ok := s.H.GetManagementGroup(m.Parent); ok {
			p["details"].(map[string]any)["parent"] = map[string]any{"id": hierarchy.MGScope(pm.Name), "name": pm.Name, "displayName": pm.DisplayName}
		}
	}
	if expand {
		var kids []any
		for _, c := range s.H.ManagementGroups() {
			if strings.EqualFold(c.Parent, m.Name) {
				child := map[string]any{"id": hierarchy.MGScope(c.Name), "type": "Microsoft.Management/managementGroups", "name": c.Name, "displayName": c.DisplayName}
				if recurse {
					child["children"] = mgBody(s, c, true, true)["properties"].(map[string]any)["children"]
				}
				kids = append(kids, child)
			}
		}
		for _, sb := range s.H.Subscriptions() {
			if strings.EqualFold(sb.ManagementGroup, m.Name) {
				kids = append(kids, map[string]any{"id": "/subscriptions/" + sb.ID, "type": "/subscriptions", "name": sb.ID, "displayName": sb.DisplayName})
			}
		}
		if kids == nil {
			kids = []any{}
		}
		p["children"] = kids
	}
	return map[string]any{"id": hierarchy.MGScope(m.Name), "type": "Microsoft.Management/managementGroups", "name": m.Name, "properties": p}
}
