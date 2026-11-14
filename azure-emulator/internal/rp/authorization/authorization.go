// Package authorization implements the RBAC parts of Microsoft.Authorization:
// role definitions (built-in + custom), role assignments and the
// permissions API. Policy types arrive in phase 3.
package authorization

import (
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/arm"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/hierarchy"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rbac"
)

const ns = "Microsoft.Authorization"

// RP is the authorization resource provider.
type RP struct{ s *arm.Server }

// Register wires the RP.
func Register(s *arm.Server) {
	rp := &RP{s: s}
	ext := []arm.ScopeKind{arm.KindExtension}
	s.Registry.Register(&arm.TypeRegistration{Namespace: ns, Types: []string{"roleDefinitions"}, APIVersions: []string{"2022-05-01-preview", "2022-04-01", "2018-01-01-preview", "2015-07-01"}, Scopes: ext, Handler: rp.roleDefinitions, Phase: 1,
		Notes: "built-in roles (well-known GUIDs) + custom roles; $filter roleName/type"})
	s.Registry.Register(&arm.TypeRegistration{Namespace: ns, Types: []string{"roleAssignments"}, APIVersions: []string{"2022-04-01", "2020-10-01-preview", "2020-04-01-preview", "2015-07-01"}, Scopes: ext, Handler: rp.roleAssignments, Phase: 1,
		Notes: "any scope (MG/sub/RG/resource), inheritance, principal validation, $filter atScope()/principalId/assignedTo(); enforced on every ARM call"})
	s.Registry.Register(&arm.TypeRegistration{Namespace: ns, Types: []string{"permissions"}, APIVersions: []string{"2022-04-01", "2015-07-01"}, Scopes: ext, Handler: rp.permissions, Phase: 1,
		Notes: "effective permissions of the caller at a scope"})
}

func scopeOf(q *arm.Request) string {
	if q.ID.Scope == "" {
		return "/"
	}
	return rp0canonical(q, q.ID.Scope)
}

// rp0canonical canonicalises a scope path (resource group casing, stored resource ids).
func rp0canonical(q *arm.Request, scope string) string {
	seg := hierarchy.Segments(scope)
	if len(seg) >= 4 && strings.EqualFold(seg[0], "subscriptions") && strings.EqualFold(seg[2], "resourceGroups") {
		if len(seg) == 4 {
			if id, ok := q.S.Routes.ResourceGroupScope(seg[1], seg[3]); ok {
				return id
			}
		} else if r, ok := q.S.LoadResource(scope); ok {
			if id, _ := r["id"].(string); id != "" {
				return id
			}
		}
	}
	if len(seg) == 2 && strings.EqualFold(seg[0], "subscriptions") {
		return "/subscriptions/" + seg[1]
	}
	if len(seg) == 4 && strings.EqualFold(seg[0], "providers") && strings.EqualFold(seg[2], "managementGroups") {
		return hierarchy.MGScope(seg[3])
	}
	return scope
}

// roleDefinitionScopePrefix returns the prefix Azure uses in role definition IDs
// for the given scope: the subscription for sub/RG/resource scopes, otherwise the scope itself.
func roleDefinitionScopePrefix(scope string) string {
	if sub := arm.ScopeSubscription(scope); sub != "" {
		return "/subscriptions/" + sub
	}
	if scope == "/" {
		return ""
	}
	return scope
}

