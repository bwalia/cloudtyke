// Package seed loads the initial tenant / subscription / identity layout
// from YAML and applies it to an empty store.
package seed

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/graph"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/hierarchy"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rbac"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

//go:embed default.yaml
var defaultSeed []byte

// Default returns the embedded default seed.
func Default() []byte { return defaultSeed }

// File is the seed.yaml schema.
type File struct {
	Tenants []Tenant `yaml:"tenants"`
}

// Tenant seeds one Entra tenant and its Azure hierarchy.
type Tenant struct {
	ID               string             `yaml:"id"`
	DisplayName      string             `yaml:"displayName"`
	DefaultDomain    string             `yaml:"defaultDomain"`
	Domains          []string           `yaml:"domains"`
	ManagementGroups []ManagementGroup  `yaml:"managementGroups"`
	Subscriptions    []Subscription     `yaml:"subscriptions"`
	SPs              []ServicePrincipal `yaml:"servicePrincipals"`
	Users            []User             `yaml:"users"`
}

// ManagementGroup seeds a management group.
type ManagementGroup struct {
	Name        string `yaml:"name"`
	DisplayName string `yaml:"displayName"`
	Parent      string `yaml:"parent"`
}

// Subscription seeds a subscription.
type Subscription struct {
	ID              string            `yaml:"id"`
	DisplayName     string            `yaml:"displayName"`
	ManagementGroup string            `yaml:"managementGroup"`
	Tags            map[string]string `yaml:"tags"`
}

// RoleAssignment seeds an Azure RBAC assignment.
type RoleAssignment struct {
	Role  string `yaml:"role"`
	Scope string `yaml:"scope"`
}

// ServicePrincipal seeds an application + service principal with a secret.
type ServicePrincipal struct {
	DisplayName         string           `yaml:"displayName"`
	AppID               string           `yaml:"appId"`
	ApplicationObjectID string           `yaml:"applicationObjectId"`
	ObjectID            string           `yaml:"objectId"`
	ClientSecret        string           `yaml:"clientSecret"`
	GraphPermissions    []string         `yaml:"graphPermissions"`
	DirectoryRoles      []string         `yaml:"directoryRoles"`
	RoleAssignments     []RoleAssignment `yaml:"roleAssignments"`
}

// User seeds a user.
type User struct {
	DisplayName       string           `yaml:"displayName"`
	UserPrincipalName string           `yaml:"userPrincipalName"`
	ObjectID          string           `yaml:"objectId"`
	Password          string           `yaml:"password"`
	RoleAssignments   []RoleAssignment `yaml:"roleAssignments"`
}

// Load reads the seed file at path or the embedded default when path is empty.
func Load(path string) (File, []byte, error) {
	raw := defaultSeed
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return File{}, nil, err
		}
		raw = b
	}
	var f File
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return File{}, nil, fmt.Errorf("parsing seed: %w", err)
	}
	return f, raw, nil
}

// IsSeeded reports whether the store was already seeded.
func IsSeeded(st store.Store) bool {
	_, ok, _ := st.Get(store.BucketMeta, "seeded")
	return ok
}

