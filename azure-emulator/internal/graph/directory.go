package graph

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
)

// RoleTemplate is a directory role template.
type RoleTemplate struct{ ID, Name, Description string }

// DirectoryRoleTemplates are the emulated Entra directory roles (well-known template IDs).
var DirectoryRoleTemplates = []RoleTemplate{
	{"62e90394-69f5-4237-9190-012177145e10", "Global Administrator", "Can manage all aspects of Microsoft Entra ID and Microsoft services that use Microsoft Entra identities."},
	{"fe930be7-5e62-47db-91af-98c3a49a38b1", "User Administrator", "Can manage all aspects of users and groups, including resetting passwords for limited admins."},
	{"9b895d92-2cd3-44c7-9d02-a6ac2d5ea5c3", "Application Administrator", "Can create and manage all aspects of app registrations and enterprise apps."},
	{"158c047a-c907-4556-b7ef-446551a6b5f7", "Cloud Application Administrator", "Can create and manage all aspects of app registrations and enterprise apps except App Proxy."},
	{"fdd7a751-b60b-444a-984c-02652fe8fa1c", "Groups Administrator", "Members of this role can create/manage groups, create/manage groups settings like naming and expiration policies, and view groups activity and audit reports."},
	{"e8611ab8-c189-46e8-94e1-60213ab1f814", "Privileged Role Administrator", "Can manage role assignments in Microsoft Entra ID, and all aspects of Privileged Identity Management."},
	{"f2ef992c-3afb-46b9-b7cf-a126ee74c451", "Global Reader", "Can read everything that a Global Administrator can, but not update anything."},
	{"88d8e3e3-8f55-4a1e-953a-9b9898b8876b", "Directory Readers", "Can read basic directory information. Commonly used to grant directory read access to applications and guests."},
}

func (q *req) organization() {
	t, _ := q.s.Dir.ResolveTenant(q.tid)
	var vds []map[string]any
	for _, d := range t.Domains {
		vds = append(vds, map[string]any{"capabilities": "Email, OfficeCommunicationsOnline", "isDefault": strings.EqualFold(d, t.DefaultDomain), "isInitial": strings.EqualFold(d, t.DefaultDomain), "name": d, "type": "Managed"})
	}
	org := map[string]any{
		"@odata.type": "#microsoft.graph.organization", "id": t.ID, "displayName": t.DisplayName, "createdDateTime": t.CreatedAt,
		"tenantType": "AAD", "countryLetterCode": "GB", "preferredLanguage": "en", "verifiedDomains": vds,
		"assignedPlans": []any{}, "provisionedPlans": []any{}, "businessPhones": []any{}, "technicalNotificationMails": []any{},
		"onPremisesSyncEnabled": nil, "securityComplianceNotificationMails": []any{}, "privacyProfile": nil,
	}
	q.list("organization", []map[string]any{org})
}

func (q *req) domains(rest []string) {
	if !q.require(false, "") {
		return
	}
	t, _ := q.s.Dir.ResolveTenant(q.tid)
	var items []map[string]any
	for _, d := range t.Domains {
		items = append(items, map[string]any{
			"id": d, "authenticationType": "Managed", "availabilityStatus": nil, "isAdminManaged": true,
			"isDefault": strings.EqualFold(d, t.DefaultDomain), "isInitial": strings.EqualFold(d, t.DefaultDomain),
			"isRoot": true, "isVerified": true, "supportedServices": []any{"Email", "OfficeCommunicationsOnline"},
			"passwordNotificationWindowInDays": nil, "passwordValidityPeriodInDays": nil, "state": nil, "manufacturer": nil, "model": nil,
		})
	}
	if len(rest) == 1 {
		for _, it := range items {
			if strings.EqualFold(it["id"].(string), rest[0]) {
				q.entity(200, "domains", it)
				return
			}
		}
		q.notFound(rest[0])
		return
	}
	q.list("domains", items)
}

