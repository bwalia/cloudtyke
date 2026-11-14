// Package graph serves the emulated Microsoft Graph subset (/v1.0 and
// /beta) used by the azuread Terraform provider, SDKs and scripts: users,
// groups (+owners/members), applications (+passwords, federated identity
// credentials), service principals, app role assignments, OAuth2
// permission grants, directory roles, directory objects and deleted items.
package graph

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/auth"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/config"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
)

// Server is the Graph front end.
type Server struct {
	Cfg  config.Config
	Auth *auth.Service
	Dir  *entra.Directory
	mu   sync.Mutex
}

// collections addressable at the root.
var rootCollections = map[string]string{
	"users": entra.Users, "groups": entra.Groups, "applications": entra.Applications, "serviceprincipals": entra.ServicePrincipals,
}

const (
	collGrants = "oauth2PermissionGrants"
	collRoles  = "directoryRoles"
)

type req struct {
	s       *Server
	w       http.ResponseWriter
	r       *http.Request
	id      *auth.Identity
	tid     string
	version string
	seg     []string
	body    map[string]any
}

// ServeHTTP handles graph.<domain>.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rid := entra.NewGUID()
	w.Header().Set("request-id", rid)
	w.Header().Set("client-request-id", firstNonEmpty(r.Header.Get("client-request-id"), rid))
	w.Header().Set("x-ms-ags-diagnostic", `{"ServerInfo":{"DataCenter":"CloudTyke","Slice":"E","Ring":"0","ScaleUnit":"000","RoleInstance":"cloudtyke"}}`)
	seg := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(seg) < 2 || (seg[0] != "v1.0" && seg[0] != "beta") {
		gerr(w, r, 400, "BadRequest", "Invalid version: "+seg[0])
		return
	}
	id, _, msg := s.Auth.Authenticate(r, s.Cfg.Host(config.SubGraph))
	if id == nil {
		if strings.Contains(msg, "audience") {
			msg = "Access token validation failure. Invalid audience."
		} else if r.Header.Get("Authorization") == "" {
			msg = "Access token is empty."
		}
		gerr(w, r, 401, "InvalidAuthenticationToken", msg)
		return
	}
	q := &req{s: s, w: w, r: r, id: id, tid: id.TenantID, version: seg[0], seg: seg[1:]}
	if r.Method == http.MethodPost || r.Method == http.MethodPatch || r.Method == http.MethodPut {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		q.body = map[string]any{}
		if len(strings.TrimSpace(string(b))) > 0 {
			if err := json.Unmarshal(b, &q.body); err != nil {
				gerr(w, r, 400, "BadRequest", "Unable to read JSON request payload. Please ensure Content-Type header is set and payload is of valid JSON format.")
				return
			}
		}
		s.mu.Lock()
		defer s.mu.Unlock()
	} else if r.Method == http.MethodDelete {
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	q.route()
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func gerr(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
		"code": code, "message": msg,
		"innerError": map[string]any{"date": time.Now().UTC().Format("2006-01-02T15:04:05"), "request-id": w.Header().Get("request-id"), "client-request-id": w.Header().Get("client-request-id")},
	}})
}

func (q *req) err(status int, code, msg string) { gerr(q.w, q.r, status, code, msg) }

func (q *req) notFound(id string) {
	q.err(404, "Request_ResourceNotFound", "Resource '"+id+"' does not exist or one of its queried reference-property objects are not present.")
}

func (q *req) json(status int, v any) {
	q.w.Header().Set("Content-Type", "application/json;odata.metadata=minimal;odata.streaming=true;IEEE754Compatible=false;charset=utf-8")
	q.w.WriteHeader(status)
	_ = json.NewEncoder(q.w).Encode(v)
}

func (q *req) base() string { return q.s.Cfg.URL(config.SubGraph) + "/" + q.version }

// entity writes a single object.
func (q *req) entity(status int, ctx string, o map[string]any) {
	out := project(public(o), q.r.URL.Query().Get("$select"))
	out["@odata.context"] = q.base() + "/$metadata#" + ctx + "/$entity"
	q.json(status, out)
}