// Apply writes the seed into the store (idempotent: existing objects with
// the same IDs are overwritten).
func Apply(st store.Store, f File) error {
	dir := entra.New(st)
	h := hierarchy.New(st)
	eng := rbac.New(st, h, dir)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, t := range f.Tenants {
		if t.DefaultDomain == "" {
			return fmt.Errorf("tenant %s: defaultDomain is required", t.ID)
		}
		if len(t.Domains) == 0 {
			t.Domains = []string{t.DefaultDomain}
		}
		if err := dir.PutTenant(entra.Tenant{ID: t.ID, DisplayName: t.DisplayName, DefaultDomain: t.DefaultDomain, Domains: t.Domains, CreatedAt: now}); err != nil {
			return err
		}
		// tenant root management group
		_ = h.PutManagementGroup(hierarchy.ManagementGroup{Name: t.ID, DisplayName: "Tenant Root Group", TenantID: t.ID})
		for _, m := range t.ManagementGroups {
			parent := m.Parent
			if parent == "" {
				parent = t.ID
			}
			_ = h.PutManagementGroup(hierarchy.ManagementGroup{Name: m.Name, DisplayName: m.DisplayName, TenantID: t.ID, Parent: parent})
		}
		for _, s := range t.Subscriptions {
			mg := s.ManagementGroup
			if mg == "" {
				mg = t.ID
			}
			_ = h.PutSubscription(hierarchy.Subscription{ID: s.ID, DisplayName: s.DisplayName, TenantID: t.ID, ManagementGroup: mg, Tags: s.Tags})
		}

		graphSP := seedMicrosoftGraph(dir, t.ID)
		roles := map[string]map[string]any{}
		for _, tmpl := range graph.DirectoryRoleTemplates {
			r := graph.ActivateRole(tmpl)
			if existing, ok := findRole(dir, t.ID, tmpl.ID); ok {
				r = existing
			}
			roles[strings.ToLower(tmpl.Name)] = r
		}

		for _, sp := range t.SPs {
			app := map[string]any{
				"id": sp.ApplicationObjectID, "appId": sp.AppID, "displayName": sp.DisplayName, "createdDateTime": now,
				"signInAudience": "AzureADMyOrg", "identifierUris": []any{}, "appRoles": []any{}, "tags": []any{"cloudtyke-seed"},
				"keyCredentials": []any{}, "requiredResourceAccess": []any{}, "publisherDomain": t.DefaultDomain,
				"api": map[string]any{"oauth2PermissionScopes": []any{}, "knownClientApplications": []any{}, "preAuthorizedApplications": []any{}},
				"web": map[string]any{"redirectUris": []any{}}, "_owners": []any{},
			}
			if sp.ClientSecret != "" {
				keyID := entra.NewGUID()
				app["passwordCredentials"] = []any{map[string]any{"customKeyIdentifier": nil, "displayName": "cloudtyke-seed", "endDateTime": "2099-12-31T00:00:00Z", "hint": sp.ClientSecret[:3], "keyId": keyID, "startDateTime": "2020-01-01T00:00:00Z"}}
				app["_secretHashes"] = map[string]any{keyID: entra.HashSecret(sp.ClientSecret)}
			} else {
				app["passwordCredentials"] = []any{}
			}
			if err := dir.Put(t.ID, entra.Applications, app); err != nil {
				return err
			}
			spObj := map[string]any{
				"id": sp.ObjectID, "appId": sp.AppID, "displayName": sp.DisplayName, "appDisplayName": sp.DisplayName,
				"accountEnabled": true, "servicePrincipalType": "Application", "servicePrincipalNames": []any{sp.AppID},
				"appOwnerOrganizationId": t.ID, "appRoles": []any{}, "oauth2PermissionScopes": []any{}, "tags": []any{"cloudtyke-seed"},
				"passwordCredentials": []any{}, "keyCredentials": []any{}, "alternativeNames": []any{}, "createdDateTime": now,
				"appRoleAssignmentRequired": false, "signInAudience": "AzureADMyOrg",
			}
			if err := dir.Put(t.ID, entra.ServicePrincipals, spObj); err != nil {
				return err
			}
			if err := grantGraphPermissions(dir, t.ID, graphSP, sp.ObjectID, sp.DisplayName, sp.GraphPermissions); err != nil {
				return err
			}
			for _, rn := range sp.DirectoryRoles {
				r, ok := roles[strings.ToLower(rn)]
				if !ok {
					return fmt.Errorf("unknown directory role %q", rn)
				}
				m := entra.StringList(r["_members"])
				if !entra.Contains(m, sp.ObjectID) {
					r["_members"] = append(toAny(m), sp.ObjectID)
				}
			}
			if err := assign(eng, st, t.ID, sp.ObjectID, "ServicePrincipal", sp.RoleAssignments); err != nil {
				return err
			}
		}
		for _, u := range t.Users {
			upn := u.UserPrincipalName
			nick := upn
			if i := strings.Index(upn, "@"); i > 0 {
				nick = upn[:i]
			}
			obj := map[string]any{
				"id": u.ObjectID, "displayName": u.DisplayName, "userPrincipalName": upn, "mailNickname": nick,
				"accountEnabled": true, "userType": "Member", "createdDateTime": now, "businessPhones": []any{},
				"otherMails": []any{}, "proxyAddresses": []any{}, "mail": nil,
				"_passwordHash": entra.HashSecret(u.Password),
			}
			if err := dir.Put(t.ID, entra.Users, obj); err != nil {
				return err
			}
			if err := assign(eng, st, t.ID, u.ObjectID, "User", u.RoleAssignments); err != nil {
				return err
			}
		}
		for _, r := range roles {
			if err := dir.Put(t.ID, "directoryRoles", r); err != nil {
				return err
			}
		}
	}
	return st.Put(store.BucketMeta, "seeded", []byte(`"`+now+`"`))
}