func (q *req) directoryObjects(rest []string) {
	if len(rest) == 0 {
		q.err(400, "Request_BadRequest", "Listing all directory objects is not supported.")
		return
	}
	if strings.EqualFold(rest[0], "getByIds") && q.r.Method == http.MethodPost {
		if !q.require(false, "") {
			return
		}
		var items []map[string]any
		types := entra.StringList(q.body["types"])
		for _, id := range entra.StringList(q.body["ids"]) {
			if c, o, ok := q.s.Dir.FindObject(q.tid, id); ok {
				if len(types) > 0 && !containsFold(types, strings.TrimSuffix(c, "s")) && !containsFold(types, c) {
					continue
				}
				items = append(items, withType(c, o))
			}
		}
		q.list("directoryObjects", items)
		return
	}
	c, o, ok := q.s.Dir.FindObject(q.tid, rest[0])
	if !ok {
		if !q.require(false, "") {
			return
		}
		q.notFound(rest[0])
		return
	}
	if len(rest) == 1 && q.r.Method == http.MethodGet {
		if !q.require(false, c) {
			return
		}
		q.entity(200, "directoryObjects", withType(c, o))
		return
	}
	q.objectRoute(c, o, rest[1:])
}

// directory/deletedItems
func (q *req) directory(rest []string) {
	if len(rest) == 0 || !strings.EqualFold(rest[0], "deletedItems") {
		q.err(501, "NotImplemented", "CloudTyke only emulates directory/deletedItems under /directory.")
		return
	}
	rest = rest[1:]
	if len(rest) == 1 && strings.HasPrefix(strings.ToLower(rest[0]), "microsoft.graph.") {
		coll := map[string]string{"microsoft.graph.user": entra.Users, "microsoft.graph.group": entra.Groups, "microsoft.graph.application": entra.Applications}[strings.ToLower(rest[0])]
		if coll == "" {
			q.err(400, "Request_BadRequest", "Unsupported deleted item type.")
			return
		}
		if !q.require(false, coll) {
			return
		}
		q.list("directory/deletedItems/"+rest[0], q.s.Dir.ListDeleted(q.tid, coll))
		return
	}
	if len(rest) == 0 {
		q.err(400, "Request_BadRequest", "Specify a type cast, e.g. directory/deletedItems/microsoft.graph.application.")
		return
	}
	c, o, ok := q.s.Dir.GetDeleted(q.tid, rest[0])
	if !ok {
		if !q.require(false, "") {
			return
		}
		q.notFound(rest[0])
		return
	}
	switch {
	case len(rest) == 1 && q.r.Method == http.MethodGet:
		if q.require(false, c) {
			q.entity(200, "directory/deletedItems", o)
		}
	case len(rest) == 1 && q.r.Method == http.MethodDelete:
		if q.require(true, c) {
			q.s.Dir.PurgeDeleted(q.tid, rest[0])
			q.w.WriteHeader(204)
		}
	case len(rest) == 2 && strings.EqualFold(rest[1], "restore") && q.r.Method == http.MethodPost:
		if q.require(true, c) {
			_, obj, _ := q.s.Dir.RestoreDeleted(q.tid, rest[0])
			q.entity(200, c, obj)
		}
	default:
		q.err(400, "Request_BadRequest", "Unsupported operation on deleted item.")
	}
}

// ------------------------------------------------------------------ app role assignments

func assignmentID() string {
	b := []byte(entra.NewGUID())
	return strings.TrimRight(base64.RawURLEncoding.EncodeToString(b[:24]), "=")
}

func principalType(c string) string {
	switch c {
	case entra.Users:
		return "User"
	case entra.Groups:
		return "Group"
	}
	return "ServicePrincipal"
}

// appRoleAssignedTo handles /servicePrincipals/{resource}/appRoleAssignedTo.
func (q *req) appRoleAssignedTo(resource map[string]any, rest []string) {
	list := entra.ObjectList(resource["_appRoleAssignedTo"])
	ctx := "servicePrincipals('" + str(resource["id"]) + "')/appRoleAssignedTo"
	switch {
	case len(rest) == 0 && q.r.Method == http.MethodGet:
		if q.require(false, "appRoleAssignments") {
			q.list(ctx, list)
		}
	case len(rest) == 0 && q.r.Method == http.MethodPost:
		if !q.require(true, "appRoleAssignments") {
			return
		}
		q.createAssignment(resource, ctx)
	case len(rest) == 1 && q.r.Method == http.MethodGet:
		for _, a := range list {
			if str(a["id"]) == rest[0] {
				q.entity(200, ctx, a)
				return
			}
		}
		q.notFound(rest[0])
	case len(rest) == 1 && q.r.Method == http.MethodDelete:
		if !q.require(true, "appRoleAssignments") {
			return
		}
		q.deleteAssignment(resource, rest[0])
	default:
		q.err(400, "Request_BadRequest", "Unsupported operation.")
	}
}