// list writes a filtered, projected, paged collection.
func (q *req) list(ctx string, items []map[string]any) {
	qs := q.r.URL.Query()
	f, err := parseFilter(qs.Get("$filter"))
	if err != nil {
		q.err(400, "Request_UnsupportedQuery", "Unsupported Query: "+err.Error())
		return
	}
	var filtered []map[string]any
	for _, it := range items {
		if f(it) {
			filtered = append(filtered, it)
		}
	}
	if s := strings.ToLower(strings.TrimSpace(qs.Get("$search"))); s != "" {
		s = strings.Trim(s, `"`)
		if i := strings.Index(s, ":"); i > 0 {
			field, val := s[:i], s[i+1:]
			var out []map[string]any
			for _, it := range filtered {
				v, _ := lookup(it, field).(string)
				if strings.Contains(strings.ToLower(v), val) {
					out = append(out, it)
				}
			}
			filtered = out
		}
	}
	if ob := qs.Get("$orderby"); ob != "" {
		field := strings.Fields(ob)[0]
		desc := strings.HasSuffix(strings.ToLower(ob), " desc")
		sort.SliceStable(filtered, func(i, j int) bool {
			a, _ := lookup(filtered[i], field).(string)
			b, _ := lookup(filtered[j], field).(string)
			if desc {
				return strings.ToLower(a) > strings.ToLower(b)
			}
			return strings.ToLower(a) < strings.ToLower(b)
		})
	}
	top := 100
	if t, err := strconv.Atoi(qs.Get("$top")); err == nil && t > 0 {
		top = min(t, 999)
	}
	start := 0
	if s, err := strconv.Atoi(qs.Get("$skiptoken")); err == nil {
		start = min(s, len(filtered))
	}
	end := min(start+top, len(filtered))
	page := make([]map[string]any, 0, end-start)
	sel := qs.Get("$select")
	for _, it := range filtered[start:end] {
		page = append(page, project(public(it), sel))
	}
	out := map[string]any{"@odata.context": q.base() + "/$metadata#" + ctx, "value": page}
	if strings.EqualFold(qs.Get("$count"), "true") {
		out["@odata.count"] = len(filtered)
	}
	if end < len(filtered) {
		qs.Set("$skiptoken", strconv.Itoa(end))
		out["@odata.nextLink"] = q.base() + "/" + strings.Join(q.seg, "/") + "?" + qs.Encode()
	}
	q.json(200, out)
}

// ------------------------------------------------------------------ authz

func (q *req) hasRole(roles ...string) bool {
	for _, r := range roles {
		for _, have := range q.id.Roles {
			if strings.EqualFold(r, have) {
				return true
			}
		}
	}
	return false
}

func (q *req) require(write bool, coll string) bool {
	var roles []string
	switch coll {
	case entra.Users:
		roles = []string{"User.ReadWrite.All", "Directory.ReadWrite.All"}
		if !write {
			roles = append(roles, "User.Read.All", "Directory.Read.All", "User.ReadBasic.All")
		}
	case entra.Groups:
		roles = []string{"Group.ReadWrite.All", "Directory.ReadWrite.All"}
		if !write {
			roles = append(roles, "Group.Read.All", "Directory.Read.All", "GroupMember.Read.All", "GroupMember.ReadWrite.All")
		}
	case "groupMembers":
		roles = []string{"Group.ReadWrite.All", "Directory.ReadWrite.All", "GroupMember.ReadWrite.All"}
	case entra.Applications, entra.ServicePrincipals:
		roles = []string{"Application.ReadWrite.All", "Directory.ReadWrite.All", "Application.ReadWrite.OwnedBy"}
		if !write {
			roles = append(roles, "Application.Read.All", "Directory.Read.All")
		}
	case "appRoleAssignments", collGrants:
		roles = []string{"AppRoleAssignment.ReadWrite.All", "Application.ReadWrite.All", "Directory.ReadWrite.All", "DelegatedPermissionGrant.ReadWrite.All"}
		if !write {
			roles = append(roles, "Application.Read.All", "Directory.Read.All")
		}
	case collRoles:
		roles = []string{"RoleManagement.ReadWrite.Directory", "Directory.ReadWrite.All"}
		if !write {
			roles = append(roles, "RoleManagement.Read.Directory", "Directory.Read.All")
		}
	default:
		roles = []string{"Directory.ReadWrite.All"}
		if !write {
			roles = append(roles, "Directory.Read.All", "Application.Read.All", "User.Read.All", "Group.Read.All")
		}
	}
	if q.hasRole(roles...) {
		return true
	}
	q.err(403, "Authorization_RequestDenied", "Insufficient privileges to complete the operation.")
	return false
}

// ------------------------------------------------------------------ routing

var altKeyRe = regexp.MustCompile(`(?i)^(\w+)\((\w+)=['{]*([^'}]*)['}]*\)$`)

