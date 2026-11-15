// Package managedidentity implements Microsoft.ManagedIdentity
// (user-assigned identities and their federated identity credentials) and
// the system-assigned "identity" block shared by other resource types.
//
// Every managed identity is backed by a real Entra service principal
// (servicePrincipalType "ManagedIdentity"), so it can be granted Azure roles
// and Graph app roles, shows up in Microsoft Graph, and, through its
// federated credentials, can sign in with workload identity.
package managedidentity

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/arm"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
)

const ns = "Microsoft.ManagedIdentity"

// APIVersions accepted.
var APIVersions = []string{"2024-11-30", "2023-07-31-preview", "2023-01-31", "2022-01-31-preview", "2018-11-30"}

// UAIType is the full resource type of a user-assigned identity.
const UAIType = ns + "/userAssignedIdentities"

var ficName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{2,119}$`)

// Register wires the RP.
func Register(s *arm.Server) {
	s.Registry.Register(&arm.TypeRegistration{
		Namespace: ns, Types: []string{"userAssignedIdentities"}, APIVersions: APIVersions, Scopes: []arm.ScopeKind{arm.KindResourceGroup}, Phase: 2,
		Notes: "backed by an Entra service principal (servicePrincipalType ManagedIdentity): principalId/clientId are stable, assignable to roles, visible in Graph",
		Handler: arm.Generic(arm.GenericOptions{Tracked: true,
			Prepare: func(q *arm.Request, body, existing map[string]any) error {
				p := props(body)
				if existing != nil {
					ep := props(existing)
					for _, k := range []string{"principalId", "clientId", "tenantId"} {
						p[k] = ep[k]
					}
				} else {
					pid, cid := CreatePrincipal(q.S, q.Identity.TenantID, q.ID.Name(), q.ID.Path(ns, q.Reg.Types), true)
					p["principalId"], p["clientId"], p["tenantId"] = pid, cid, q.Identity.TenantID
				}
				if strOf(p["isolationScope"]) == "" {
					p["isolationScope"] = "None"
				}
				return nil
			},
			Present: func(q *arm.Request, b map[string]any) map[string]any {
				b = arm.DeepCopy(b)
				delete(props(b), "provisioningState") // UAIs have no provisioningState in Azure
				return b
			},
		}),
	})
	s.Registry.Register(&arm.TypeRegistration{
		Namespace: ns, Types: []string{"userAssignedIdentities", "federatedIdentityCredentials"}, APIVersions: APIVersions, Scopes: []arm.ScopeKind{arm.KindResourceGroup}, Phase: 2,
		Notes: "issuer/subject/audiences, unique issuer+subject, max 20 per identity; the token service accepts matching client assertions for the identity's clientId",
		Handler: arm.Generic(arm.GenericOptions{
			Prepare:    prepareFIC,
			AfterWrite: func(q *arm.Request, body map[string]any) { syncFICs(q.S, q.ID.ParentPath(ns, q.Reg.Types)) },
			Present: func(q *arm.Request, b map[string]any) map[string]any {
				b = arm.DeepCopy(b)
				delete(props(b), "provisioningState")
				return b
			},
		}),
	})
	s.OnDeleted(UAIType, func(b map[string]any) { DeletePrincipal(s, props(b)) })
	s.OnDeleted(UAIType+"/federatedIdentityCredentials", func(b map[string]any) {
		id := strOf(b["id"])
		if i := strings.Index(strings.ToLower(id), "/federatedidentitycredentials/"); i > 0 {
			syncFICs(s, id[:i])
		}
	})
}

func strOf(v any) string { s, _ := v.(string); return s }

func props(b map[string]any) map[string]any {
	p, _ := b["properties"].(map[string]any)
	if p == nil {
		p = map[string]any{}
		b["properties"] = p
	}
	return p
}

// CreatePrincipal creates the service principal behind a managed identity and
// returns its object id (principalId) and application id (clientId).
func CreatePrincipal(s *arm.Server, tid, name, resourceID string, explicit bool) (principalID, clientID string) {
	principalID, clientID = entra.NewGUID(), entra.NewGUID()
	exp := "isExplicit=False"
	if explicit {
		exp = "isExplicit=True"
	}
	s.Dir.Mu.Lock()
	defer s.Dir.Mu.Unlock()
	_ = s.Dir.Put(tid, entra.ServicePrincipals, entra.Object{
		"id": principalID, "appId": clientID, "displayName": name, "appDisplayName": nil,
		"servicePrincipalType": "ManagedIdentity", "accountEnabled": true,
		"alternativeNames":       []any{exp, resourceID},
		"servicePrincipalNames":  []any{clientID, "https://identity.azure.net/" + strings.ReplaceAll(entra.NewGUID(), "-", "")},
		"appOwnerOrganizationId": nil, "appRoles": []any{}, "oauth2PermissionScopes": []any{}, "tags": []any{},
		"passwordCredentials": []any{}, "keyCredentials": []any{}, "createdDateTime": entra.Now(),
		"appRoleAssignmentRequired": false, "signInAudience": nil, "_resourceId": resourceID,
	})
	return principalID, clientID
}

// DeletePrincipal removes the service principal of a deleted identity
// (managed identity service principals are not soft-deleted).
func DeletePrincipal(s *arm.Server, p map[string]any) {
	tid, pid := strOf(p["tenantId"]), strOf(p["principalId"])
	if tid == "" || pid == "" {
		return
	}
	s.Dir.Mu.Lock()
	defer s.Dir.Mu.Unlock()
	if s.Dir.Delete(tid, entra.ServicePrincipals, pid) {
		s.Dir.PurgeDeleted(tid, pid)
	}
}

// Lookup returns the stored user-assigned identity with this resource id.
func Lookup(s *arm.Server, id string) (map[string]any, bool) {
	m, ok := s.LoadResource(id)
	if !ok || !strings.EqualFold(strOf(m["type"]), UAIType) {
		return nil, false
	}
	return m, true
}

// ApplyIdentity resolves the "identity" block of a resource being written:
// system-assigned identities get (and keep) a service principal, and
// user-assigned identities must exist and get their principal/client ids.
func ApplyIdentity(q *arm.Request, body, existing map[string]any) error {
	idn, _ := body["identity"].(map[string]any)
	var old map[string]any
	if existing != nil {
		old, _ = existing["identity"].(map[string]any)
	}
	typ := strings.ReplaceAll(strings.ToLower(strOf(idn["type"])), " ", "")
	hadSystem := old != nil && strOf(old["principalId"]) != "" && strings.Contains(strings.ToLower(strOf(old["type"])), "systemassigned")
	wantSystem := strings.Contains(typ, "systemassigned")
	if idn == nil || typ == "" || typ == "none" {
		if hadSystem {
			DeletePrincipal(q.S, map[string]any{"tenantId": old["tenantId"], "principalId": old["principalId"]})
		}
		if idn != nil {
			body["identity"] = map[string]any{"type": "None"}
		}
		return nil
	}
	switch typ {
	case "systemassigned", "userassigned", "systemassigned,userassigned":
	default:
		return arm.BadRequest("InvalidIdentityType", arm.Errorf("Invalid identity type '%v'. Allowed values: SystemAssigned, UserAssigned, 'SystemAssigned, UserAssigned', None.", idn["type"]))
	}
	if wantSystem {
		if hadSystem {
			idn["principalId"], idn["tenantId"] = old["principalId"], old["tenantId"]
		} else {
			pid, _ := CreatePrincipal(q.S, q.Identity.TenantID, strOf(body["name"]), strOf(body["id"]), false)
			idn["principalId"], idn["tenantId"] = pid, q.Identity.TenantID
		}
	} else {
		if hadSystem {
			DeletePrincipal(q.S, map[string]any{"tenantId": old["tenantId"], "principalId": old["principalId"]})
		}
		delete(idn, "principalId")
		delete(idn, "tenantId")
	}
	if strings.Contains(typ, "userassigned") {
		uais, _ := idn["userAssignedIdentities"].(map[string]any)
		if len(uais) == 0 {
			return arm.BadRequest("BadRequest", "The identity type includes UserAssigned but no userAssignedIdentities were provided.")
		}
		out := map[string]any{}
		for rid := range uais {
			m, ok := Lookup(q.S, rid)
			if !ok {
				return arm.BadRequest("InvalidUserAssignedIdentities", arm.Errorf("Resource '%s' was not found. The user assigned identity must exist before it can be assigned.", rid))
			}
			mp := props(m)
			out[strOf(m["id"])] = map[string]any{"principalId": mp["principalId"], "clientId": mp["clientId"]}
		}
		idn["userAssignedIdentities"] = out
	} else {
		delete(idn, "userAssignedIdentities")
	}
	idn["type"] = canonicalType(typ)
	return nil
}

// CleanupSystemIdentity removes the system-assigned principal of a deleted resource.
func CleanupSystemIdentity(s *arm.Server, body map[string]any) {
	if idn, ok := body["identity"].(map[string]any); ok && strOf(idn["principalId"]) != "" {
		DeletePrincipal(s, map[string]any{"tenantId": idn["tenantId"], "principalId": idn["principalId"]})
	}
}

func canonicalType(t string) string {
	switch t {
	case "systemassigned":
		return "SystemAssigned"
	case "userassigned":
		return "UserAssigned"
	default:
		return "SystemAssigned, UserAssigned"
	}
}

func prepareFIC(q *arm.Request, body, existing map[string]any) error {
	p := props(body)
	name := q.ID.Name()
	if !ficName.MatchString(name) {
		return arm.BadRequest("BadRequest", arm.Errorf("Federated identity credential name '%s' is invalid: use 3-120 letters, digits, '-' or '_', starting with a letter or digit.", name))
	}
	iss := strOf(p["issuer"])
	if u, err := url.Parse(iss); err != nil || u.Scheme != "https" || u.Host == "" {
		return arm.BadRequest("BadRequest", arm.Errorf("Issuer '%s' is invalid: it must be an https URL.", iss))
	}
	if strOf(p["subject"]) == "" {
		return arm.BadRequest("BadRequest", "properties.subject is required.")
	}
	if len(entra.StringList(p["audiences"])) == 0 {
		p["audiences"] = []any{"api://AzureADTokenExchange"}
	}
	parent := q.ID.ParentPath(ns, q.Reg.Types)
	siblings := q.S.Children(parent + "/federatedIdentityCredentials")
	count := 0
	for _, f := range siblings {
		if strings.EqualFold(strOf(f["name"]), name) {
			continue
		}
		count++
		fp := props(f)
		if strings.TrimSuffix(strOf(fp["issuer"]), "/") == strings.TrimSuffix(iss, "/") && strOf(fp["subject"]) == strOf(p["subject"]) {
			return &arm.APIError{Status: 409, Code: "Conflict", Message: arm.Errorf("A federated identity credential with issuer '%s' and subject '%s' already exists on this identity (%s).", iss, p["subject"], f["name"])}
		}
	}
	if count >= 20 {
		return arm.BadRequest("BadRequest", "A user-assigned identity can have at most 20 federated identity credentials.")
	}
	return nil
}

// syncFICs copies the identity's federated credentials onto its service
// principal, where the token service looks them up.
func syncFICs(s *arm.Server, uaiID string) {
	m, ok := Lookup(s, uaiID)
	if !ok {
		return
	}
	p := props(m)
	tid, pid := strOf(p["tenantId"]), strOf(p["principalId"])
	var fics []any
	for _, f := range s.Children(uaiID + "/federatedIdentityCredentials") {
		fp := props(f)
		fics = append(fics, map[string]any{"name": f["name"], "issuer": fp["issuer"], "subject": fp["subject"], "audiences": fp["audiences"]})
	}
	s.Dir.Mu.Lock()
	defer s.Dir.Mu.Unlock()
	if sp, ok := s.Dir.Get(tid, entra.ServicePrincipals, pid); ok {
		sp["_fics"] = fics
		_ = s.Dir.Put(tid, entra.ServicePrincipals, sp)
	}
}

// CreateUAI stores a user-assigned identity created by a platform service
// (AKS kubelet and add-on identities in the node resource group).
func CreateUAI(s *arm.Server, tid, rgID, name, location string, tags map[string]any) map[string]any {
	id := rgID + "/providers/" + UAIType + "/" + name
	if m, ok := Lookup(s, id); ok {
		return m
	}
	pid, cid := CreatePrincipal(s, tid, name, id, true)
	m := map[string]any{
		"id": id, "name": name, "type": UAIType, "location": location, "etag": arm.NewETag(),
		"properties": map[string]any{"tenantId": tid, "principalId": pid, "clientId": cid, "isolationScope": "None", "provisioningState": "Succeeded"},
	}
	if len(tags) > 0 {
		m["tags"] = tags
	}
	_ = s.SaveResource(m)
	return m
}

// Ref returns {resourceId, clientId, objectId}, the shape AKS uses for identities.
func Ref(m map[string]any) map[string]any {
	p := props(m)
	return map[string]any{"resourceId": m["id"], "clientId": p["clientId"], "objectId": p["principalId"]}
}