// appRoleAssignments handles /{users|groups|servicePrincipals}/{principal}/appRoleAssignments.
func (q *req) appRoleAssignments(coll string, principal map[string]any, rest []string) {
	pid := str(principal["id"])
	ctx := coll + "('" + pid + "')/appRoleAssignments"
	var mine []map[string]any
	var owners = map[string]map[string]any{}
	for _, sp := range q.s.Dir.List(q.tid, entra.ServicePrincipals) {
		for _, a := range entra.ObjectList(sp["_appRoleAssignedTo"]) {
			if strings.EqualFold(str(a["principalId"]), pid) {
				mine = append(mine, a)
				owners[str(a["id"])] = sp
			}
		}
	}
	switch {
	case len(rest) == 0 && q.r.Method == http.MethodGet:
		if q.require(false, "appRoleAssignments") {
			q.list(ctx, mine)
		}
	case len(rest) == 0 && q.r.Method == http.MethodPost:
		if !q.require(true, "appRoleAssignments") {
			return
		}
		res, ok := q.s.Dir.Get(q.tid, entra.ServicePrincipals, str(q.body["resourceId"]))
		if !ok {
			q.err(400, "Request_BadRequest", "Invalid value specified for property 'resourceId' of resource 'AppRoleAssignment'.")
			return
		}
		q.createAssignment(res, ctx)
	case len(rest) == 1 && q.r.Method == http.MethodGet:
		for _, a := range mine {
			if str(a["id"]) == rest[0] {
				q.entity(200, ctx, a)
				return
			}
		}
		q.notFound(rest[0])
	case len(rest) == 1 && q.r.Method == http.MethodDelete:
		if !q.require(true, "appRoleAssignments") {
			return
		}
		sp, ok := owners[rest[0]]
		if !ok {
			q.notFound(rest[0])
			return
		}
		q.deleteAssignment(sp, rest[0])
	default:
		q.err(400, "Request_BadRequest", "Unsupported operation.")
	}
}

func (q *req) createAssignment(resource map[string]any, ctx string) {
	pid := str(q.body["principalId"])
	roleID := str(q.body["appRoleId"])
	c, p, ok := q.s.Dir.FindObject(q.tid, pid)
	if !ok {
		q.err(400, "Request_BadRequest", "Invalid value specified for property 'principalId' of resource 'AppRoleAssignment'.")
		return
	}
	valid := roleID == "00000000-0000-0000-0000-000000000000"
	for _, r := range entra.ObjectList(resource["appRoles"]) {
		valid = valid || strings.EqualFold(str(r["id"]), roleID)
	}
	if !valid {
		q.err(400, "Request_BadRequest", "Permission being assigned was not found on application '"+str(resource["appId"])+"'.")
		return
	}
	list := entra.ObjectList(resource["_appRoleAssignedTo"])
	for _, a := range list {
		if strings.EqualFold(str(a["principalId"]), pid) && strings.EqualFold(str(a["appRoleId"]), roleID) {
			q.err(400, "Request_BadRequest", "Permission being assigned already exists on the object")
			return
		}
	}
	a := map[string]any{
		"@odata.type": "#microsoft.graph.appRoleAssignment",
		"id":          assignmentID(), "deletedDateTime": nil, "appRoleId": roleID, "createdDateTime": entra.Now(),
		"principalDisplayName": p["displayName"], "principalId": pid, "principalType": principalType(c),
		"resourceDisplayName": resource["displayName"], "resourceId": resource["id"],
	}
	resource["_appRoleAssignedTo"] = mapsToAny(append(list, a))
	_ = q.s.Dir.Put(q.tid, entra.ServicePrincipals, resource)
	q.entity(201, ctx, a)
}

func (q *req) deleteAssignment(resource map[string]any, id string) {
	list := entra.ObjectList(resource["_appRoleAssignedTo"])
	for i, a := range list {
		if str(a["id"]) == id {
			resource["_appRoleAssignedTo"] = mapsToAny(append(list[:i], list[i+1:]...))
			_ = q.s.Dir.Put(q.tid, entra.ServicePrincipals, resource)
			q.w.WriteHeader(204)
			return
		}
	}
	q.notFound(id)
}

