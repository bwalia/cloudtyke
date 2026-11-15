// Package registry implements the Microsoft.ContainerRegistry control plane:
// registries with SKUs, admin credentials, name availability and the
// replication / webhook child collections that Terraform reads.
package registry

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/arm"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/managedidentity"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

const ns = "Microsoft.ContainerRegistry"

// RegistryType is the full type of a container registry.
const RegistryType = ns + "/registries"

var versions = []string{"2025-04-01", "2024-11-01-preview", "2023-11-01-preview", "2023-07-01", "2023-01-01-preview", "2021-09-01", "2019-05-01"}

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9]{5,50}$`)

// Register wires the RP.
func Register(s *arm.Server) {
	s.Registry.Register(&arm.TypeRegistration{
		Namespace: ns, Types: []string{"registries"}, APIVersions: versions, Scopes: []arm.ScopeKind{arm.KindResourceGroup}, Phase: 2,
		Notes: "Basic/Standard/Premium, globally unique 5-50 alphanumeric names, loginServer, admin user + listCredentials/regenerateCredential, Premium-only features validated",
		Handler: arm.Generic(arm.GenericOptions{Tracked: true, Async: true, Prepare: func(q *arm.Request, body, existing map[string]any) error { return prepare(s, q, body, existing) },
			Actions: map[string]arm.Handler{
				"listCredentials":      func(q *arm.Request) { creds(q, "") },
				"regenerateCredential": func(q *arm.Request) { creds(q, "regen") },
			},
			Present: func(q *arm.Request, b map[string]any) map[string]any {
				b = arm.DeepCopy(b)
				p := props(b)
				delete(p, "_passwords")
				return b
			},
		}),
	})
	for _, child := range []struct{ typ, notes string }{
		{"replications", "geo-replicas (Premium only)"},
		{"webhooks", "registry webhooks"},
		{"scopeMaps", "repository-scoped tokens: scope maps"},
		{"tokens", "repository-scoped tokens"},
	} {
		child := child
		s.Registry.Register(&arm.TypeRegistration{
			Namespace: ns, Types: []string{"registries", child.typ}, APIVersions: versions, Scopes: []arm.ScopeKind{arm.KindResourceGroup}, Phase: 2, Notes: child.notes,
			Handler: arm.Generic(arm.GenericOptions{Tracked: child.typ == "replications" || child.typ == "webhooks", Async: child.typ == "replications",
				Prepare: func(q *arm.Request, body, existing map[string]any) error {
					if child.typ != "replications" {
						return nil
					}
					parent, _ := q.S.LoadResource(q.ID.ParentPath(ns, q.Reg.Types))
					sku, _ := parent["sku"].(map[string]any)
					if strOf(sku["name"]) != "Premium" {
						return arm.BadRequest("ReplicationNotSupported", "Geo-replication is only available for registries in the Premium SKU.")
					}
					if strings.EqualFold(strOf(body["location"]), strOf(parent["location"])) {
						return arm.BadRequest("ReplicationAlreadyExists", "A registry already exists in its home region; replicate to a different location.")
					}
					return nil
				},
			}),
		})
	}
	// subscription-level name check
	s.Registry.Register(&arm.TypeRegistration{
		Namespace: ns, Types: []string{"checkNameAvailability"}, APIVersions: versions, Scopes: []arm.ScopeKind{arm.KindSubscription}, Phase: 2,
		Notes: "POST /subscriptions/{id}/providers/Microsoft.ContainerRegistry/checkNameAvailability",
		Handler: func(q *arm.Request) {
			if q.R.Method != http.MethodPost {
				q.Error(405, "MethodNotAllowed", "Use POST.")
				return
			}
			in, ok := q.BodyOrError()
			if !ok {
				return
			}
			name := strOf(in["name"])
			if !nameRe.MatchString(name) {
				q.JSON(200, map[string]any{"nameAvailable": false, "reason": "Invalid", "message": "The specified resource name is not valid: use 5-50 alphanumeric characters."})
				return
			}
			if owner := ownerOf(s, name); owner != "" {
				q.JSON(200, map[string]any{"nameAvailable": false, "reason": "AlreadyExists", "message": "The registry " + name + " is already in use."})
				return
			}
			q.JSON(200, map[string]any{"nameAvailable": true})
		},
	})
	s.OnDeleted(RegistryType, func(b map[string]any) { managedidentity.CleanupSystemIdentity(s, b) })
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

func secret() string {
	b := make([]byte, 39)
	_, _ = rand.Read(b)
	return strings.NewReplacer("/", "A", "+", "B", "=", "C").Replace(base64.StdEncoding.EncodeToString(b))
}

// ownerOf returns the id of the registry using name (names are global in Azure).
func ownerOf(s *arm.Server, name string) string {
	owner := ""
	_ = s.Store.Scan(store.BucketARM, "/subscriptions/", func(k string, _ []byte) bool {
		if strings.HasSuffix(k, "/providers/microsoft.containerregistry/registries/"+strings.ToLower(name)) {
			owner = k
			return false
		}
		return true
	})
	return owner
}

// LoginServer returns the registry login server host name for the emulator domain.
func LoginServer(domain, name string) string {
	return strings.ToLower(name) + ".azurecr." + domain
}

func prepare(s *arm.Server, q *arm.Request, body, existing map[string]any) error {
	name := q.ID.Name()
	if !nameRe.MatchString(name) {
		return arm.BadRequest("InvalidRegistryName", arm.Errorf("Registry name '%s' is invalid: use 5-50 alphanumeric characters.", name))
	}
	if existing == nil {
		if owner := ownerOf(s, name); owner != "" {
			return &arm.APIError{Status: 409, Code: "AlreadyInUse", Message: arm.Errorf("The registry %s is already in use. Registry names are globally unique.", name)}
		}
	}
	sku, _ := body["sku"].(map[string]any)
	if sku == nil {
		return arm.BadRequest("MissingSku", "The sku is required (Basic, Standard or Premium).")
	}
	switch strOf(sku["name"]) {
	case "Basic", "Standard", "Premium":
		sku["tier"] = sku["name"]
	default:
		return arm.BadRequest("InvalidSku", arm.Errorf("The sku '%v' is not valid. Use Basic, Standard or Premium.", sku["name"]))
	}
	p := props(body)
	premium := strOf(sku["name"]) == "Premium"
	if !premium {
		if strOf(p["publicNetworkAccess"]) == "Disabled" {
			return arm.BadRequest("PublicNetworkAccessNotSupported", "Disabling public network access requires the Premium SKU.")
		}
		if zr := strOf(p["zoneRedundancy"]); zr == "Enabled" {
			return arm.BadRequest("ZoneRedundancyNotSupported", "Zone redundancy requires the Premium SKU.")
		}
		if nr, _ := p["networkRuleSet"].(map[string]any); nr != nil && strOf(nr["defaultAction"]) == "Deny" {
			return arm.BadRequest("NetworkRuleSetNotSupported", "Network rule sets require the Premium SKU.")
		}
	}
	p["loginServer"] = LoginServer(q.S.Cfg.Domain, name)
	defaults := map[string]any{"adminUserEnabled": false, "publicNetworkAccess": "Enabled", "zoneRedundancy": "Disabled", "dataEndpointEnabled": false,
		"networkRuleBypassOptions": "AzureServices", "anonymousPullEnabled": false}
	for k, v := range defaults {
		if _, ok := p[k]; !ok {
			p[k] = v
		}
	}
	if p["policies"] == nil {
		p["policies"] = map[string]any{
			"quarantinePolicy": map[string]any{"status": "disabled"}, "trustPolicy": map[string]any{"type": "Notary", "status": "disabled"},
			"retentionPolicy": map[string]any{"days": 7, "status": "disabled"}, "exportPolicy": map[string]any{"status": "enabled"},
			"azureADAuthenticationAsArmPolicy": map[string]any{"status": "enabled"}, "softDeletePolicy": map[string]any{"retentionDays": 7, "status": "disabled"},
		}
	}
	if p["encryption"] == nil {
		p["encryption"] = map[string]any{"status": "disabled"}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if existing != nil {
		ep := props(existing)
		p["creationDate"], p["_passwords"] = ep["creationDate"], ep["_passwords"]
	} else {
		p["creationDate"] = now
		p["_passwords"] = []any{secret(), secret()}
	}
	p["status"] = map[string]any{"displayStatus": "Ready", "timestamp": now}
	return managedidentity.ApplyIdentity(q, body, existing)
}

func creds(q *arm.Request, mode string) {
	path := q.ID.Path(ns, q.Reg.Types)
	m, _ := q.S.LoadResource(path)
	p := props(m)
	if p["adminUserEnabled"] != true {
		q.Error(400, "AdminUserDisabled", "Admin user is disabled. Enable it with adminUserEnabled=true to use listCredentials.")
		return
	}
	pw, _ := p["_passwords"].([]any)
	if len(pw) < 2 {
		pw = []any{secret(), secret()}
	}
	if mode == "regen" {
		in, _ := q.Body()
		if strOf(in["name"]) == "password2" {
			pw[1] = secret()
		} else {
			pw[0] = secret()
		}
		p["_passwords"] = pw
		_ = q.S.SaveResource(m)
	}
	q.JSON(200, map[string]any{
		"username":  strOf(m["name"]),
		"passwords": []any{map[string]any{"name": "password", "value": pw[0]}, map[string]any{"name": "password2", "value": pw[1]}},
	})
}
