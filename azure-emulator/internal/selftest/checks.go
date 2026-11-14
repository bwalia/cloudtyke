package selftest

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
)

const (
	apiRes  = "2022-09-01"
	apiAuth = "2022-04-01"
	apiRD   = "2022-04-01"
	apiAL   = "2015-04-01"
)

const (
	get, put, post, patch, del, head = http.MethodGet, http.MethodPut, http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodHead
)

// Checks maps admin.Service IDs to their check. Every coverage-matrix row
// must have exactly one entry (enforced by the runner and the conformance
// test).
var Checks = map[string]Check{
	// ------------------------------------------------------------ ARM: Microsoft.Resources
	"Microsoft.Resources/subscriptions": {"list filtered by RBAC, get, writes rejected", func(c *C) {
		all := c.Status(c.Do(get, c.ARM("/subscriptions", apiRes), c.Boot("management"), nil), 200, "", "list as Owner").Value()
		c.Expect(len(all) == 3, "Owner should see 3 seeded subscriptions, saw %d", len(all))
		mine := c.Status(c.Do(get, c.ARM("/subscriptions", apiRes), c.Reader("management"), nil), 200, "", "list as Reader").Value()
		c.Expect(len(mine) == 1 && HasID(mine, "subscriptionId", SubSandbox), "Reader should only see sub-sandbox, saw %v", mine)
		s := c.Status(c.Do(get, c.ARM("/subscriptions/"+SubSandbox, apiRes), c.Reader("management"), nil), 200, "", "get sandbox")
		c.Expect(s.Str("displayName") == "sub-sandbox" && s.Str("state") == "Enabled", "unexpected subscription body %s", short(s.Raw))
		c.Status(c.Do(put, c.ARM("/subscriptions/"+SubSandbox, apiRes), c.Boot("management"), map[string]any{}), 405, "MethodNotAllowed", "PUT subscription")
	}},
	"Microsoft.Resources/tenants": {"lists the caller's home tenant", func(c *C) {
		ts := c.Status(c.Do(get, c.ARM("/tenants", apiRes), c.Boot("management"), nil), 200, "", "list tenants").Value()
		c.Expect(len(ts) == 1 && HasID(ts, "tenantId", Tenant), "expected the seeded tenant, got %v", ts)
	}},
	"Microsoft.Resources/locations": {"curated region list with zone mappings", func(c *C) {
		ls := c.Status(c.Do(get, c.ARM("/subscriptions/"+SubSandbox+"/locations", apiRes), c.Reader("management"), nil), 200, "", "list locations").Value()
		c.Expect(len(ls) >= 5 && HasID(ls, "name", "uksouth") && HasID(ls, "name", "westeurope"), "expected uksouth and westeurope among %d regions", len(ls))
	}},
	"Microsoft.Resources/providers": {"list, get, register/unregister, unknown namespace", func(c *C) {
		tok := c.Boot("management")
		base := "/subscriptions/" + SubConn + "/providers"
		ps := c.Status(c.Do(get, c.ARM(base, apiRes), tok, nil), 200, "", "list providers").Value()
		c.Expect(HasID(ps, "namespace", "Microsoft.Authorization"), "Microsoft.Authorization should be listed")
		p := c.Status(c.Do(get, c.ARM(base+"/microsoft.authorization", apiRes), tok, nil), 200, "", "get provider (case-insensitive)")
		c.Expect(p.Str("namespace") == "Microsoft.Authorization" && p.Str("registrationState") == "Registered", "unexpected provider %s", short(p.Raw))
		u := c.Status(c.Do(post, c.ARM(base+"/Microsoft.Authorization/unregister", apiRes), tok, nil), 200, "", "unregister")
		c.Expect(u.Str("registrationState") == "Unregistered", "unregister should report Unregistered")
		r := c.Status(c.Do(post, c.ARM(base+"/Microsoft.Authorization/register", apiRes), tok, nil), 200, "", "register")
		c.Expect(r.Str("registrationState") == "Registered", "register should report Registered")
		c.Status(c.Do(get, c.ARM(base+"/Microsoft.Nope", apiRes), tok, nil), 404, "InvalidResourceNamespace", "unknown namespace")
		c.Status(c.Do(post, c.ARM("/subscriptions/"+SubSandbox+"/providers/Microsoft.Authorization/register", apiRes), c.Reader("management"), nil), 403, "AuthorizationFailed", "Reader cannot register")
	}},
	"Microsoft.Resources/resourceGroups": {"create, idempotent PUT, HEAD, tags, RBAC, async delete", func(c *C) {
		tok := c.Boot("management")
		name := c.Name("rg")
		u := c.ARM("/subscriptions/"+SubSandbox+"/resourcegroups/"+name, apiRes)
		c.Status(c.Do(get, strings.Split(u, "?")[0], tok, nil), 400, "MissingApiVersionParameter", "missing api-version")
		c.Status(c.Do(get, u, tok, nil), 404, "ResourceGroupNotFound", "get before create")
		c.Status(c.Do(head, u, tok, nil), 404, "", "HEAD before create")
		c.Status(c.Do(put, u, tok, map[string]any{"location": "Mars Central"}), 400, "LocationNotAvailableForResourceGroup", "bad location")
		rg := c.Status(c.Do(put, u, tok, map[string]any{"location": "UK South", "tags": map[string]any{"env": "selftest"}}), 201, "", "create")
		c.Expect(rg.Str("location") == "uksouth" && rg.Str("properties.provisioningState") == "Succeeded", "unexpected body %s", short(rg.Raw))
		c.Status(c.Do(put, u, tok, map[string]any{"location": "uksouth", "tags": map[string]any{"env": "selftest"}}), 200, "", "idempotent PUT")
		c.Status(c.Do(head, u, tok, nil), 204, "", "HEAD after create")
		p := c.Status(c.Do(patch, u, tok, map[string]any{"tags": map[string]any{"env": "selftest", "owner": "ct"}}), 200, "", "PATCH tags")
		c.Expect(p.Str("tags.owner") == "ct", "PATCH should merge tags")
		f := url.QueryEscape("tagName eq 'owner' and tagValue eq 'ct'")
		l := c.Status(c.Do(get, c.ARM("/subscriptions/"+SubSandbox+"/resourcegroups?$filter="+f, apiRes), tok, nil), 200, "", "list with tag filter").Value()
		c.Expect(len(l) == 1 && HasID(l, "name", name), "tag filter should return only %s, got %d", name, len(l))
		c.Status(c.Do(put, u, c.Reader("management"), map[string]any{"location": "uksouth"}), 403, "AuthorizationFailed", "Reader cannot write")
		c.Status(c.Do(get, u, c.Reader("management"), nil), 200, "", "Reader can read")
		d := c.Status(c.Do(del, u, tok, nil), 202, "", "delete")
		c.Expect(d.Header.Get("Location") != "", "delete must return a Location header")
		c.Status(c.WaitAsync(d.Header.Get("Location"), tok), 200, "", "delete operation result")
		c.Status(c.Do(get, u, tok, nil), 404, "ResourceGroupNotFound", "get after delete")
	}},
	"Microsoft.Resources/resources": {"list at subscription and resource group scope, $filter", func(c *C) {
		tok := c.Boot("management")
		name := c.Name("rg")
		c.Status(c.Do(put, c.ARM("/subscriptions/"+SubSandbox+"/resourcegroups/"+name, apiRes), tok, map[string]any{"location": "uksouth"}), 201, "", "create rg")
		c.Status(c.Do(get, c.ARM("/subscriptions/"+SubSandbox+"/resources", apiRes), tok, nil), 200, "", "list subscription resources")
		c.Status(c.Do(get, c.ARM("/subscriptions/"+SubSandbox+"/resourceGroups/"+name+"/resources", apiRes), c.Reader("management"), nil), 200, "", "Reader lists rg resources")
		f := url.QueryEscape("resourceType eq 'Microsoft.Web/sites'")
		r := c.Status(c.Do(get, c.ARM("/subscriptions/"+SubSandbox+"/resources?$filter="+f, apiRes), tok, nil), 200, "", "list with resourceType filter")
		c.Expect(len(r.Value()) == 0, "no Microsoft.Web/sites exist, got %d", len(r.Value()))
		c.Status(c.Do(get, c.ARM("/subscriptions/"+SubSandbox+"/resourceGroups/rg-does-not-exist/resources", apiRes), tok, nil), 404, "ResourceGroupNotFound", "unknown rg")
		c.Status(c.Do(get, c.ARM("/subscriptions/"+SubConn+"/resources", apiRes), c.Reader("management"), nil), 403, "AuthorizationFailed", "Reader outside its subscription")
		c.Status(c.Do(get, c.ARM("/subscriptions/"+SubSandbox+"/providers/Microsoft.Web/sites", "2023-01-01"), tok, nil), 501, "NotImplemented", "unemulated type returns 501")
	}},

	// ------------------------------------------------------------ ARM: Microsoft.Authorization
	"Microsoft.Authorization/roleDefinitions": {"built-ins, filters, custom role lifecycle", func(c *C) {
		tok := c.Boot("management")
		base := "/subscriptions/" + SubSandbox + "/providers/Microsoft.Authorization/roleDefinitions"
		f := url.QueryEscape("roleName eq 'Reader'")
		r := c.Status(c.Do(get, c.ARM(base+"?$filter="+f, apiRD), tok, nil), 200, "", "filter by roleName").Value()
		c.Expect(len(r) == 1 && HasID(r, "name", RoleReader), "roleName filter should return the Reader built-in, got %v", r)
		bi := c.Status(c.Do(get, c.ARM(base+"?$filter="+url.QueryEscape("type eq 'BuiltInRole'"), apiRD), tok, nil), 200, "", "filter by type").Value()
		c.Expect(len(bi) >= 3, "expected built-in roles, got %d", len(bi))
		c.Status(c.Do(get, c.ARM(base+"/"+RoleContributor, apiRD), tok, nil), 200, "", "get Contributor")
		c.Status(c.Do(put, c.ARM(base+"/"+RoleReader, apiRD), tok, map[string]any{"properties": map[string]any{"roleName": "Reader"}}), 403, "RoleDefinitionUpdateNotPermitted", "update built-in")

		id := entra.NewGUID()
		name := c.Name("Self Test Role")
		body := map[string]any{"properties": map[string]any{
			"roleName": name, "description": "self-test", "type": "CustomRole",
			"assignableScopes": []string{"/subscriptions/" + SubSandbox},
			"permissions":      []any{map[string]any{"actions": []string{"Microsoft.Resources/subscriptions/resourceGroups/read"}, "notActions": []string{}}},
		}}
		cr := c.Do(put, c.ARM(base+"/"+id, apiRD), tok, body)
		c.Expect(cr.Code == 201 || cr.Code == 200, "create custom role: got %d %s", cr.Code, short(cr.Raw))
		c.Expect(cr.Str("properties.type") == "CustomRole", "custom role type should be CustomRole")
		dup := body["properties"].(map[string]any)
		c.Status(c.Do(put, c.ARM(base+"/"+entra.NewGUID(), apiRD), tok, map[string]any{"properties": dup}), 409, "RoleDefinitionWithSameNameExists", "duplicate name")
		c.Status(c.Do(put, c.ARM(base+"/"+entra.NewGUID(), apiRD), tok, map[string]any{"properties": map[string]any{"roleName": c.Name("x")}}), 400, "MissingAssignableScopes", "missing assignableScopes")
		d := c.Do(del, c.ARM(base+"/"+id, apiRD), tok, nil)
		c.Expect(d.Code == 200 || d.Code == 204, "delete custom role: got %d", d.Code)
		c.Status(c.Do(get, c.ARM(base+"/"+id, apiRD), tok, nil), 404, "RoleDefinitionDoesNotExist", "get after delete")
	}},
	"Microsoft.Authorization/roleAssignments": {"assign, inherit, filter, duplicates, principal validation, enforcement", func(c *C) {
		tok := c.Boot("management")
		rg := c.Name("rg")
		rgPath := "/subscriptions/" + SubSandbox + "/resourceGroups/" + rg
		c.Status(c.Do(put, c.ARM(rgPath, apiRes), tok, map[string]any{"location": "uksouth"}), 201, "", "create rg")
		// Reader cannot write until granted Contributor on the rg
		c.Status(c.Do(patch, c.ARM(rgPath, apiRes), c.Reader("management"), map[string]any{"tags": map[string]any{"a": "b"}}), 403, "AuthorizationFailed", "Reader write before grant")
		raID := entra.NewGUID()
		ra := rgPath + "/providers/Microsoft.Authorization/roleAssignments/"
		props := map[string]any{"properties": map[string]any{
			"roleDefinitionId": "/subscriptions/" + SubSandbox + "/providers/Microsoft.Authorization/roleDefinitions/" + RoleContributor,
			"principalId":      ReaderSPID,
		}}
		a := c.Status(c.Do(put, c.ARM(ra+raID, apiAuth), tok, props), 201, "", "create assignment")
		c.Expect(a.Str("properties.principalType") == "ServicePrincipal", "principalType should be inferred as ServicePrincipal")
		c.Status(c.Do(put, c.ARM(ra+entra.NewGUID(), apiAuth), tok, props), 409, "RoleAssignmentExists", "duplicate assignment")
		c.Status(c.Do(patch, c.ARM(rgPath, apiRes), c.Reader("management"), map[string]any{"tags": map[string]any{"a": "b"}}), 200, "", "Reader write after grant (enforced RBAC)")

		f := url.QueryEscape("principalId eq '" + ReaderSPID + "'")
		l := c.Status(c.Do(get, c.ARM(rgPath+"/providers/Microsoft.Authorization/roleAssignments?$filter="+f, apiAuth), tok, nil), 200, "", "filter principalId").Value()
		c.Expect(HasID(l, "name", raID), "principalId filter should include the new assignment")
		inh := c.Status(c.Do(get, c.ARM(rgPath+"/providers/Microsoft.Authorization/roleAssignments", apiAuth), tok, nil), 200, "", "list at rg").Value()
		c.Expect(len(inh) > len(l), "list at rg should include inherited assignments (Owner at /)")
		at := c.Status(c.Do(get, c.ARM(rgPath+"/providers/Microsoft.Authorization/roleAssignments?$filter="+url.QueryEscape("atScope()"), apiAuth), tok, nil), 200, "", "atScope()").Value()
		c.Expect(HasID(at, "name", raID), "atScope() should include the rg assignment")

		c.Status(c.Do(put, c.ARM(ra+"not-a-guid", apiAuth), tok, props), 400, "InvalidRoleAssignmentId", "non-GUID name")
		bad := map[string]any{"properties": map[string]any{"roleDefinitionId": "/providers/Microsoft.Authorization/roleDefinitions/" + RoleReader, "principalId": "00000000-0000-4000-8000-00000000dead"}}
		c.Status(c.Do(put, c.ARM(ra+entra.NewGUID(), apiAuth), tok, bad), 400, "PrincipalNotFound", "unknown principal")
		mism := map[string]any{"properties": map[string]any{"roleDefinitionId": "/providers/Microsoft.Authorization/roleDefinitions/" + RoleReader, "principalId": ReaderSPID, "principalType": "User"}}
		c.Status(c.Do(put, c.ARM(ra+entra.NewGUID(), apiAuth), tok, mism), 400, "UnmatchedPrincipalType", "wrong principalType")

		c.Status(c.Do(del, c.ARM(ra+raID, apiAuth), tok, nil), 200, "", "delete assignment")
		c.Status(c.Do(get, c.ARM(ra+raID, apiAuth), tok, nil), 404, "RoleAssignmentNotFound", "get after delete")
		c.Status(c.Do(patch, c.ARM(rgPath, apiRes), c.Reader("management"), map[string]any{"tags": map[string]any{"a": "c"}}), 403, "AuthorizationFailed", "Reader write after revoke")
	}},
	"Microsoft.Authorization/permissions": {"effective permissions of the caller", func(c *C) {
		u := c.ARM("/subscriptions/"+SubSandbox+"/providers/Microsoft.Authorization/permissions", apiAuth)
		r := c.Status(c.Do(get, u, c.Reader("management"), nil), 200, "", "Reader permissions").Value()
		c.Expect(hasAction(r, "*/read"), "Reader should have */read, got %v", r)
		c.Expect(!hasAction(r, "*"), "Reader must not have *")
		o := c.Status(c.Do(get, u, c.Boot("management"), nil), 200, "", "Owner permissions").Value()
		c.Expect(hasAction(o, "*"), "Owner should have *")
		c.Status(c.Do(get, c.ARM("/subscriptions/"+SubConn+"/providers/Microsoft.Authorization/permissions", apiAuth), c.Reader("management"), nil), 403, "AuthorizationFailed", "Reader outside its scope")
		c.Status(c.Do(post, u, c.Boot("management"), nil), 405, "MethodNotAllowed", "POST permissions")
	}},

	// ------------------------------------------------------------ ARM: Microsoft.Insights
	"Microsoft.Insights/eventtypes/values": {"activity log records writes, $filter", func(c *C) {
		tok := c.Boot("management")
		rg := c.Name("rg")
		u := c.ARM("/subscriptions/"+SubSandbox+"/resourcegroups/"+rg, apiRes)
		c.Status(c.Do(put, u, tok, map[string]any{"location": "uksouth"}), 201, "", "create rg")
		c.Status(c.Do(patch, u, tok, map[string]any{"tags": map[string]any{"x": "y"}}), 200, "", "patch rg")
		f := url.QueryEscape("resourceGroupName eq '" + rg + "'")
		ev := c.Status(c.Do(get, c.ARM("/subscriptions/"+SubSandbox+"/providers/Microsoft.Insights/eventtypes/management/values?$filter="+f, apiAL), tok, nil), 200, "", "query activity log").Value()
		c.Expect(len(ev) >= 2, "expected events for the rg writes, got %d", len(ev))
		e := ev[0]
		c.Expect(dig(e, "caller") != nil && dig(e, "operationName") != nil && dig(e, "status") != nil, "event missing caller/operationName/status: %v", e)
		other := c.Status(c.Do(get, c.ARM("/subscriptions/"+SubSandbox+"/providers/Microsoft.Insights/eventtypes/management/values?$filter="+url.QueryEscape("resourceGroupName eq 'nope-"+rg+"'"), apiAL), tok, nil), 200, "", "filter no match").Value()
		c.Expect(len(other) == 0, "filter for another rg should be empty, got %d", len(other))
		c.Status(c.Do(get, c.ARM("/subscriptions/"+SubSandbox+"/providers/Microsoft.Insights/eventtypes/other/values", apiAL), tok, nil), 400, "BadRequest", "wrong event type")
	}},

	// ------------------------------------------------------------ Token service
	"token": {"client secret, v1 + v2 endpoints, claims, error codes", func(c *C) {
		tok := c.Boot("management")
		cl := Claims(tok)
		c.Expect(cl["tid"] == Tenant && cl["appid"] == BootAppID && cl["oid"] == BootSPID, "unexpected claims %v", cl)
		g := Claims(c.Boot("graph"))
		c.Expect(containsStr(g["roles"], "Directory.ReadWrite.All"), "Graph token should carry app roles, got %v", g["roles"])
		ep := c.URL("login", "/"+Tenant+"/oauth2/v2.0/token")
		c.Status(c.Form(ep, url.Values{"client_id": {BootAppID}, "client_secret": {"wrong"}, "grant_type": {"client_credentials"}, "scope": {c.URL("management", "/.default")}}), 401, "invalid_client", "wrong secret")
		c.Status(c.Form(ep, url.Values{"client_id": {BootAppID}, "client_secret": {BootSecret}, "grant_type": {"client_credentials"}, "scope": {"no-default-suffix"}}), 400, "invalid_scope", "scope without /.default")
		c.Status(c.Form(ep, url.Values{"client_id": {BootAppID}, "client_assertion": {"x.y.z"}, "client_assertion_type": {"wrong"}, "grant_type": {"client_credentials"}, "scope": {c.URL("management", "/.default")}}), 400, "invalid_request", "bad client_assertion_type")
		c.Status(c.Form(c.URL("login", "/00000000-0000-4000-8000-0000000000ff/oauth2/v2.0/token"), url.Values{"client_id": {BootAppID}, "client_secret": {BootSecret}, "grant_type": {"client_credentials"}, "scope": {c.URL("management", "/.default")}}), 400, "invalid_request", "unknown tenant")
		v1 := c.Status(c.Form(c.URL("login", "/"+Tenant+"/oauth2/token"), url.Values{"client_id": {BootAppID}, "client_secret": {BootSecret}, "grant_type": {"client_credentials"}, "resource": {c.URL("management", "/")}}), 200, "", "v1 endpoint with resource=")
		c.Expect(v1.Body["access_token"] != nil, "v1 response missing access_token")
		c.Status(c.Do(get, c.ARM("/subscriptions", apiRes), c.Boot("graph"), nil), 401, "InvalidAuthenticationTokenAudience", "Graph token used on ARM")
		c.Status(c.Do(get, c.ARM("/subscriptions", apiRes), "", nil), 401, "", "no token on ARM")
	}},
	"oidc-discovery": {"OpenID configuration and JWKS signing keys", func(c *C) {
		d := c.Status(c.Do(get, c.URL("login", "/"+Tenant+"/v2.0/.well-known/openid-configuration"), "", nil), 200, "", "v2 discovery")
		c.Expect(strings.Contains(d.Str("issuer"), Tenant) && strings.HasSuffix(d.Str("token_endpoint"), "/oauth2/v2.0/token"), "unexpected discovery %s", short(d.Raw))
		jwks := d.Str("jwks_uri")
		c.Expect(jwks != "", "jwks_uri missing")
		k := c.Status(c.Do(get, jwks, "", nil), 200, "", "JWKS")
		keys, _ := k.Body["keys"].([]any)
		c.Expect(len(keys) >= 1, "JWKS has no keys")
		kid := jwtHeaderKid(c.Boot("management"))
		found := false
		for _, kk := range keys {
			if m, _ := kk.(map[string]any); m["kid"] == kid && m["kty"] == "RSA" {
				found = true
			}
		}
		c.Expect(found, "token kid %q not published in JWKS", kid)
		v1 := c.Status(c.Do(get, c.URL("login", "/"+Tenant+"/.well-known/openid-configuration"), "", nil), 200, "", "v1 discovery")
		c.Expect(strings.HasSuffix(v1.Str("token_endpoint"), "/oauth2/token"), "v1 token endpoint expected")
	}},
	"instance-discovery": {"MSAL instance discovery", func(c *C) {
		r := c.Status(c.Do(get, c.URL("login", "/common/discovery/instance?api-version=1.1&authorization_endpoint="+url.QueryEscape(c.URL("login", "/common/oauth2/authorize"))), "", nil), 200, "", "instance discovery")
		c.Expect(strings.Contains(r.Str("tenant_discovery_endpoint"), "/.well-known/openid-configuration"), "tenant_discovery_endpoint missing: %s", short(r.Raw))
	}},

	// ------------------------------------------------------------ Graph
	"users": {"CRUD, UPN lookup, OData, paging, manager, password + domain rules", func(c *C) {
		tok := c.Boot("graph")
		n := c.Name("user")
		upn := n + "@cloudtyke.local"
		mk := func(upn, pw string) Resp {
			return c.Do(post, c.Graph("/users"), tok, map[string]any{"accountEnabled": true, "displayName": n, "mailNickname": n, "userPrincipalName": upn, "passwordProfile": map[string]any{"password": pw}})
		}
		c.Status(mk(n+"@example.com", "Sup3r-secret!"), 400, "", "unverified domain")
		c.Status(mk(upn, "weak"), 400, "", "weak password")
		u := c.Status(mk(upn, "Sup3r-secret!"), 201, "", "create user")
		id := u.Str("id")
		c.Expect(u.Body["passwordProfile"] == nil, "passwordProfile must never be returned")
		c.Status(c.Do(get, c.Graph("/users/"+id), tok, nil), 200, "", "get by id")
		c.Status(c.Do(get, c.Graph("/users/"+upn), tok, nil), 200, "", "get by UPN")
		c.Status(c.Do(patch, c.Graph("/users/"+id), tok, map[string]any{"jobTitle": "Platform Engineer"}), 204, "", "PATCH")
		g := c.Do(get, c.Graph("/users/"+id+"?$select=id,jobTitle"), tok, nil)
		c.Expect(g.Str("jobTitle") == "Platform Engineer" && g.Body["displayName"] == nil, "$select / PATCH not applied: %s", short(g.Raw))
		f := c.Status(c.Do(get, c.Graph("/users?$count=true&$filter="+url.QueryEscape("startswith(userPrincipalName,'"+n+"')")), tok, nil), 200, "", "$filter startswith")
		c.Expect(len(f.Value()) == 1 && f.Body["@odata.count"] == float64(1), "filter should match exactly one user: %s", short(f.Raw))
		pg := c.Status(c.Do(get, c.Graph("/users?$top=1"), tok, nil), 200, "", "$top=1")
		next, _ := pg.Body["@odata.nextLink"].(string)
		c.Expect(len(pg.Value()) == 1 && next != "", "paging should return one item and a nextLink")
		c.Status(c.Do(get, next, tok, nil), 200, "", "follow nextLink")
		// manager / directReports
		c.Status(c.Do(get, c.Graph("/users/"+id+"/manager"), tok, nil), 404, "Request_ResourceNotFound", "no manager yet")
		c.Status(c.Do(put, c.Graph("/users/"+id+"/manager/$ref"), tok, map[string]any{"@odata.id": c.Graph("/directoryObjects/" + AdaUserID)}), 204, "", "set manager")
		m := c.Status(c.Do(get, c.Graph("/users/"+id+"/manager"), tok, nil), 200, "", "get manager")
		c.Expect(m.Str("id") == AdaUserID, "manager should be Ada")
		dr := c.Status(c.Do(get, c.Graph("/users/"+AdaUserID+"/directReports"), tok, nil), 200, "", "directReports").Value()
		c.Expect(HasID(dr, "id", id), "Ada's directReports should include the new user")
		c.Status(c.Do(get, c.Graph("/users/"+id+"/memberOf"), tok, nil), 200, "", "memberOf")
		c.Status(mk(upn, "Sup3r-secret!"), 400, "", "duplicate UPN")
		c.Status(c.Do(post, c.Graph("/users"), c.Reader("graph"), map[string]any{"displayName": "x"}), 403, "Authorization_RequestDenied", "Reader cannot create users")
		c.Status(c.Do(del, c.Graph("/users/"+id), tok, nil), 204, "", "delete")
		c.Status(c.Do(get, c.Graph("/users/"+id), tok, nil), 404, "Request_ResourceNotFound", "get after delete")
	}},
	"groups": {"create with members@odata.bind, owners/members $ref, nesting, transitive", func(c *C) {
		tok := c.Boot("graph")
		mk := func(extra map[string]any) Resp {
			n := c.Name("grp")
			b := map[string]any{"displayName": n, "mailEnabled": false, "mailNickname": n, "securityEnabled": true}
			for k, v := range extra {
				b[k] = v
			}
			return c.Status(c.Do(post, c.Graph("/groups"), tok, b), 201, "", "create group")
		}
		inner := mk(map[string]any{"members@odata.bind": []string{c.Graph("/directoryObjects/" + AdaUserID)}})
		outer := mk(nil)
		iid, oid := inner.Str("id"), outer.Str("id")
		m := c.Status(c.Do(get, c.Graph("/groups/"+iid+"/members"), tok, nil), 200, "", "list members").Value()
		c.Expect(len(m) == 1 && HasID(m, "id", AdaUserID), "inner group should contain Ada")
		c.Expect(m[0]["@odata.type"] == "#microsoft.graph.user", "members must carry @odata.type, got %v", m[0]["@odata.type"])
		ref := map[string]any{"@odata.id": c.Graph("/directoryObjects/" + iid)}
		c.Status(c.Do(post, c.Graph("/groups/"+oid+"/members/$ref"), tok, ref), 204, "", "nest inner in outer")
		c.Status(c.Do(post, c.Graph("/groups/"+oid+"/members/$ref"), tok, ref), 400, "Request_BadRequest", "duplicate member")
		tm := c.Status(c.Do(get, c.Graph("/groups/"+oid+"/transitiveMembers"), tok, nil), 200, "", "transitiveMembers").Value()
		c.Expect(HasID(tm, "id", AdaUserID) && HasID(tm, "id", iid), "transitiveMembers should include the nested group and Ada")
		tmo := c.Status(c.Do(get, c.Graph("/users/"+AdaUserID+"/transitiveMemberOf"), tok, nil), 200, "", "transitiveMemberOf").Value()
		c.Expect(HasID(tmo, "id", oid), "Ada should transitively belong to the outer group")
		c.Status(c.Do(post, c.Graph("/groups/"+oid+"/owners/$ref"), tok, map[string]any{"@odata.id": c.Graph("/directoryObjects/" + AdaUserID)}), 204, "", "add owner")
		ow := c.Status(c.Do(get, c.Graph("/groups/"+oid+"/owners"), tok, nil), 200, "", "list owners").Value()
		c.Expect(HasID(ow, "id", AdaUserID), "owners should include Ada")
		c.Status(c.Do(del, c.Graph("/groups/"+oid+"/members/"+iid+"/$ref"), tok, nil), 204, "", "remove member")
		c.Status(c.Do(get, c.Graph("/groups?$filter="+url.QueryEscape("id eq '"+oid+"'")+"&$select=id,displayName"), tok, nil), 200, "", "$filter by id")
		c.Status(c.Do(post, c.Graph("/groups"), c.Reader("graph"), map[string]any{"displayName": "x", "mailEnabled": false, "mailNickname": "x", "securityEnabled": true}), 403, "Authorization_RequestDenied", "Reader cannot create groups")
		c.Status(c.Do(del, c.Graph("/groups/"+oid), tok, nil), 204, "", "delete outer")
		c.Status(c.Do(del, c.Graph("/groups/"+iid), tok, nil), 204, "", "delete inner")
	}},
	"applications": {"CRUD, alternate key, secrets that authenticate, federated credentials", func(c *C) {
		tok := c.Boot("graph")
		a := c.Status(c.Do(post, c.Graph("/applications"), tok, map[string]any{"displayName": c.Name("app")}), 201, "", "create application")
		id, appID := a.Str("id"), a.Str("appId")
		c.Status(c.Do(get, c.Graph("/applications(appId='"+appID+"')"), tok, nil), 200, "", "alternate key")
		c.Status(c.Do(patch, c.Graph("/applications/"+id), tok, map[string]any{"signInAudience": "AzureADMyOrg", "notes": "self-test"}), 204, "", "PATCH")
		c.Status(c.Do(post, c.Graph("/servicePrincipals"), tok, map[string]any{"appId": appID}), 201, "", "create service principal")
		pw := c.Status(c.Do(post, c.Graph("/applications/"+id+"/addPassword"), tok, map[string]any{"passwordCredential": map[string]any{"displayName": "ci"}}), 200, "", "addPassword")
		secret, keyID := pw.Str("secretText"), pw.Str("keyId")
		c.Expect(secret != "" && keyID != "", "addPassword must return secretText and keyId")
		c.Token(appID, secret, "management") // the new secret authenticates
		c.Status(c.Do(post, c.Graph("/applications/"+id+"/removePassword"), tok, map[string]any{"keyId": keyID}), 204, "", "removePassword")
		c.Status(c.Form(c.URL("login", "/"+Tenant+"/oauth2/v2.0/token"), url.Values{"client_id": {appID}, "client_secret": {secret}, "grant_type": {"client_credentials"}, "scope": {c.URL("management", "/.default")}}), 401, "invalid_client", "removed secret no longer works")
		fic := map[string]any{"name": "github-main", "issuer": "https://token.actions.githubusercontent.com", "subject": "repo:example/app:ref:refs/heads/main", "audiences": []string{"api://AzureADTokenExchange"}}
		c.Status(c.Do(post, c.Graph("/applications/"+id+"/federatedIdentityCredentials"), tok, fic), 201, "", "create federated credential")
		c.Status(c.Do(post, c.Graph("/applications/"+id+"/federatedIdentityCredentials"), tok, fic), 409, "Request_MultipleObjectsWithSameKeyValue", "duplicate federated credential")
		fl := c.Status(c.Do(get, c.Graph("/applications/"+id+"/federatedIdentityCredentials"), tok, nil), 200, "", "list federated credentials").Value()
		c.Expect(len(fl) == 1, "expected 1 federated credential, got %d", len(fl))
		c.Status(c.Do(post, c.Graph("/applications/"+id+"/owners/$ref"), tok, map[string]any{"@odata.id": c.Graph("/directoryObjects/" + AdaUserID)}), 204, "", "add owner")
		c.Status(c.Do(post, c.Graph("/applications"), c.Reader("graph"), map[string]any{"displayName": "x"}), 403, "Authorization_RequestDenied", "Reader cannot create applications")
		c.Status(c.Do(del, c.Graph("/applications/"+id), tok, nil), 204, "", "delete application")
		c.Status(c.Do(get, c.Graph("/servicePrincipals(appId='"+appID+"')"), tok, nil), 404, "Request_ResourceNotFound", "service principal deleted with its application")
	}},
	"servicePrincipals": {"CRUD, alternate key, app role assignment grants Graph permissions", func(c *C) {
		tok := c.Boot("graph")
		a := c.Status(c.Do(post, c.Graph("/applications"), tok, map[string]any{"displayName": c.Name("sp")}), 201, "", "create application")
		appID := a.Str("appId")
		sp := c.Status(c.Do(post, c.Graph("/servicePrincipals"), tok, map[string]any{"appId": appID}), 201, "", "create service principal")
		spID := sp.Str("id")
		c.Status(c.Do(get, c.Graph("/servicePrincipals(appId='"+appID+"')"), tok, nil), 200, "", "alternate key")
		c.Status(c.Do(patch, c.Graph("/servicePrincipals/"+spID), tok, map[string]any{"notes": "self-test"}), 204, "", "PATCH")
		pw := c.Status(c.Do(post, c.Graph("/servicePrincipals/"+spID+"/addPassword"), tok, map[string]any{"passwordCredential": map[string]any{"displayName": "sp"}}), 200, "", "addPassword on SP")
		secret := pw.Str("secretText")
		// without permissions the new principal cannot read users
		c.Status(c.Do(get, c.Graph("/users"), c.Token(appID, secret, "graph"), nil), 403, "Authorization_RequestDenied", "no Graph permission yet")
		gsp := c.Status(c.Do(get, c.Graph("/servicePrincipals/"+GraphSPID), tok, nil), 200, "", "get Microsoft Graph SP")
		roleID := appRoleID(gsp.Body, "User.Read.All")
		c.Expect(roleID != "", "Microsoft Graph SP should expose the User.Read.All app role")
		as := c.Status(c.Do(post, c.Graph("/servicePrincipals/"+GraphSPID+"/appRoleAssignedTo"), tok, map[string]any{"principalId": spID, "resourceId": GraphSPID, "appRoleId": roleID}), 201, "", "grant User.Read.All")
		mine := c.Status(c.Do(get, c.Graph("/servicePrincipals/"+spID+"/appRoleAssignments"), tok, nil), 200, "", "appRoleAssignments").Value()
		c.Expect(HasID(mine, "appRoleId", roleID), "principal should list its app role assignment")
		c.Status(c.Do(post, c.Graph("/servicePrincipals/"+GraphSPID+"/appRoleAssignedTo"), tok, map[string]any{"principalId": spID, "resourceId": GraphSPID, "appRoleId": roleID}), 400, "Request_BadRequest", "duplicate grant")
		// fresh token now carries the role
		delete(c.toks, appID+"|"+secret+"|graph")
		gt := c.Token(appID, secret, "graph")
		c.Expect(containsStr(Claims(gt)["roles"], "User.Read.All"), "token should carry User.Read.All")
		c.Status(c.Do(get, c.Graph("/users"), gt, nil), 200, "", "read users with granted role")
		c.Status(c.Do(post, c.Graph("/groups"), gt, map[string]any{"displayName": "x"}), 403, "Authorization_RequestDenied", "write still denied")
		c.Status(c.Do(del, c.Graph("/servicePrincipals/"+GraphSPID+"/appRoleAssignedTo/"+as.Str("id")), tok, nil), 204, "", "revoke grant")
		c.Status(c.Do(del, c.Graph("/servicePrincipals/"+spID), tok, nil), 204, "", "delete service principal")
	}},
	"oauth2PermissionGrants": {"delegated permission grant lifecycle", func(c *C) {
		tok := c.Boot("graph")
		a := c.Status(c.Do(post, c.Graph("/applications"), tok, map[string]any{"displayName": c.Name("grant")}), 201, "", "create application")
		sp := c.Status(c.Do(post, c.Graph("/servicePrincipals"), tok, map[string]any{"appId": a.Str("appId")}), 201, "", "create service principal")
		spID := sp.Str("id")
		c.Status(c.Do(post, c.Graph("/oauth2PermissionGrants"), tok, map[string]any{"clientId": spID, "consentType": "AllPrincipals"}), 400, "Request_BadRequest", "missing resourceId/scope")
		g := c.Status(c.Do(post, c.Graph("/oauth2PermissionGrants"), tok, map[string]any{"clientId": spID, "consentType": "AllPrincipals", "resourceId": GraphSPID, "scope": "User.Read"}), 201, "", "create grant")
		gid := g.Str("id")
		c.Status(c.Do(patch, c.Graph("/oauth2PermissionGrants/"+gid), tok, map[string]any{"scope": "User.Read Group.Read.All"}), 204, "", "update scope")
		r := c.Status(c.Do(get, c.Graph("/oauth2PermissionGrants/"+gid), tok, nil), 200, "", "get grant")
		c.Expect(r.Str("scope") == "User.Read Group.Read.All", "scope not updated: %s", r.Str("scope"))
		l := c.Status(c.Do(get, c.Graph("/servicePrincipals/"+spID+"/oauth2PermissionGrants"), tok, nil), 200, "", "grants of the client").Value()
		c.Expect(HasID(l, "id", gid), "client SP should list its grant")
		c.Status(c.Do(get, c.Graph("/oauth2PermissionGrants"), tok, nil), 200, "", "list all grants")
		c.Status(c.Do(del, c.Graph("/oauth2PermissionGrants/"+gid), tok, nil), 204, "", "delete grant")
		c.Status(c.Do(get, c.Graph("/oauth2PermissionGrants/"+gid), tok, nil), 404, "Request_ResourceNotFound", "get after delete")
	}},
	"directoryRoles": {"templates, activation, membership", func(c *C) {
		tok := c.Boot("graph")
		const directoryReaders = "88d8e3e3-8f55-4a1e-953a-9b9898b8876b"
		t := c.Status(c.Do(get, c.Graph("/directoryRoleTemplates"), tok, nil), 200, "", "list templates").Value()
		c.Expect(HasID(t, "displayName", "Global Administrator") && HasID(t, "id", directoryReaders), "templates should include Global Administrator and Directory Readers")
		roles := c.Status(c.Do(get, c.Graph("/directoryRoles"), tok, nil), 200, "", "list active roles").Value()
		c.Expect(HasID(roles, "displayName", "Global Administrator"), "seeded Global Administrator role should be active")
		var roleID string
		for _, r := range roles {
			if strings.EqualFold(str(r["roleTemplateId"]), directoryReaders) {
				roleID = str(r["id"])
			}
		}
		if roleID == "" {
			act := c.Status(c.Do(post, c.Graph("/directoryRoles"), tok, map[string]any{"roleTemplateId": directoryReaders}), 201, "", "activate Directory Readers")
			roleID = act.Str("id")
		}
		c.Status(c.Do(post, c.Graph("/directoryRoles"), tok, map[string]any{"roleTemplateId": directoryReaders}), 409, "", "activate twice")
		c.Status(c.Do(post, c.Graph("/directoryRoles"), tok, map[string]any{"roleTemplateId": "00000000-0000-4000-8000-000000000000"}), 400, "Request_BadRequest", "unknown template")
		u := c.Status(c.Do(post, c.Graph("/users"), tok, map[string]any{"accountEnabled": true, "displayName": "role member", "mailNickname": c.Name("rm"), "userPrincipalName": c.Name("rm") + "@cloudtyke.local", "passwordProfile": map[string]any{"password": "Sup3r-secret!"}}), 201, "", "create user")
		c.Status(c.Do(post, c.Graph("/directoryRoles/"+roleID+"/members/$ref"), tok, map[string]any{"@odata.id": c.Graph("/directoryObjects/" + u.Str("id"))}), 204, "", "add role member")
		m := c.Status(c.Do(get, c.Graph("/directoryRoles/"+roleID+"/members"), tok, nil), 200, "", "list role members").Value()
		c.Expect(HasID(m, "id", u.Str("id")), "role members should include the user")
		c.Status(c.Do(get, c.Graph("/directoryRoles/roleTemplateId="+directoryReaders), tok, nil), 200, "", "get by roleTemplateId")
		c.Status(c.Do(del, c.Graph("/directoryRoles/"+roleID+"/members/"+u.Str("id")+"/$ref"), tok, nil), 204, "", "remove role member")
		c.Status(c.Do(post, c.Graph("/directoryRoles"), c.Reader("graph"), map[string]any{"roleTemplateId": directoryReaders}), 403, "Authorization_RequestDenied", "Reader cannot activate roles")
	}},
	"directoryObjects": {"get any object by id, getByIds with type filter", func(c *C) {
		tok := c.Boot("graph")
		u := c.Status(c.Do(get, c.Graph("/directoryObjects/"+AdaUserID), tok, nil), 200, "", "get user via directoryObjects")
		c.Expect(u.Body["@odata.type"] == "#microsoft.graph.user", "expected a user, got %v", u.Body["@odata.type"])
		sp := c.Status(c.Do(get, c.Graph("/directoryObjects/"+BootSPID), tok, nil), 200, "", "get service principal via directoryObjects")
		c.Expect(sp.Body["@odata.type"] == "#microsoft.graph.servicePrincipal", "expected a service principal, got %v", sp.Body["@odata.type"])
		all := c.Status(c.Do(post, c.Graph("/directoryObjects/getByIds"), tok, map[string]any{"ids": []string{AdaUserID, BootSPID, "00000000-0000-4000-8000-00000000dead"}}), 200, "", "getByIds").Value()
		c.Expect(len(all) == 2, "getByIds should resolve 2 of 3 ids, got %d", len(all))
		users := c.Status(c.Do(post, c.Graph("/directoryObjects/getByIds"), tok, map[string]any{"ids": []string{AdaUserID, BootSPID}, "types": []string{"user"}}), 200, "", "getByIds types=user").Value()
		c.Expect(len(users) == 1 && HasID(users, "id", AdaUserID), "type filter should keep only the user, got %d", len(users))
		c.Status(c.Do(get, c.Graph("/directoryObjects/00000000-0000-4000-8000-00000000dead"), tok, nil), 404, "Request_ResourceNotFound", "unknown id")
		c.Status(c.Do(get, c.Graph("/directoryObjects"), tok, nil), 400, "Request_BadRequest", "listing all objects")
	}},
	"deletedItems": {"soft delete, list by type, restore, permanent delete", func(c *C) {
		tok := c.Boot("graph")
		a := c.Status(c.Do(post, c.Graph("/applications"), tok, map[string]any{"displayName": c.Name("del")}), 201, "", "create application")
		id := a.Str("id")
		c.Status(c.Do(del, c.Graph("/applications/"+id), tok, nil), 204, "", "delete (soft)")
		l := c.Status(c.Do(get, c.Graph("/directory/deletedItems/microsoft.graph.application"), tok, nil), 200, "", "list deleted applications").Value()
		c.Expect(HasID(l, "id", id), "deleted application should be listed")
		c.Status(c.Do(get, c.Graph("/directory/deletedItems/"+id), tok, nil), 200, "", "get deleted item")
		c.Status(c.Do(post, c.Graph("/directory/deletedItems/"+id+"/restore"), tok, nil), 200, "", "restore")
		c.Status(c.Do(get, c.Graph("/applications/"+id), tok, nil), 200, "", "restored application is back")
		c.Status(c.Do(del, c.Graph("/applications/"+id), tok, nil), 204, "", "delete again")
		c.Status(c.Do(del, c.Graph("/directory/deletedItems/"+id), tok, nil), 204, "", "permanent delete")
		c.Status(c.Do(get, c.Graph("/directory/deletedItems/"+id), tok, nil), 404, "Request_ResourceNotFound", "gone after permanent delete")
		c.Status(c.Do(get, c.Graph("/directory/deletedItems/microsoft.graph.device"), tok, nil), 400, "Request_BadRequest", "unsupported type")
	}},
	"organization": {"organization and verified domains", func(c *C) {
		tok := c.Reader("graph")
		o := c.Status(c.Do(get, c.Graph("/organization"), tok, nil), 200, "", "organization").Value()
		c.Expect(len(o) == 1 && HasID(o, "id", Tenant), "organization should be the seeded tenant")
		d := c.Status(c.Do(get, c.Graph("/domains"), tok, nil), 200, "", "domains").Value()
		c.Expect(HasID(d, "id", "cloudtyke.local"), "domains should include cloudtyke.local")
		one := c.Status(c.Do(get, c.Graph("/domains/cloudtyke.local"), tok, nil), 200, "", "get domain")
		c.Expect(one.Body["isDefault"] == true && one.Body["isVerified"] == true, "cloudtyke.local should be default and verified")
		c.Status(c.Do(get, c.Graph("/domains/example.com"), tok, nil), 404, "Request_ResourceNotFound", "unknown domain")
		c.Status(c.Do(get, c.Graph("/me"), tok, nil), 400, "BadRequest", "/me needs delegated auth")
		c.Status(c.Do(get, c.Graph("/devices"), tok, nil), 501, "NotImplemented", "unemulated Graph resource")
	}},
}

func str(v any) string { s, _ := v.(string); return s }

func containsStr(v any, want string) bool {
	arr, _ := v.([]any)
	for _, x := range arr {
		if strings.EqualFold(str(x), want) {
			return true
		}
	}
	return false
}

func hasAction(perms []map[string]any, action string) bool {
	for _, p := range perms {
		if containsStr(p["actions"], action) {
			return true
		}
	}
	return false
}

func appRoleID(sp map[string]any, value string) string {
	roles, _ := sp["appRoles"].([]any)
	for _, r := range roles {
		m, _ := r.(map[string]any)
		if strings.EqualFold(str(m["value"]), value) {
			return str(m["id"])
		}
	}
	return ""
}

func jwtHeaderKid(tok string) string {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return ""
	}
	h := Claims(parts[0] + "." + parts[0] + "." + parts[2]) // decode header as payload
	return str(h["kid"])
}