// ------------------------------------------------------------------ oauth2PermissionGrants

func (q *req) grants(rest []string) {
	switch {
	case len(rest) == 0 && q.r.Method == http.MethodGet:
		if q.require(false, collGrants) {
			q.list(collGrants, q.s.Dir.List(q.tid, collGrants))
		}
	case len(rest) == 0 && q.r.Method == http.MethodPost:
		if !q.require(true, collGrants) {
			return
		}
		for _, f := range []string{"clientId", "consentType", "resourceId", "scope"} {
			if str(q.body[f]) == "" {
				q.err(400, "Request_BadRequest", "Invalid value specified for property '"+f+"' of resource 'OAuth2PermissionGrant'.")
				return
			}
		}
		g := map[string]any{"id": assignmentID(), "clientId": q.body["clientId"], "consentType": q.body["consentType"], "principalId": q.body["principalId"], "resourceId": q.body["resourceId"], "scope": q.body["scope"]}
		_ = q.s.Dir.Put(q.tid, collGrants, g)
		q.entity(201, collGrants, g)
	case len(rest) == 1:
		g, ok := q.s.Dir.Get(q.tid, collGrants, rest[0])
		if !ok {
			q.notFound(rest[0])
			return
		}
		switch q.r.Method {
		case http.MethodGet:
			if q.require(false, collGrants) {
				q.entity(200, collGrants, g)
			}
		case http.MethodPatch:
			if q.require(true, collGrants) {
				if s, ok := q.body["scope"]; ok {
					g["scope"] = s
				}
				_ = q.s.Dir.Put(q.tid, collGrants, g)
				q.w.WriteHeader(204)
			}
		case http.MethodDelete:
			if q.require(true, collGrants) {
				q.s.Dir.Delete(q.tid, collGrants, rest[0])
				q.w.WriteHeader(204)
			}
		}
	default:
		q.err(400, "Request_BadRequest", "Unsupported operation.")
	}
}

// ------------------------------------------------------------------ directoryRoles

func (q *req) directoryRoles(rest []string) {
	switch {
	case len(rest) == 0 && q.r.Method == http.MethodGet:
		if q.require(false, collRoles) {
			q.list(collRoles, q.s.Dir.List(q.tid, collRoles))
		}
	case len(rest) == 0 && q.r.Method == http.MethodPost:
		if !q.require(true, collRoles) {
			return
		}
		tmpl := str(q.body["roleTemplateId"])
		for _, r := range q.s.Dir.List(q.tid, collRoles) {
			if strings.EqualFold(str(r["roleTemplateId"]), tmpl) {
				q.err(409, "Request_BadRequest", "A conflicting object with one or more of the specified property values is present in the directory.")
				return
			}
		}
		for _, t := range DirectoryRoleTemplates {
			if strings.EqualFold(t.ID, tmpl) {
				r := ActivateRole(t)
				_ = q.s.Dir.Put(q.tid, collRoles, r)
				q.entity(201, collRoles, r)
				return
			}
		}
		q.err(400, "Request_BadRequest", "Invalid value specified for property 'roleTemplateId'.")
	default:
		var role map[string]any
		for _, r := range q.s.Dir.List(q.tid, collRoles) {
			if strings.EqualFold(str(r["id"]), rest[0]) || strings.EqualFold(str(r["roleTemplateId"]), strings.TrimSuffix(strings.TrimPrefix(rest[0], "roleTemplateId="), "")) {
				role = r
			}
		}
		if role == nil {
			q.notFound(rest[0])
			return
		}
		if len(rest) == 1 {
			if q.require(false, collRoles) {
				q.entity(200, collRoles, role)
			}
			return
		}
		if strings.EqualFold(rest[1], "members") {
			q.relation(collRoles, role, "_members", "members", rest[2:])
			return
		}
		q.err(400, "Request_BadRequest", "Unsupported operation.")
	}
}

// ActivateRole builds an activated directoryRole object from a template.
func ActivateRole(t RoleTemplate) map[string]any {
	return map[string]any{"id": entra.NewGUID(), "deletedDateTime": nil, "description": t.Description, "displayName": t.Name, "roleTemplateId": t.ID, "_members": []any{}}
}