func findRole(dir *entra.Directory, tid, tmpl string) (map[string]any, bool) {
	for _, r := range dir.List(tid, "directoryRoles") {
		if s, _ := r["roleTemplateId"].(string); strings.EqualFold(s, tmpl) {
			return r, true
		}
	}
	return nil, false
}

func toAny(s []string) []any {
	out := make([]any, len(s))
	for i, x := range s {
		out[i] = x
	}
	return out
}

func assign(eng *rbac.Engine, st store.Store, tid, principalID, ptype string, ras []RoleAssignment) error {
	for i, ra := range ras {
		var rd rbac.RoleDefinition
		found := false
		for _, b := range rbac.BuiltinRoles {
			if strings.EqualFold(b.RoleName, ra.Role) {
				rd, found = b, true
			}
		}
		if !found {
			return fmt.Errorf("unknown built-in role %q", ra.Role)
		}
		scope := "/" + strings.Trim(ra.Scope, "/")
		prefix := scope
		if scope == "/" {
			prefix = ""
		}
		rdPrefix := ""
		if seg := hierarchy.Segments(scope); len(seg) >= 2 && strings.EqualFold(seg[0], "subscriptions") {
			rdPrefix = "/subscriptions/" + seg[1]
		}
		// deterministic assignment name derived from principal + index
		name := deterministicGUID(principalID, i)
		body := map[string]any{
			"id":   prefix + "/providers/Microsoft.Authorization/roleAssignments/" + name,
			"type": "Microsoft.Authorization/roleAssignments",
			"name": name,
			"properties": map[string]any{
				"roleDefinitionId": rdPrefix + "/providers/Microsoft.Authorization/roleDefinitions/" + rd.ID,
				"principalId":      principalID, "principalType": ptype, "scope": scope,
				"createdOn": "2020-01-01T00:00:00Z", "updatedOn": "2020-01-01T00:00:00Z", "createdBy": "cloudtyke-seed", "updatedBy": "cloudtyke-seed",
				"description": "Seeded by CloudTyke",
			},
		}
		if err := store.PutJSON(st, store.BucketARM, strings.ToLower(body["id"].(string)), body); err != nil {
			return err
		}
	}
	_ = eng
	return nil
}

func deterministicGUID(principal string, i int) string {
	p := strings.ReplaceAll(principal, "-", "")
	if len(p) < 32 {
		p += strings.Repeat("0", 32-len(p))
	}
	b := []byte(p[:32])
	b[0] = 'e'
	b[31] = "0123456789abcdef"[i%16]
	s := string(b)
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32]
}

// MicrosoftGraphAppID is the well-known application id of Microsoft Graph.
const MicrosoftGraphAppID = "00000003-0000-0000-c000-000000000000"