func (q *req) route() {
	seg := q.seg
	m := q.r.Method
	head := seg[0]

	// alternate key syntax: applications(appId='...') / servicePrincipals(appId='...')
	if mm := altKeyRe.FindStringSubmatch(head); mm != nil {
		coll, ok := rootCollections[strings.ToLower(mm[1])]
		if !ok {
			q.err(400, "BadRequest", "Unsupported alternate key on "+mm[1])
			return
		}
		o, found := q.s.Dir.FindBy(q.tid, coll, mm[2], mm[3])
		if !found {
			q.notFound(mm[3])
			return
		}
		q.objectRoute(coll, o, seg[1:])
		return
	}

	switch strings.ToLower(head) {
	case "organization":
		if !q.require(false, "") {
			return
		}
		q.organization()
		return
	case "domains":
		q.domains(seg[1:])
		return
	case "me":
		q.err(400, "BadRequest", "/me request is only valid with delegated authentication flow.")
		return
	case "directoryobjects":
		q.directoryObjects(seg[1:])
		return
	case "directory":
		q.directory(seg[1:])
		return
	case "oauth2permissiongrants":
		q.grants(seg[1:])
		return
	case "directoryroles":
		q.directoryRoles(seg[1:])
		return
	case "directoryroletemplates":
		if !q.require(false, collRoles) {
			return
		}
		var items []map[string]any
		for _, t := range DirectoryRoleTemplates {
			items = append(items, map[string]any{"@odata.type": "#microsoft.graph.directoryRoleTemplate", "id": t.ID, "displayName": t.Name, "description": t.Description, "deletedDateTime": nil})
		}
		q.list("directoryRoleTemplates", items)
		return
	}

	coll, ok := rootCollections[strings.ToLower(head)]
	if !ok {
		q.err(501, "NotImplemented", "CloudTyke does not emulate Microsoft Graph resource '"+head+"' yet. See "+q.s.Cfg.DocsURL)
		return
	}
	if len(seg) == 1 {
		switch m {
		case http.MethodGet:
			if !q.require(false, coll) {
				return
			}
			q.list(coll, q.s.Dir.List(q.tid, coll))
		case http.MethodPost:
			if !q.require(true, coll) {
				return
			}
			q.create(coll)
		default:
			q.err(405, "Request_BadRequest", "Method not allowed.")
		}
		return
	}
	// "delta" and casts are not supported
	o, found := q.s.Dir.Get(q.tid, coll, seg[1])
	if !found && coll == entra.Users {
		o, found = q.s.Dir.FindBy(q.tid, entra.Users, "userPrincipalName", seg[1])
	}
	if !found {
		if !q.require(false, coll) {
			return
		}
		q.notFound(seg[1])
		return
	}
	q.objectRoute(coll, o, seg[2:])
}

func (q *req) objectRoute(coll string, o map[string]any, rest []string) {
	m := q.r.Method
	id, _ := o["id"].(string)
	if len(rest) == 0 {
		switch m {
		case http.MethodGet:
			if !q.require(false, coll) {
				return
			}
			q.entity(200, coll, o)
		case http.MethodPatch:
			if !q.require(true, coll) || !q.ownedByCheck(coll, o) {
				return
			}
			q.update(coll, o)
		case http.MethodDelete:
			if !q.require(true, coll) || !q.ownedByCheck(coll, o) {
				return
			}
			q.s.Dir.Delete(q.tid, coll, id)
			if coll == entra.Applications {
				if sp, ok := q.s.Dir.FindBy(q.tid, entra.ServicePrincipals, "appId", o["appId"].(string)); ok {
					q.s.Dir.Delete(q.tid, entra.ServicePrincipals, sp["id"].(string))
				}
			}
			q.w.WriteHeader(204)
		default:
			q.err(405, "Request_BadRequest", "Method not allowed.")
		}
		return
	}
	nav := strings.ToLower(rest[0])
	switch nav {
	case "owners":
		q.relation(coll, o, "_owners", "owners", rest[1:])
	case "members":
		if coll != entra.Groups {
			q.notFound(rest[0])
			return
		}
		q.relation(coll, o, "_members", "members", rest[1:])
	case "transitivemembers":
		if !q.require(false, entra.Groups) {
			return
		}
		q.list("directoryObjects", q.transitiveMembers(id))
	case "memberof", "transitivememberof":
		if !q.require(false, entra.Groups) {
			return
		}
		var groups []string
		if nav == "memberof" {
			for _, g := range q.s.Dir.List(q.tid, entra.Groups) {
				if entra.Contains(entra.StringList(g["_members"]), id) {
					groups = append(groups, g["id"].(string))
				}
			}
		} else {
			groups = q.s.Dir.TransitiveGroups(q.tid, id)
		}
		var items []map[string]any
		for _, gid := range groups {
			if g, ok := q.s.Dir.Get(q.tid, entra.Groups, gid); ok {
				items = append(items, withType(entra.Groups, g))
			}
		}
		q.list("directoryObjects", items)
	case "checkmembergroups", "getmembergroups":
		groups := q.s.Dir.TransitiveGroups(q.tid, id)
		if nav == "checkmembergroups" {
			want := entra.StringList(q.body["groupIds"])
			var out []string
			for _, g := range want {
				if entra.Contains(groups, g) {
					out = append(out, g)
				}
			}
			groups = out
		}
		q.json(200, map[string]any{"value": orEmpty(groups)})
	case "addpassword":
		q.addPassword(coll, o)
	case "removepassword":
		q.removePassword(coll, o)
	case "federatedidentitycredentials":
		if coll != entra.Applications {
			q.notFound(rest[0])
			return
		}
		q.fics(o, rest[1:])
	case "approleassignedto":
		if coll != entra.ServicePrincipals {
			q.notFound(rest[0])
			return
		}
		q.appRoleAssignedTo(o, rest[1:])
	case "approleassignments":
		q.appRoleAssignments(coll, o, rest[1:])
	case "oauth2permissiongrants":
		if !q.require(false, collGrants) {
			return
		}
		var items []map[string]any
		for _, g := range q.s.Dir.List(q.tid, collGrants) {
			if strings.EqualFold(str(g["clientId"]), id) || strings.EqualFold(str(g["principalId"]), id) {
				items = append(items, g)
			}
		}
		q.list(collGrants, items)
	case "manager":
		if coll != entra.Users {
			q.notFound(rest[0])
			return
		}
		q.manager(o, rest[1:])
	case "directreports":
		if !q.require(false, entra.Users) {
			return
		}
		var items []map[string]any
		for _, u := range q.s.Dir.List(q.tid, entra.Users) {
			if strings.EqualFold(str(u["_manager"]), id) {
				items = append(items, withType(entra.Users, u))
			}
		}
		q.list("directoryObjects", items)
	case "restore":
		q.err(400, "Request_BadRequest", "Object is not deleted.")
	default:
		q.err(501, "NotImplemented", "CloudTyke does not emulate navigation '"+rest[0]+"' on "+coll+" yet.")
	}
}

