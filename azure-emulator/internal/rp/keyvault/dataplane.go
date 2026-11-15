package keyvault

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/auth"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

// DataPlane serves https://<vault>.vault.<domain>/ (Key Vault REST API 7.x).
func (rp *RP) DataPlane() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := strings.ToLower(r.Host)
		if h, _, ok := strings.Cut(host, ":"); ok {
			host = h
		}
		name := strings.TrimSuffix(host, "."+rp.s.Cfg.Host("vault"))
		vault, ok := rp.vaultByName(name)
		if !ok {
			kvError(w, 404, "VaultNotFound", fmt.Sprintf("The vault '%s' was not found.", name))
			return
		}
		challenge := `Bearer authorization="` + rp.s.Cfg.URL("login") + "/" + strOf(props(vault)["tenantId"]) + `", resource="https://` + rp.s.Cfg.Host("vault") + `"`
		id, code, msg := rp.authenticate(r)
		if id == nil {
			w.Header().Set("WWW-Authenticate", challenge)
			kvError(w, 401, "Unauthorized", msg+" ("+code+")")
			return
		}
		if strings.EqualFold(strOf(props(vault)["publicNetworkAccess"]), "Disabled") {
			kvError(w, 403, "ForbiddenByConnection", "Public network access is disabled and request is not from a trusted service nor via an approved private link.\r\nVault: "+name+";location="+strOf(vault["location"]))
			return
		}
		rp.serveSecrets(w, r, vault, id)
	})
}

// authenticate accepts tokens for https://vault.azure.net or https://vault.<domain>.
func (rp *RP) authenticate(r *http.Request) (*auth.Identity, string, string) {
	if r.Header.Get("Authorization") == "" {
		return nil, "Unauthorized", "AKV10000: Request is missing a Bearer or PoP token."
	}
	id, code, msg := rp.s.Auth.Authenticate(r, rp.s.Cfg.Host("vault"))
	if id == nil {
		if id2, _, _ := rp.s.Auth.Authenticate(r, "vault.azure.net"); id2 != nil {
			return id2, "", ""
		}
	}
	return id, code, msg
}

func kvError(w http.ResponseWriter, status int, code, msg string, inner ...string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	e := map[string]any{"code": code, "message": msg}
	if len(inner) > 0 {
		e["innererror"] = map[string]any{"code": inner[0]}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"error": e})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

var rbacActions = map[string]string{
	"get":     "Microsoft.KeyVault/vaults/secrets/getSecret/action",
	"list":    "Microsoft.KeyVault/vaults/secrets/readMetadata/action",
	"set":     "Microsoft.KeyVault/vaults/secrets/setSecret/action",
	"delete":  "Microsoft.KeyVault/vaults/secrets/delete",
	"recover": "Microsoft.KeyVault/vaults/secrets/recover/action",
	"purge":   "Microsoft.KeyVault/vaults/secrets/purge/action",
	"backup":  "Microsoft.KeyVault/vaults/secrets/backup/action",
}

// allowed checks RBAC (enableRbacAuthorization) or the vault's access policies.
func (rp *RP) allowed(w http.ResponseWriter, vault map[string]any, id *auth.Identity, perm string) bool {
	p := props(vault)
	vaultName := strOf(vault["name"]) + ";location=" + strOf(vault["location"])
	caller := "appid=" + id.AppID + ";oid=" + id.ObjectID
	if p["enableRbacAuthorization"] == true {
		action := rbacActions[perm]
		if rp.s.RBAC.Allowed(id, action, strOf(vault["id"]), true) {
			return true
		}
		kvError(w, 403, "Forbidden", fmt.Sprintf("Caller is not authorized to perform action on resource.\r\nIf role assignments, deny assignments or role definitions were changed recently, please observe propagation time.\r\nCaller: %s\r\nAction: '%s'\r\nResource: '%s'\r\nAssignment: (not found)\r\nDecisionReason: 'DeniedWithNoValidRBAC' \r\nVault: %s\r\n", caller, action, strings.ToLower(strOf(vault["id"])), vaultName), "ForbiddenByRbac")
		return false
	}
	principals := map[string]bool{strings.ToLower(id.ObjectID): true}
	for _, g := range rp.s.Dir.TransitiveGroups(id.TenantID, id.ObjectID) {
		principals[strings.ToLower(g)] = true
	}
	for _, x := range asList(p["accessPolicies"]) {
		ap, _ := x.(map[string]any)
		if !principals[strings.ToLower(strOf(ap["objectId"]))] {
			continue
		}
		perms, _ := ap["permissions"].(map[string]any)
		for _, s := range asList(perms["secrets"]) {
			if v := strings.ToLower(strOf(s)); v == perm || v == "all" {
				return true
			}
		}
	}
	kvError(w, 403, "Forbidden", fmt.Sprintf("The user, group or application '%s' does not have secrets %s permission on key vault '%s'. For help resolving this issue, please see https://go.microsoft.com/fwlink/?linkid=2125287", caller, perm, vaultName), "AccessDenied")
	return false
}