// graphAppRoles are a subset of Microsoft Graph application permissions with
// their real, well-known IDs.
var graphAppRoles = map[string]string{
	"Directory.ReadWrite.All":                "19dbc75e-c2e2-444c-a770-ec69d8559fc7",
	"Directory.Read.All":                     "7ab1d382-f21e-4acd-a863-ba3e13f7da61",
	"Application.ReadWrite.All":              "1bfefb4e-e0b5-418b-a88f-73c46d2cc8e9",
	"Application.ReadWrite.OwnedBy":          "18a4783c-866b-4cc7-a460-3d5e5662c884",
	"Application.Read.All":                   "9a5d68dd-52b0-4cc2-bd40-abcf44ac3a30",
	"AppRoleAssignment.ReadWrite.All":        "06b708a9-e830-4db3-a914-8e69da51d44f",
	"User.ReadWrite.All":                     "741f803b-c850-494e-b5df-cde7c675a1ca",
	"User.Read.All":                          "df021288-bdef-4463-88db-98f22de89214",
	"Group.ReadWrite.All":                    "62a82d76-70ea-41e2-9197-370581804d09",
	"Group.Read.All":                         "5b567255-7703-4780-807c-7be8301ae99b",
	"GroupMember.ReadWrite.All":              "dbaae8cf-10b5-4b86-a4a1-f871c94c6695",
	"GroupMember.Read.All":                   "98830695-27a2-44f7-8c18-0c3ebc9698f6",
	"RoleManagement.ReadWrite.Directory":     "9e3f62cf-ca93-4989-b6ce-bf83c28f9fe8",
	"RoleManagement.Read.Directory":          "483bed4a-2ad3-4361-a73b-c83ccdbdc53c",
	"DelegatedPermissionGrant.ReadWrite.All": "8e8e4742-1d95-4f68-9d56-6ee75648c72a",
}

func seedMicrosoftGraph(dir *entra.Directory, tid string) map[string]any {
	if sp, ok := dir.FindBy(tid, entra.ServicePrincipals, "appId", MicrosoftGraphAppID); ok {
		return sp
	}
	var roles []any
	for v, id := range graphAppRoles {
		roles = append(roles, map[string]any{"allowedMemberTypes": []any{"Application"}, "description": "Allows the app to " + v, "displayName": v, "id": id, "isEnabled": true, "origin": "Application", "value": v})
	}
	sp := map[string]any{
		"id": "0e100000-0000-4000-8000-00000000c0de", "appId": MicrosoftGraphAppID, "displayName": "Microsoft Graph", "appDisplayName": "Microsoft Graph",
		"accountEnabled": true, "servicePrincipalType": "Application", "appOwnerOrganizationId": "f8cdef31-a31e-4b4a-93e4-5f571e91255a",
		"servicePrincipalNames": []any{MicrosoftGraphAppID, "https://graph.microsoft.com", "https://graph." + strings.TrimPrefix(firstDomainSuffix(), "."), "00000003-0000-0000-c000-000000000000/ags.windows.net"},
		"appRoles":              roles, "oauth2PermissionScopes": []any{}, "tags": []any{}, "passwordCredentials": []any{}, "keyCredentials": []any{},
		"alternativeNames": []any{}, "_appRoleAssignedTo": []any{},
	}
	_ = dir.Put(tid, entra.ServicePrincipals, sp)
	return sp
}

// graphHost is set by the emulator before seeding so the Microsoft Graph
// service principal matches the configured domain.
var graphHost = "cloudtyke.local"

// SetDomain configures the domain used for well-known service principal names.
func SetDomain(d string) { graphHost = d }

func firstDomainSuffix() string { return graphHost }

func grantGraphPermissions(dir *entra.Directory, tid string, graphSP map[string]any, principalID, principalName string, perms []string) error {
	list := entra.ObjectList(graphSP["_appRoleAssignedTo"])
	for _, p := range perms {
		id, ok := graphAppRoles[p]
		if !ok {
			return fmt.Errorf("unknown Graph permission %q", p)
		}
		exists := false
		for _, a := range list {
			if strings.EqualFold(a["principalId"].(string), principalID) && strings.EqualFold(a["appRoleId"].(string), id) {
				exists = true
			}
		}
		if exists {
			continue
		}
		list = append(list, map[string]any{
			"@odata.type": "#microsoft.graph.appRoleAssignment",
			"id":          strings.ReplaceAll(entra.NewGUID(), "-", "")[:24], "deletedDateTime": nil, "appRoleId": id,
			"createdDateTime": entra.Now(), "principalDisplayName": principalName, "principalId": principalID, "principalType": "ServicePrincipal",
			"resourceDisplayName": "Microsoft Graph", "resourceId": graphSP["id"],
		})
	}
	out := make([]any, len(list))
	for i, x := range list {
		out[i] = x
	}
	graphSP["_appRoleAssignedTo"] = out
	return dir.Put(tid, entra.ServicePrincipals, graphSP)
}