// ownedByCheck enforces Application.ReadWrite.OwnedBy semantics.
func (q *req) ownedByCheck(coll string, o map[string]any) bool {
	if coll != entra.Applications && coll != entra.ServicePrincipals {
		return true
	}
	if q.hasRole("Application.ReadWrite.All", "Directory.ReadWrite.All") {
		return true
	}
	if entra.Contains(entra.StringList(o["_owners"]), q.id.ObjectID) {
		return true
	}
	q.err(403, "Authorization_RequestDenied", "Insufficient privileges to complete the operation.")
	return false
}

// ------------------------------------------------------------------ create / update

var passwordClasses = []func(rune) bool{unicode.IsUpper, unicode.IsLower, unicode.IsDigit, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }}

func passwordOK(p string) bool {
	if len(p) < 8 || len(p) > 256 {
		return false
	}
	n := 0
	for _, cls := range passwordClasses {
		for _, r := range p {
			if cls(r) {
				n++
				break
			}
		}
	}
	return n >= 3
}

func (q *req) create(coll string) {
	b := q.body
	now := entra.Now()
	o := map[string]any{}
	for k, v := range b {
		if !strings.Contains(k, "@odata.bind") && k != "id" {
			o[k] = v
		}
	}
	o["id"] = entra.NewGUID()
	o["deletedDateTime"] = nil
	o["createdDateTime"] = now
	tenant, _ := q.s.Dir.ResolveTenant(q.tid)

	switch coll {
	case entra.Users:
		for _, f := range []string{"accountEnabled", "displayName", "mailNickname", "userPrincipalName", "passwordProfile"} {
			if _, ok := b[f]; !ok {
				q.err(400, "Request_BadRequest", "Invalid value specified for property '"+f+"' of resource 'User'.")
				return
			}
		}
		upn := str(b["userPrincipalName"])
		at := strings.LastIndex(upn, "@")
		if at <= 0 || !containsFold(tenant.Domains, upn[at+1:]) {
			q.err(400, "Request_BadRequest", "The domain portion of the userPrincipalName property is invalid. You must use one of the verified domain names in your organization.")
			return
		}
		if _, dup := q.s.Dir.FindBy(q.tid, entra.Users, "userPrincipalName", upn); dup {
			q.err(400, "Request_BadRequest", "Another object with the same value for property userPrincipalName already exists.")
			return
		}
		pp, _ := b["passwordProfile"].(map[string]any)
		pw := str(pp["password"])
		if !passwordOK(pw) {
			q.err(400, "Request_BadRequest", "The specified password does not comply with password complexity requirements. Please provide a different password.")
			return
		}
		delete(o, "passwordProfile")
		o["_passwordHash"] = entra.HashSecret(pw)
		o["_forceChangePassword"] = pp["forceChangePasswordNextSignIn"]
		defaults(o, map[string]any{
			"businessPhones": []any{}, "givenName": nil, "surname": nil, "jobTitle": nil, "mail": nil, "mobilePhone": nil,
			"officeLocation": nil, "preferredLanguage": nil, "userType": "Member", "otherMails": []any{}, "proxyAddresses": []any{},
			"imAddresses": []any{}, "department": nil, "companyName": nil, "employeeId": nil, "usageLocation": nil,
			"onPremisesSyncEnabled": nil, "onPremisesImmutableId": nil, "creationType": nil, "externalUserState": nil,
			"securityIdentifier": "S-1-12-1-" + strconv.Itoa(int(time.Now().UnixNano()%2147483647)),
		})
	case entra.Groups:
		for _, f := range []string{"displayName", "mailEnabled", "mailNickname", "securityEnabled"} {
			if _, ok := b[f]; !ok {
				q.err(400, "Request_BadRequest", "Invalid value specified for property '"+f+"' of resource 'Group'.")
				return
			}
		}
		defaults(o, map[string]any{
			"description": nil, "groupTypes": []any{}, "mail": nil, "membershipRule": nil, "membershipRuleProcessingState": nil,
			"visibility": nil, "isAssignableToRole": nil, "classification": nil, "proxyAddresses": []any{},
			"securityIdentifier":    "S-1-12-1-" + strconv.Itoa(int(time.Now().UnixNano()%2147483647)),
			"onPremisesSyncEnabled": nil, "preferredDataLocation": nil, "renewedDateTime": now, "theme": nil, "resourceProvisioningOptions": []any{}, "resourceBehaviorOptions": []any{},
		})
		if mail, _ := b["mailEnabled"].(bool); mail {
			o["mail"] = str(b["mailNickname"]) + "@" + tenant.DefaultDomain
		}
	case entra.Applications:
		if str(b["displayName"]) == "" {
			q.err(400, "Request_BadRequest", "Invalid value specified for property 'displayName' of resource 'Application'.")
			return
		}
		o["appId"] = entra.NewGUID()
		o["passwordCredentials"] = []any{}
		o["keyCredentials"] = []any{}
		o["publisherDomain"] = tenant.DefaultDomain
		defaults(o, map[string]any{
			"signInAudience": "AzureADMyOrg", "identifierUris": []any{}, "appRoles": []any{}, "requiredResourceAccess": []any{},
			"tags": []any{}, "description": nil, "notes": nil, "groupMembershipClaims": nil, "isFallbackPublicClient": nil,
			"isDeviceOnlyAuthSupported": nil, "serviceManagementReference": nil, "tokenEncryptionKeyId": nil, "logo": nil,
			"api":                               map[string]any{"acceptMappedClaims": nil, "knownClientApplications": []any{}, "requestedAccessTokenVersion": nil, "oauth2PermissionScopes": []any{}, "preAuthorizedApplications": []any{}},
			"web":                               map[string]any{"homePageUrl": nil, "logoutUrl": nil, "redirectUris": []any{}, "implicitGrantSettings": map[string]any{"enableAccessTokenIssuance": false, "enableIdTokenIssuance": false}, "redirectUriSettings": []any{}},
			"spa":                               map[string]any{"redirectUris": []any{}},
			"publicClient":                      map[string]any{"redirectUris": []any{}},
			"info":                              map[string]any{"logoUrl": nil, "marketingUrl": nil, "privacyStatementUrl": nil, "supportUrl": nil, "termsOfServiceUrl": nil},
			"optionalClaims":                    nil,
			"parentalControlSettings":           map[string]any{"countriesBlockedForMinors": []any{}, "legalAgeGroupRule": "Allow"},
			"verifiedPublisher":                 map[string]any{"displayName": nil, "verifiedPublisherId": nil, "addedDateTime": nil},
			"addIns":                            []any{},
			"defaultRedirectUri":                nil,
			"disabledByMicrosoftStatus":         nil,
			"applicationTemplateId":             nil,
			"samlMetadataUrl":                   nil,
			"certification":                     nil,
			"servicePrincipalLockConfiguration": nil,
		})
		if q.id.Type == "app" {
			o["_owners"] = []any{q.id.ObjectID}
		}
	case entra.ServicePrincipals:
		appID := str(b["appId"])
		app, ok := q.s.Dir.FindBy(q.tid, entra.Applications, "appId", appID)
		if !ok {
			q.err(400, "Request_BadRequest", "The appId '"+appID+"' of the service principal does not reference a valid application object.")
			return
		}
		if _, dup := q.s.Dir.FindBy(q.tid, entra.ServicePrincipals, "appId", appID); dup {
			q.err(409, "Request_MultipleObjectsWithSameKeyValue", "The service principal cannot be created, updated, or restored because the service principal name "+appID+" is already in use.")
			return
		}
		api, _ := app["api"].(map[string]any)
		web, _ := app["web"].(map[string]any)
		spns := []any{appID}
		for _, u := range entra.StringList(app["identifierUris"]) {
			spns = append(spns, u)
		}
		defaults(o, map[string]any{
			"accountEnabled": true, "displayName": app["displayName"], "appDisplayName": app["displayName"], "appDescription": app["description"],
			"appOwnerOrganizationId": q.tid, "appRoleAssignmentRequired": false, "appRoles": app["appRoles"],
			"oauth2PermissionScopes": orAny(api["oauth2PermissionScopes"]), "servicePrincipalNames": spns,
			"servicePrincipalType": "Application", "signInAudience": app["signInAudience"], "tags": []any{},
			"alternativeNames": []any{}, "keyCredentials": []any{}, "passwordCredentials": []any{}, "notificationEmailAddresses": []any{},
			"replyUrls": orAny(web["redirectUris"]), "homepage": web["homePageUrl"], "logoutUrl": web["logoutUrl"], "loginUrl": nil,
			"preferredSingleSignOnMode": nil, "notes": nil, "description": nil, "samlSingleSignOnSettings": nil,
			"info": app["info"], "resourceSpecificApplicationPermissions": []any{}, "verifiedPublisher": app["verifiedPublisher"],
			"addIns": []any{}, "applicationTemplateId": nil, "disabledByMicrosoftStatus": nil, "preferredTokenSigningKeyThumbprint": nil,
		})
		o["passwordCredentials"] = []any{}
		if q.id.Type == "app" {
			o["_owners"] = []any{q.id.ObjectID}
		}
	}

	// owners@odata.bind / members@odata.bind
	for rel, field := range map[string]string{"owners@odata.bind": "_owners", "members@odata.bind": "_members"} {
		if refs := entra.StringList(b[rel]); len(refs) > 0 {
			var ids []any
			for _, ref := range refs {
				rid := lastSeg(ref)
				if _, _, ok := q.s.Dir.FindObject(q.tid, rid); !ok {
					q.err(400, "Request_BadRequest", "Invalid object identifier '"+rid+"'.")
					return
				}
				ids = append(ids, rid)
			}
			if field == "_owners" {
				o[field] = ids
			} else {
				o[field] = ids
			}
		}
	}
	if err := q.s.Dir.Put(q.tid, coll, o); err != nil {
		q.err(500, "InternalServerError", err.Error())
		return
	}
	q.w.Header().Set("Location", q.base()+"/directoryObjects/"+o["id"].(string)+"/"+strings.TrimPrefix(entra.ODataType(coll), "#"))
	q.entity(201, coll, o)
}