func roleDefinitionBody(prefix string, r rbac.RoleDefinition) map[string]any {
	return map[string]any{
		"id":   prefix + "/providers/Microsoft.Authorization/roleDefinitions/" + r.ID,
		"type": "Microsoft.Authorization/roleDefinitions",
		"name": r.ID,
		"properties": map[string]any{
			"roleName": r.RoleName, "type": r.Type, "description": r.Description,
			"assignableScopes": r.AssignableScopes, "permissions": r.Permissions,
			"createdOn": r.CreatedOn, "updatedOn": r.UpdatedOn, "createdBy": nilIfEmpty(r.CreatedBy), "updatedBy": nilIfEmpty(r.UpdatedBy),
		},
	}
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

var roleNameFilter = regexp.MustCompile(`(?i)roleName\s+eq\s+'([^']*)'`)
var typeFilter = regexp.MustCompile(`(?i)type\s+eq\s+'([^']*)'`)

func (rp *RP) assignableAt(r rbac.RoleDefinition, scope string) bool {
	for _, as := range r.AssignableScopes {
		if rp.s.H.IsAncestorOrSelf(as, scope) {
			return true
		}
	}
	return false
}

func (rp *RP) roleDefinitions(q *arm.Request) {
	scope := scopeOf(q)
	prefix := roleDefinitionScopePrefix(scope)
	if q.ID.IsCollection() {
		f := q.R.URL.Query().Get("$filter")
		var nameF, typeF string
		if m := roleNameFilter.FindStringSubmatch(f); m != nil {
			nameF = m[1]
		}
		if m := typeFilter.FindStringSubmatch(f); m != nil {
			typeF = m[1]
		}
		var items []map[string]any
		all := append(append([]rbac.RoleDefinition{}, rbac.BuiltinRoles...), rp.s.RBAC.CustomRoles()...)
		for _, r := range all {
			if nameF != "" && !strings.EqualFold(r.RoleName, nameF) {
				continue
			}
			if typeF != "" && !strings.EqualFold(r.Type, typeF) {
				continue
			}
			if r.Type == "CustomRole" && !rp.assignableAt(r, scope) {
				continue
			}
			items = append(items, roleDefinitionBody(prefix, r))
		}
		q.List(items)
		return
	}
	guid := strings.ToLower(q.ID.Name())
	existing, exists := rp.s.RBAC.RoleDefinition(guid)
	switch q.R.Method {
	case http.MethodGet:
		if !exists {
			q.Error(404, "RoleDefinitionDoesNotExist", arm.Errorf("The specified role definition with ID '%s' does not exist.", guid))
			return
		}
		q.JSON(200, roleDefinitionBody(prefix, existing))
	case http.MethodPut:
		if exists && existing.Type == "BuiltInRole" {
			q.Error(403, "RoleDefinitionUpdateNotPermitted", "Built-in role definitions cannot be updated.")
			return
		}
		in, ok := q.BodyOrError()
		if !ok {
			return
		}
		props, _ := in["properties"].(map[string]any)
		if props == nil {
			q.Error(400, "InvalidRoleDefinition", "The role definition 'properties' are required.")
			return
		}
		name, _ := props["roleName"].(string)
		if name == "" {
			q.Error(400, "RoleDefinitionNameNullOrEmpty", "The role definition name cannot be null or empty.")
			return
		}
		scopes := entra.StringList(props["assignableScopes"])
		if len(scopes) == 0 {
			q.Error(400, "MissingAssignableScopes", "Assignable scopes cannot be null or empty.")
			return
		}
		for _, other := range rp.s.RBAC.CustomRoles() {
			if strings.EqualFold(other.RoleName, name) && !strings.EqualFold(other.ID, guid) {
				q.Error(409, "RoleDefinitionWithSameNameExists", "A custom role with the same name already exists in this directory. Use a different name.")
				return
			}
		}
		for _, b := range rbac.BuiltinRoles {
			if strings.EqualFold(b.RoleName, name) {
				q.Error(409, "RoleDefinitionWithSameNameExists", "A built-in role with the same name already exists. Use a different name.")
				return
			}
		}
		var perms []rbac.Permission
		for _, p := range entra.ObjectList(props["permissions"]) {
			perms = append(perms, rbac.Permission{
				Actions: orEmpty(entra.StringList(p["actions"])), NotActions: orEmpty(entra.StringList(p["notActions"])),
				DataActions: orEmpty(entra.StringList(p["dataActions"])), NotDataActions: orEmpty(entra.StringList(p["notDataActions"])),
			})
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		r := rbac.RoleDefinition{ID: guid, RoleName: name, Description: strOf(props["description"]), Type: "CustomRole", Permissions: perms, AssignableScopes: scopes,
			CreatedOn: now, UpdatedOn: now, CreatedBy: q.Identity.ObjectID, UpdatedBy: q.Identity.ObjectID}
		if exists {
			r.CreatedOn, r.CreatedBy = existing.CreatedOn, existing.CreatedBy
		}
		_ = rp.s.RBAC.PutCustomRole(r)
		status := 201
		if exists {
			status = 200
		}
		q.JSON(status, roleDefinitionBody(prefix, r))
	case http.MethodDelete:
		if !exists {
			q.Empty(204)
			return
		}
		if existing.Type == "BuiltInRole" {
			q.Error(403, "RoleDefinitionDeleteNotPermitted", "Built-in role definitions cannot be deleted.")
			return
		}
		for _, a := range rp.s.RBAC.AssignmentsBelow("/") {
			if strings.EqualFold(a.RoleDefinitionID, guid) {
				q.Error(409, "RoleDefinitionHasAssignments", "There are existing role assignments referencing role (code: RoleDefinitionHasAssignments).")
				return
			}
		}
		rp.s.RBAC.DeleteCustomRole(guid)
		q.JSON(200, roleDefinitionBody(prefix, existing))
	default:
		q.Error(405, "MethodNotAllowed", "The requested method is not allowed.")
	}
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func strOf(v any) string { s, _ := v.(string); return s }

var guidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

var principalIDFilter = regexp.MustCompile(`(?i)principalId\s+eq\s+'([^']*)'`)
var assignedToFilter = regexp.MustCompile(`(?i)assignedTo\('([^']*)'\)`)

func assignmentPath(scope, name string) string {
	if scope == "/" {
		scope = ""
	}
	return scope + "/providers/Microsoft.Authorization/roleAssignments/" + name
}

func (rp *RP) roleAssignments(q *arm.Request) {
	scope := scopeOf(q)
	if q.ID.IsCollection() {
		rp.listAssignments(q, scope)
		return
	}
	name := q.ID.Name()
	path := assignmentPath(scope, name)
	existing, exists := rp.s.LoadResource(path)
	switch q.R.Method {
	case http.MethodGet:
		if !exists {
			q.Error(404, "RoleAssignmentNotFound", arm.Errorf("The role assignment '%s' is not found.", name))
			return
		}
		q.JSON(200, existing)
	case http.MethodPut:
		if !guidRe.MatchString(name) {
			q.Error(400, "InvalidRoleAssignmentId", arm.Errorf("The role assignment ID '%s' is not valid. The role assignment ID must be a GUID.", name))
			return
		}
		in, ok := q.BodyOrError()
		if !ok {
			return
		}
		props, _ := in["properties"].(map[string]any)
		if props == nil {
			q.Error(400, "InvalidRoleAssignmentRequest", "The request body must contain 'properties'.")
			return
		}
		rdID, principalID := strOf(props["roleDefinitionId"]), strOf(props["principalId"])
		if rdID == "" || principalID == "" {
			q.Error(400, "InvalidRoleAssignmentRequest", "The 'roleDefinitionId' and 'principalId' properties are required.")
			return
		}
		rd, ok := rp.s.RBAC.RoleDefinition(rbac.LastSegment(rdID))
		if !ok {
			q.Error(400, "RoleDefinitionDoesNotExist", arm.Errorf("The specified role definition with ID '%s' does not exist.", rbac.LastSegment(rdID)))
			return
		}
		if rd.Type == "CustomRole" && !rp.assignableAt(rd, scope) {
			q.Error(403, "RoleAssignmentScopeNotAssignableToRoleDefinition", arm.Errorf("The role %s is not available for assignment at the requested scope.", rd.RoleName))
			return
		}
		coll, _, found := rp.s.Dir.FindObject(q.Identity.TenantID, principalID)
		if !found {
			q.Error(400, "PrincipalNotFound", arm.Errorf("Principal %s does not exist in the directory %s. Check that you have the correct principal ID. If you are creating this principal and then immediately assigning a role, this error might be related to a replication delay. In this case, set the role assignment principalType property to a value, such as ServicePrincipal, User, or Group.  See https://aka.ms/docs-principaltype", strings.ReplaceAll(principalID, "-", ""), q.Identity.TenantID))
			return
		}
		actualType := map[string]string{entra.Users: "User", entra.Groups: "Group", entra.ServicePrincipals: "ServicePrincipal", entra.Applications: "ServicePrincipal"}[coll]
		ptype := strOf(props["principalType"])
		if ptype == "" {
			ptype = actualType
		} else if !strings.EqualFold(ptype, actualType) && !(strings.EqualFold(ptype, "ForeignGroup") && actualType == "Group") {
			q.Error(400, "UnmatchedPrincipalType", arm.Errorf("The PrincipalId '%s' has type '%s' , which is different from specified PrincipalType '%s'.", strings.ReplaceAll(principalID, "-", ""), actualType, ptype))
			return
		}
		if exists {
			ep, _ := existing["properties"].(map[string]any)
			if !strings.EqualFold(rbac.LastSegment(strOf(ep["roleDefinitionId"])), rd.ID) || !strings.EqualFold(strOf(ep["principalId"]), principalID) {
				q.Error(400, "RoleAssignmentUpdateNotPermitted", "Tenant ID, application ID, principal ID, and scope are not allowed to be updated.")
				return
			}
		} else {
			for _, a := range rp.s.RBAC.AssignmentsAt(scope) {
				if strings.EqualFold(a.PrincipalID, principalID) && strings.EqualFold(a.RoleDefinitionID, rd.ID) {
					q.Error(409, "RoleAssignmentExists", "The role assignment already exists.", map[string]any{"code": "RoleAssignmentExists", "message": rbac.LastSegment(a.ID)})
					return
				}
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		created, createdBy := now, q.Identity.ObjectID
		if exists {
			ep, _ := existing["properties"].(map[string]any)
			created, createdBy = strOf(ep["createdOn"]), strOf(ep["createdBy"])
		}
		body := map[string]any{
			"id":   path,
			"type": "Microsoft.Authorization/roleAssignments",
			"name": strings.ToLower(name),
			"properties": map[string]any{
				"roleDefinitionId":                   rdID,
				"principalId":                        principalID,
				"principalType":                      ptype,
				"scope":                              scope,
				"condition":                          props["condition"],
				"conditionVersion":                   props["conditionVersion"],
				"description":                        props["description"],
				"delegatedManagedIdentityResourceId": props["delegatedManagedIdentityResourceId"],
				"createdOn":                          created,
				"updatedOn":                          now,
				"createdBy":                          createdBy,
				"updatedBy":                          q.Identity.ObjectID,
			},
		}
		if err := rp.s.SaveResource(body); err != nil {
			q.Error(500, "InternalServerError", err.Error())
			return
		}
		q.JSON(201, body)
	case http.MethodDelete:
		if !exists {
			q.Empty(204)
			return
		}
		rp.s.DeleteTree(path)
		q.JSON(200, existing)
	default:
		q.Error(405, "MethodNotAllowed", "The requested method is not allowed.")
	}
}

func (rp *RP) listAssignments(q *arm.Request, scope string) {
	f := q.R.URL.Query().Get("$filter")
	atScope := strings.Contains(strings.ToLower(f), "atscope()")
	var principal string
	if m := principalIDFilter.FindStringSubmatch(f); m != nil {
		principal = m[1]
	}
	var assignedTo string
	if m := assignedToFilter.FindStringSubmatch(f); m != nil {
		assignedTo = m[1]
	}
	seen := map[string]bool{}
	var keys []string
	add := func(id string) {
		k := strings.ToLower(id)
		if !seen[k] {
			seen[k] = true
			keys = append(keys, id)
		}
	}
	for _, s := range rp.s.H.Chain(scope) {
		for _, a := range rp.s.RBAC.AssignmentsAt(s) {
			add(a.ID)
		}
	}
	if !atScope {
		for _, a := range rp.s.RBAC.AssignmentsBelow(scope) {
			add(a.ID)
		}
	}
	var groups map[string]bool
	if assignedTo != "" {
		groups = map[string]bool{strings.ToLower(assignedTo): true}
		for _, g := range rp.s.Dir.TransitiveGroups(q.Identity.TenantID, assignedTo) {
			groups[g] = true
		}
	}
	sort.Strings(keys)
	var items []map[string]any
	for _, k := range keys {
		r, ok := rp.s.LoadResource(k)
		if !ok {
			continue
		}
		p, _ := r["properties"].(map[string]any)
		pid := strings.ToLower(strOf(p["principalId"]))
		if principal != "" && pid != strings.ToLower(principal) {
			continue
		}
		if groups != nil && !groups[pid] {
			continue
		}
		items = append(items, r)
	}
	q.List(items)
}

func (rp *RP) permissions(q *arm.Request) {
	if q.R.Method != http.MethodGet || !q.ID.IsCollection() {
		q.Error(405, "MethodNotAllowed", "Only GET is supported.")
		return
	}
	scope := scopeOf(q)
	principals := map[string]bool{strings.ToLower(q.Identity.ObjectID): true}
	for _, g := range rp.s.Dir.TransitiveGroups(q.Identity.TenantID, q.Identity.ObjectID) {
		principals[g] = true
	}
	var items []map[string]any
	for _, s := range rp.s.H.Chain(scope) {
		for _, a := range rp.s.RBAC.AssignmentsAt(s) {
			if !principals[strings.ToLower(a.PrincipalID)] {
				continue
			}
			if rd, ok := rp.s.RBAC.RoleDefinition(a.RoleDefinitionID); ok {
				for _, p := range rd.Permissions {
					items = append(items, map[string]any{"actions": p.Actions, "notActions": p.NotActions, "dataActions": p.DataActions, "notDataActions": p.NotDataActions})
				}
			}
		}
	}
	if items == nil {
		items = []map[string]any{}
	}
	q.JSON(200, map[string]any{"value": items})
}