// ------------------------------------------------------------ secrets

type secretVersion struct {
	Value       string            `json:"value"`
	ContentType string            `json:"contentType,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
	Enabled     bool              `json:"enabled"`
	Exp         int64             `json:"exp,omitempty"`
	Nbf         int64             `json:"nbf,omitempty"`
	Created     int64             `json:"created"`
	Updated     int64             `json:"updated"`
	Version     string            `json:"version"`
}

type deletedSecret struct {
	Versions           []secretVersion `json:"versions"`
	DeletedDate        int64           `json:"deletedDate"`
	ScheduledPurgeDate int64           `json:"scheduledPurgeDate"`
}

func vkey(vault, secret string) string {
	return "kv/" + strings.ToLower(vault) + "/secrets/" + strings.ToLower(secret)
}
func dkey(vault, secret string) string {
	return "kv/" + strings.ToLower(vault) + "/deleted/" + strings.ToLower(secret)
}

func (rp *RP) versions(vault, secret string) ([]secretVersion, bool) {
	var v []secretVersion
	ok, err := store.GetJSON(rp.s.Store, store.BucketData, vkey(vault, secret), &v)
	return v, ok && err == nil && len(v) > 0
}

func (rp *RP) bundle(vault map[string]any, name string, v secretVersion, withValue bool) map[string]any {
	base := strings.TrimSuffix(strOf(props(vault)["vaultUri"]), "/")
	level := "Recoverable+Purgeable"
	if props(vault)["enablePurgeProtection"] == true {
		level = "Recoverable"
	}
	attrs := map[string]any{"enabled": v.Enabled, "created": v.Created, "updated": v.Updated, "recoveryLevel": level,
		"recoverableDays": props(vault)["softDeleteRetentionInDays"]}
	if v.Exp > 0 {
		attrs["exp"] = v.Exp
	}
	if v.Nbf > 0 {
		attrs["nbf"] = v.Nbf
	}
	b := map[string]any{"id": base + "/secrets/" + name + "/" + v.Version, "attributes": attrs}
	if withValue {
		b["value"] = v.Value
	}
	if v.ContentType != "" {
		b["contentType"] = v.ContentType
	}
	if len(v.Tags) > 0 {
		b["tags"] = v.Tags
	}
	return b
}

func (rp *RP) serveSecrets(w http.ResponseWriter, r *http.Request, vault map[string]any, id *auth.Identity) {
	vname := strOf(vault["name"])
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	notFound := func(name string) {
		kvError(w, 404, "SecretNotFound", fmt.Sprintf("A secret with (name/id) %s was not found in this key vault. If you recently deleted this secret you may be able to recover it using the correct recovery command. For help resolving this issue, please see https://go.microsoft.com/fwlink/?linkid=2125182", name))
	}
	now := time.Now().UTC().Unix()
	switch {
	// certificate contacts (read by Terraform's azurerm_key_vault)
	case len(parts) == 2 && parts[0] == "certificates" && parts[1] == "contacts":
		kvError(w, 404, "ContactsNotFound", "Contacts not found.")

	// list secrets
	case len(parts) == 1 && parts[0] == "secrets" && r.Method == http.MethodGet:
		if !rp.allowed(w, vault, id, "list") {
			return
		}
		var out []any
		prefix := "kv/" + strings.ToLower(vname) + "/secrets/"
		_ = rp.s.Store.Scan(store.BucketData, prefix, func(k string, v []byte) bool {
			var vs []secretVersion
			if json.Unmarshal(v, &vs) == nil && len(vs) > 0 {
				b := rp.bundle(vault, strings.TrimPrefix(k, prefix), vs[len(vs)-1], false)
				b["id"] = strings.TrimSuffix(strOf(props(vault)["vaultUri"]), "/") + "/secrets/" + strings.TrimPrefix(k, prefix)
				out = append(out, b)
			}
			return true
		})
		if out == nil {
			out = []any{}
		}
		writeJSON(w, 200, map[string]any{"value": out, "nextLink": nil})

	// set secret
	case len(parts) == 2 && parts[0] == "secrets" && r.Method == http.MethodPut:
		if !rp.allowed(w, vault, id, "set") {
			return
		}
		name := parts[1]
		if _, deleted := rp.deletedSecret(vname, name); deleted {
			kvError(w, 409, "Conflict", fmt.Sprintf("Secret %s is currently in a deleted but recoverable state, and its name cannot be reused; in this state, the secret can only be recovered or purged.", name), "ObjectIsDeletedButRecoverable")
			return
		}
		var in struct {
			Value       *string           `json:"value"`
			ContentType string            `json:"contentType"`
			Tags        map[string]string `json:"tags"`
			Attributes  struct {
				Enabled *bool `json:"enabled"`
				Exp     int64 `json:"exp"`
				Nbf     int64 `json:"nbf"`
			} `json:"attributes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Value == nil {
			kvError(w, 400, "BadParameter", "The request body must be a JSON object with a 'value' property.")
			return
		}
		enabled := true
		if in.Attributes.Enabled != nil {
			enabled = *in.Attributes.Enabled
		}
		vs, _ := rp.versions(vname, name)
		v := secretVersion{Value: *in.Value, ContentType: in.ContentType, Tags: in.Tags, Enabled: enabled, Exp: in.Attributes.Exp, Nbf: in.Attributes.Nbf, Created: now, Updated: now, Version: newVersion()}
		vs = append(vs, v)
		_ = store.PutJSON(rp.s.Store, store.BucketData, vkey(vname, name), vs)
		writeJSON(w, 200, rp.bundle(vault, name, v, true))

	// get secret (latest or version) / update attributes
	case (len(parts) == 2 || len(parts) == 3) && parts[0] == "secrets" && (r.Method == http.MethodGet || r.Method == http.MethodPatch):
		name := parts[1]
		if len(parts) == 3 && parts[2] == "versions" {
			if !rp.allowed(w, vault, id, "list") {
				return
			}
			vs, ok := rp.versions(vname, name)
			if !ok {
				notFound(name)
				return
			}
			var out []any
			for _, v := range vs {
				out = append(out, rp.bundle(vault, name, v, false))
			}
			writeJSON(w, 200, map[string]any{"value": out, "nextLink": nil})
			return
		}
		perm := "get"
		if r.Method == http.MethodPatch {
			perm = "set"
		}
		if !rp.allowed(w, vault, id, perm) {
			return
		}
		vs, ok := rp.versions(vname, name)
		if !ok {
			notFound(name)
			return
		}
		idx := len(vs) - 1
		if len(parts) == 3 && parts[2] != "" {
			idx = -1
			for i, v := range vs {
				if v.Version == parts[2] {
					idx = i
				}
			}
			if idx < 0 {
				notFound(name + "/" + parts[2])
				return
			}
		}
		if r.Method == http.MethodPatch {
			var in struct {
				ContentType *string           `json:"contentType"`
				Tags        map[string]string `json:"tags"`
				Attributes  struct {
					Enabled *bool  `json:"enabled"`
					Exp     *int64 `json:"exp"`
					Nbf     *int64 `json:"nbf"`
				} `json:"attributes"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			v := &vs[idx]
			if in.ContentType != nil {
				v.ContentType = *in.ContentType
			}
			if in.Tags != nil {
				v.Tags = in.Tags
			}
			if in.Attributes.Enabled != nil {
				v.Enabled = *in.Attributes.Enabled
			}
			if in.Attributes.Exp != nil {
				v.Exp = *in.Attributes.Exp
			}
			if in.Attributes.Nbf != nil {
				v.Nbf = *in.Attributes.Nbf
			}
			v.Updated = now
			_ = store.PutJSON(rp.s.Store, store.BucketData, vkey(vname, name), vs)
			writeJSON(w, 200, rp.bundle(vault, name, *v, false))
			return
		}
		v := vs[idx]
		if !v.Enabled {
			kvError(w, 403, "Forbidden", "Operation get is not allowed on a disabled secret.", "SecretDisabled")
			return
		}
		writeJSON(w, 200, rp.bundle(vault, name, v, true))

	// delete secret (soft delete)
	case len(parts) == 2 && parts[0] == "secrets" && r.Method == http.MethodDelete:
		if !rp.allowed(w, vault, id, "delete") {
			return
		}
		name := parts[1]
		vs, ok := rp.versions(vname, name)
		if !ok {
			notFound(name)
			return
		}
		days, _ := props(vault)["softDeleteRetentionInDays"].(float64)
		if days == 0 {
			days = 90
		}
		d := deletedSecret{Versions: vs, DeletedDate: now, ScheduledPurgeDate: now + int64(days)*86400}
		_ = store.PutJSON(rp.s.Store, store.BucketData, dkey(vname, name), d)
		_, _ = rp.s.Store.Delete(store.BucketData, vkey(vname, name))
		writeJSON(w, 200, rp.deletedBundle(vault, name, d))

	// deleted secrets: list, get, purge, recover
	case parts[0] == "deletedsecrets":
		rp.serveDeleted(w, r, vault, id, parts)

	default:
		kvError(w, 400, "BadParameter", "CloudTyke Key Vault supports the secrets API (/secrets, /deletedsecrets). Keys and certificates arrive later.")
	}
}

func (rp *RP) deletedSecret(vault, name string) (deletedSecret, bool) {
	var d deletedSecret
	ok, err := store.GetJSON(rp.s.Store, store.BucketData, dkey(vault, name), &d)
	return d, ok && err == nil
}

func (rp *RP) deletedBundle(vault map[string]any, name string, d deletedSecret) map[string]any {
	b := rp.bundle(vault, name, d.Versions[len(d.Versions)-1], false)
	base := strings.TrimSuffix(strOf(props(vault)["vaultUri"]), "/")
	b["recoveryId"] = base + "/deletedsecrets/" + name
	b["deletedDate"] = d.DeletedDate
	b["scheduledPurgeDate"] = d.ScheduledPurgeDate
	return b
}

func (rp *RP) serveDeleted(w http.ResponseWriter, r *http.Request, vault map[string]any, id *auth.Identity, parts []string) {
	vname := strOf(vault["name"])
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		if !rp.allowed(w, vault, id, "list") {
			return
		}
		var out []any
		prefix := "kv/" + strings.ToLower(vname) + "/deleted/"
		var names []string
		_ = rp.s.Store.Scan(store.BucketData, prefix, func(k string, _ []byte) bool { names = append(names, strings.TrimPrefix(k, prefix)); return true })
		sort.Strings(names)
		for _, n := range names {
			if d, ok := rp.deletedSecret(vname, n); ok {
				out = append(out, rp.deletedBundle(vault, n, d))
			}
		}
		if out == nil {
			out = []any{}
		}
		writeJSON(w, 200, map[string]any{"value": out, "nextLink": nil})
	case len(parts) >= 2:
		name, _ := url.PathUnescape(parts[1])
		d, ok := rp.deletedSecret(vname, name)
		if !ok {
			kvError(w, 404, "SecretNotFound", fmt.Sprintf("Deleted Secret not found: %s", name))
			return
		}
		switch {
		case len(parts) == 2 && r.Method == http.MethodGet:
			if rp.allowed(w, vault, id, "list") {
				writeJSON(w, 200, rp.deletedBundle(vault, name, d))
			}
		case len(parts) == 2 && r.Method == http.MethodDelete:
			if !rp.allowed(w, vault, id, "purge") {
				return
			}
			if props(vault)["enablePurgeProtection"] == true {
				kvError(w, 403, "Forbidden", "Operation \"purge\" is not allowed because purge protection is enabled for this vault. Key Vault service will automatically purge it after the retention period has passed.", "OperationNotAllowed")
				return
			}
			_, _ = rp.s.Store.Delete(store.BucketData, dkey(vname, name))
			w.WriteHeader(204)
		case len(parts) == 3 && parts[2] == "recover" && r.Method == http.MethodPost:
			if !rp.allowed(w, vault, id, "recover") {
				return
			}
			_ = store.PutJSON(rp.s.Store, store.BucketData, vkey(vname, name), d.Versions)
			_, _ = rp.s.Store.Delete(store.BucketData, dkey(vname, name))
			writeJSON(w, 200, rp.bundle(vault, name, d.Versions[len(d.Versions)-1], false))
		default:
			kvError(w, 405, "MethodNotAllowed", "Unsupported operation on a deleted secret.")
		}
	}
}