func (q *req) update(coll string, o map[string]any) {
	for k, v := range q.body {
		switch {
		case k == "id" || k == "appId" || k == "createdDateTime" || k == "@odata.type" || strings.HasPrefix(k, "_"):
			continue
		case strings.Contains(k, "@odata.bind"):
			continue
		case coll == entra.Users && k == "passwordProfile":
			pp, _ := v.(map[string]any)
			pw := str(pp["password"])
			if pw != "" {
				if !passwordOK(pw) {
					q.err(400, "Request_BadRequest", "The specified password does not comply with password complexity requirements. Please provide a different password.")
					return
				}
				o["_passwordHash"] = entra.HashSecret(pw)
			}
			continue
		case coll == entra.Users && k == "userPrincipalName":
			upn := str(v)
			if other, dup := q.s.Dir.FindBy(q.tid, entra.Users, "userPrincipalName", upn); dup && other["id"] != o["id"] {
				q.err(400, "Request_BadRequest", "Another object with the same value for property userPrincipalName already exists.")
				return
			}
		case (coll == entra.Applications || coll == entra.ServicePrincipals) && (k == "passwordCredentials" || k == "keyCredentials"):
			// secrets are managed through addPassword/removePassword
			if k == "passwordCredentials" {
				continue
			}
		}
		if vm, ok := v.(map[string]any); ok {
			if om, ok := o[k].(map[string]any); ok {
				for kk, vv := range vm {
					om[kk] = vv
				}
				continue
			}
		}
		o[k] = v
	}
	// keep the service principal in sync with app role / display name changes
	if coll == entra.Applications {
		if sp, ok := q.s.Dir.FindBy(q.tid, entra.ServicePrincipals, "appId", str(o["appId"])); ok {
			sp["appRoles"] = o["appRoles"]
			sp["appDisplayName"] = o["displayName"]
			if api, ok := o["api"].(map[string]any); ok {
				sp["oauth2PermissionScopes"] = orAny(api["oauth2PermissionScopes"])
			}
			spns := []any{o["appId"]}
			for _, u := range entra.StringList(o["identifierUris"]) {
				spns = append(spns, u)
			}
			sp["servicePrincipalNames"] = spns
			_ = q.s.Dir.Put(q.tid, entra.ServicePrincipals, sp)
		}
	}
	_ = q.s.Dir.Put(q.tid, coll, o)
	q.w.WriteHeader(204)
}

// ------------------------------------------------------------------ relations

func (q *req) relation(coll string, o map[string]any, field, name string, rest []string) {
	m := q.r.Method
	permColl := coll
	if field == "_members" {
		permColl = "groupMembers"
	}
	ids := entra.StringList(o[field])
	switch {
	case len(rest) == 0 && m == http.MethodGet:
		if !q.require(false, coll) {
			return
		}
		var items []map[string]any
		for _, id := range ids {
			if c, obj, ok := q.s.Dir.FindObject(q.tid, id); ok {
				items = append(items, withType(c, obj))
			}
		}
		q.list("directoryObjects", items)
	case len(rest) == 1 && rest[0] == "$ref" && m == http.MethodGet:
		var items []map[string]any
		for _, id := range ids {
			items = append(items, map[string]any{"@odata.id": q.base() + "/directoryObjects/" + id})
		}
		q.json(200, map[string]any{"value": orEmptyMaps(items)})
	case len(rest) == 1 && rest[0] == "$ref" && m == http.MethodPost:
		if !q.require(true, permColl) || !q.ownedByCheck(coll, o) {
			return
		}
		ref := lastSeg(str(q.body["@odata.id"]))
		if _, _, ok := q.s.Dir.FindObject(q.tid, ref); !ok {
			q.notFound(ref)
			return
		}
		if entra.Contains(ids, ref) {
			q.err(400, "Request_BadRequest", "One or more added object references already exist for the following modified properties: '"+name+"'.")
			return
		}
		o[field] = append(toAny(ids), ref)
		_ = q.s.Dir.Put(q.tid, coll, o)
		q.w.WriteHeader(204)
	case len(rest) == 2 && rest[1] == "$ref" && m == http.MethodDelete:
		if !q.require(true, permColl) || !q.ownedByCheck(coll, o) {
			return
		}
		if !entra.Contains(ids, rest[0]) {
			q.notFound(rest[0])
			return
		}
		if field == "_owners" && len(ids) == 1 && coll == entra.Groups && false {
			return
		}
		o[field] = toAny(entra.Remove(ids, rest[0]))
		_ = q.s.Dir.Put(q.tid, coll, o)
		q.w.WriteHeader(204)
	default:
		q.err(400, "Request_BadRequest", "Unsupported operation on "+name+".")
	}
}

func (q *req) transitiveMembers(groupID string) []map[string]any {
	seen := map[string]bool{}
	var out []map[string]any
	var walk func(string)
	walk = func(gid string) {
		g, ok := q.s.Dir.Get(q.tid, entra.Groups, gid)
		if !ok {
			return
		}
		for _, m := range entra.StringList(g["_members"]) {
			if seen[strings.ToLower(m)] {
				continue
			}
			seen[strings.ToLower(m)] = true
			if c, obj, ok := q.s.Dir.FindObject(q.tid, m); ok {
				out = append(out, withType(c, obj))
				if c == entra.Groups {
					walk(m)
				}
			}
		}
	}
	walk(groupID)
	return out
}

// manager handles users/{id}/manager and manager/$ref.
func (q *req) manager(u map[string]any, rest []string) {
	mid := str(u["_manager"])
	switch {
	case q.r.Method == http.MethodGet && (len(rest) == 0 || rest[0] == "$ref"):
		if !q.require(false, entra.Users) {
			return
		}
		c, m, ok := q.s.Dir.FindObject(q.tid, mid)
		if mid == "" || !ok {
			q.notFound("manager")
			return
		}
		if len(rest) == 1 {
			q.json(200, map[string]any{"@odata.id": q.base() + "/directoryObjects/" + mid})
			return
		}
		q.entity(200, c, withType(c, m))
	case q.r.Method == http.MethodPut && len(rest) == 1 && rest[0] == "$ref":
		if !q.require(true, entra.Users) {
			return
		}
		ref := lastSeg(str(q.body["@odata.id"]))
		if _, _, ok := q.s.Dir.FindObject(q.tid, ref); !ok {
			q.notFound(ref)
			return
		}
		u["_manager"] = ref
		_ = q.s.Dir.Put(q.tid, entra.Users, u)
		q.w.WriteHeader(204)
	case q.r.Method == http.MethodDelete && len(rest) == 1 && rest[0] == "$ref":
		if !q.require(true, entra.Users) {
			return
		}
		if mid == "" {
			q.notFound("manager")
			return
		}
		delete(u, "_manager")
		_ = q.s.Dir.Put(q.tid, entra.Users, u)
		q.w.WriteHeader(204)
	default:
		q.err(400, "Request_BadRequest", "Unsupported operation on manager.")
	}
}

// ------------------------------------------------------------------ passwords

func (q *req) addPassword(coll string, o map[string]any) {
	if q.r.Method != http.MethodPost {
		q.err(405, "Request_BadRequest", "Method not allowed.")
		return
	}
	if !q.require(true, coll) || !q.ownedByCheck(coll, o) {
		return
	}
	pc, _ := q.body["passwordCredential"].(map[string]any)
	if pc == nil {
		pc = map[string]any{}
	}
	now := time.Now().UTC()
	start := str(pc["startDateTime"])
	if start == "" {
		start = now.Format(time.RFC3339)
	}
	end := str(pc["endDateTime"])
	if end == "" {
		end = now.AddDate(2, 0, 0).Format(time.RFC3339)
	}
	secret := entra.NewSecret()
	keyID := entra.NewGUID()
	cred := map[string]any{
		"customKeyIdentifier": nil, "displayName": pc["displayName"], "endDateTime": end, "hint": secret[:3],
		"keyId": keyID, "startDateTime": start,
	}
	o["passwordCredentials"] = append(orAny(o["passwordCredentials"]), cred)
	hashes, _ := o["_secretHashes"].(map[string]any)
	if hashes == nil {
		hashes = map[string]any{}
	}
	hashes[keyID] = entra.HashSecret(secret)
	o["_secretHashes"] = hashes
	_ = q.s.Dir.Put(q.tid, coll, o)
	// service principal secrets authenticate the app as well
	if coll == entra.ServicePrincipals {
		if app, ok := q.s.Dir.FindBy(q.tid, entra.Applications, "appId", str(o["appId"])); ok {
			h, _ := app["_secretHashes"].(map[string]any)
			if h == nil {
				h = map[string]any{}
			}
			h[keyID] = hashes[keyID]
			app["_secretHashes"] = h
			app["_spCredentials"] = append(orAny(app["_spCredentials"]), cred)
			_ = q.s.Dir.Put(q.tid, entra.Applications, app)
		}
	}
	out := map[string]any{}
	for k, v := range cred {
		out[k] = v
	}
	out["secretText"] = secret
	out["@odata.context"] = q.base() + "/$metadata#microsoft.graph.passwordCredential"
	q.json(200, out)
}

func (q *req) removePassword(coll string, o map[string]any) {
	if !q.require(true, coll) || !q.ownedByCheck(coll, o) {
		return
	}
	keyID := str(q.body["keyId"])
	creds := entra.ObjectList(o["passwordCredentials"])
	var keep []any
	found := false
	for _, c := range creds {
		if strings.EqualFold(str(c["keyId"]), keyID) {
			found = true
			continue
		}
		keep = append(keep, c)
	}
	if !found {
		q.err(400, "Request_BadRequest", "No password credential found with keyId '"+keyID+"'.")
		return
	}
	o["passwordCredentials"] = orAny(keep)
	if h, ok := o["_secretHashes"].(map[string]any); ok {
		delete(h, keyID)
	}
	_ = q.s.Dir.Put(q.tid, coll, o)
	q.w.WriteHeader(204)
}

// ------------------------------------------------------------------ federated identity credentials

var ficNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{2,119}$`)

func (q *req) fics(app map[string]any, rest []string) {
	m := q.r.Method
	list := entra.ObjectList(app["_fics"])
	save := func() {
		app["_fics"] = mapsToAny(list)
		_ = q.s.Dir.Put(q.tid, entra.Applications, app)
	}
	ctx := "applications('" + str(app["id"]) + "')/federatedIdentityCredentials"
	if len(rest) == 0 {
		switch m {
		case http.MethodGet:
			if !q.require(false, entra.Applications) {
				return
			}
			q.list(ctx, list)
		case http.MethodPost:
			if !q.require(true, entra.Applications) || !q.ownedByCheck(entra.Applications, app) {
				return
			}
			name := str(q.body["name"])
			if !ficNameRe.MatchString(name) {
				q.err(400, "Request_BadRequest", "Invalid value specified for property 'name' of resource 'FederatedIdentityCredential'.")
				return
			}
			if str(q.body["issuer"]) == "" || str(q.body["subject"]) == "" || len(entra.StringList(q.body["audiences"])) == 0 {
				q.err(400, "Request_BadRequest", "Properties 'issuer', 'subject' and 'audiences' are required.")
				return
			}
			if len(list) >= 20 {
				q.err(400, "Request_BadRequest", "Maximum number of federated identity credentials (20) reached for this application.")
				return
			}
			for _, f := range list {
				if strings.EqualFold(str(f["name"]), name) {
					q.err(409, "Request_MultipleObjectsWithSameKeyValue", "FederatedIdentityCredential with name "+name+" already exists.")
					return
				}
				if str(f["issuer"]) == str(q.body["issuer"]) && str(f["subject"]) == str(q.body["subject"]) {
					q.err(400, "Request_BadRequest", "FederatedIdentityCredential with the same issuer and subject already exists.")
					return
				}
			}
			f := map[string]any{"id": entra.NewGUID(), "name": name, "issuer": q.body["issuer"], "subject": q.body["subject"], "audiences": q.body["audiences"], "description": q.body["description"]}
			list = append(list, f)
			save()
			q.entity(201, ctx, f)
		default:
			q.err(405, "Request_BadRequest", "Method not allowed.")
		}
		return
	}
	idx := -1
	for i, f := range list {
		if strings.EqualFold(str(f["id"]), rest[0]) || strings.EqualFold(str(f["name"]), rest[0]) {
			idx = i
		}
	}
	if idx < 0 {
		if !q.require(false, entra.Applications) {
			return
		}
		q.notFound(rest[0])
		return
	}
	switch m {
	case http.MethodGet:
		if !q.require(false, entra.Applications) {
			return
		}
		q.entity(200, ctx, list[idx])
	case http.MethodPatch:
		if !q.require(true, entra.Applications) || !q.ownedByCheck(entra.Applications, app) {
			return
		}
		for _, k := range []string{"issuer", "subject", "audiences", "description"} {
			if v, ok := q.body[k]; ok {
				list[idx][k] = v
			}
		}
		save()
		q.w.WriteHeader(204)
	case http.MethodDelete:
		if !q.require(true, entra.Applications) || !q.ownedByCheck(entra.Applications, app) {
			return
		}
		list = append(list[:idx], list[idx+1:]...)
		save()
		q.w.WriteHeader(204)
	default:
		q.err(405, "Request_BadRequest", "Method not allowed.")
	}
}

// ------------------------------------------------------------------ helpers

func str(v any) string { s, _ := v.(string); return s }

func lastSeg(s string) string {
	s = strings.TrimSuffix(s, "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	// directoryObjects('id') form
	if i := strings.Index(s, "('"); i >= 0 {
		s = strings.TrimSuffix(s[i+2:], "')")
	}
	return s
}

func defaults(o, d map[string]any) {
	for k, v := range d {
		if _, ok := o[k]; !ok {
			o[k] = v
		}
	}
}

func orAny(v any) []any {
	if a, ok := v.([]any); ok {
		return a
	}
	return []any{}
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func orEmptyMaps(s []map[string]any) []map[string]any {
	if s == nil {
		return []map[string]any{}
	}
	return s
}

func toAny(s []string) []any {
	out := make([]any, len(s))
	for i, x := range s {
		out[i] = x
	}
	return out
}

func mapsToAny(s []map[string]any) []any {
	out := make([]any, len(s))
	for i, x := range s {
		out[i] = x
	}
	return out
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

// withType returns a copy of o carrying its @odata.type. Graph includes the
// type on every object in a polymorphic (directoryObject) response; clients
// such as the azuread provider rely on it to tell users, groups and service
// principals apart.
func withType(coll string, o map[string]any) map[string]any {
	t := entra.ODataType(coll)
	if t == "" {
		return o
	}
	out := make(map[string]any, len(o)+1)
	for k, v := range o {
		out[k] = v
	}
	out["@odata.type"] = t
	return out
}
