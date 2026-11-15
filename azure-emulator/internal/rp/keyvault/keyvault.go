// Package keyvault implements Azure Key Vault: the Microsoft.KeyVault
// control plane (vaults, access policies, soft delete, purge protection,
// deleted vaults) and the secrets data plane at
// https://<vault>.vault.<domain>/secrets, authorised with Azure RBAC or
// vault access policies exactly like Azure, including soft-deleted secrets.
package keyvault

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/arm"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

const ns = "Microsoft.KeyVault"

// VaultType is the full resource type of a vault.
const VaultType = ns + "/vaults"

var versions = []string{"2024-11-01", "2024-04-01-preview", "2023-07-01", "2023-02-01", "2022-07-01", "2021-10-01"}

var nameRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{1,22}[a-zA-Z0-9]$`)

// RP is the Key Vault resource provider.
type RP struct{ s *arm.Server }

// Register wires the RP and returns it (for the data plane).
func Register(s *arm.Server) *RP {
	rp := &RP{s: s}
	reg := func(types []string, scope arm.ScopeKind, notes string, h arm.Handler) {
		s.Registry.Register(&arm.TypeRegistration{Namespace: ns, Types: types, APIVersions: versions, Scopes: []arm.ScopeKind{scope}, Phase: 2, Notes: notes, Handler: h})
	}
	reg([]string{"vaults"}, arm.KindResourceGroup, "globally unique names, standard/premium, RBAC or access policies, soft delete (7-90 days) and purge protection (cannot be turned off), network ACLs, vaultUri; deleting moves the vault to deletedVaults",
		arm.Generic(arm.GenericOptions{Tracked: true, Async: true, DeleteStatus: 200, Prepare: rp.prepare, Present: present,
			OnDelete: func(q *arm.Request, existing map[string]any) error { rp.softDelete(existing); return nil }}))
	reg([]string{"vaults", "accessPolicies"}, arm.KindResourceGroup, "PUT .../accessPolicies/add|replace|remove: merge access policy entries",
		rp.accessPolicies)
	reg([]string{"locations", "deletedVaults"}, arm.KindSubscription, "GET a soft-deleted vault; POST .../purge removes it (blocked while purge protection is on)", rp.deletedVault)
	reg([]string{"deletedVaults"}, arm.KindSubscription, "list soft-deleted vaults in the subscription", rp.listDeleted)
	reg([]string{"checkNameAvailability"}, arm.KindSubscription, "POST: vault names are global", rp.checkName)
	return rp
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

func present(q *arm.Request, b map[string]any) map[string]any { return b }

func newVersion() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// VaultURI returns the vault's data-plane URL.
func (rp *RP) VaultURI(name string) string {
	return "https://" + strings.ToLower(name) + "." + rp.s.Cfg.Host("vault") + "/"
}

// vaultByName finds a live vault anywhere (names are global).
func (rp *RP) vaultByName(name string) (map[string]any, bool) {
	var found map[string]any
	suffix := "/providers/microsoft.keyvault/vaults/" + strings.ToLower(name)
	_ = rp.s.Store.Scan(store.BucketARM, "/subscriptions/", func(k string, v []byte) bool {
		if strings.HasSuffix(k, suffix) {
			_ = json.Unmarshal(v, &found)
			return false
		}
		return true
	})
	return found, found != nil
}

func deletedKey(name string) string { return "kv-deleted-vault/" + strings.ToLower(name) }

func (rp *RP) deleted(name string) (map[string]any, bool) {
	var m map[string]any
	ok, err := store.GetJSON(rp.s.Store, store.BucketData, deletedKey(name), &m)
	return m, ok && err == nil
}

func (rp *RP) prepare(q *arm.Request, body, existing map[string]any) error {
	name := q.ID.Name()
	if !nameRe.MatchString(name) || strings.Contains(name, "--") {
		return arm.BadRequest("VaultNameNotValid", arm.Errorf("The vault name '%s' is invalid. A vault's name must be between 3-24 alphanumeric characters. The name must begin with a letter, end with a letter or digit, and not contain consecutive hyphens.", name))
	}
	p := props(body)
	if existing == nil {
		if other, ok := rp.vaultByName(name); ok && !strings.EqualFold(strOf(other["id"]), strOf(body["id"])) {
			return &arm.APIError{Status: 409, Code: "VaultAlreadyExists", Message: arm.Errorf("The vault name '%s' is already in use. Vault names are globally unique so it is possible that the name is already taken.", name)}
		}
		if d, ok := rp.deleted(name); ok {
			if strings.EqualFold(strOf(p["createMode"]), "recover") {
				rp.recoverVault(d, body)
			} else {
				return &arm.APIError{Status: 409, Code: "ConflictError", Message: arm.Errorf("A vault with the same name already exists in deleted state. You need to either recover or purge existing key vault. Follow this link https://go.microsoft.com/fwlink/?linkid=2149745 for more information on soft delete.")}
			}
		}
	}
	delete(p, "createMode")
	if strOf(p["tenantId"]) == "" {
		return arm.BadRequest("BadRequest", "properties.tenantId is required.")
	}
	sku, _ := p["sku"].(map[string]any)
	if sku == nil {
		return arm.BadRequest("BadRequest", "properties.sku is required (family A, name standard or premium).")
	}
	switch strings.ToLower(strOf(sku["name"])) {
	case "standard", "premium":
	default:
		return arm.BadRequest("BadRequest", arm.Errorf("The SKU '%v' is not valid. Use standard or premium.", sku["name"]))
	}
	if strOf(sku["family"]) == "" {
		sku["family"] = "A"
	}
	if p["enableSoftDelete"] == false {
		return arm.BadRequest("BadRequest", "Soft delete can no longer be disabled on key vaults (enableSoftDelete must be true).")
	}
	p["enableSoftDelete"] = true
	if r, ok := p["softDeleteRetentionInDays"].(float64); ok {
		if r < 7 || r > 90 {
			return arm.BadRequest("BadRequest", "softDeleteRetentionInDays must be between 7 and 90.")
		}
	} else {
		p["softDeleteRetentionInDays"] = 90
	}
	if existing != nil {
		ep := props(existing)
		if ep["enablePurgeProtection"] == true && p["enablePurgeProtection"] != true {
			return arm.BadRequest("BadRequest", "Purge protection cannot be disabled once enabled.")
		}
		if ep["softDeleteRetentionInDays"] != nil && p["softDeleteRetentionInDays"] != ep["softDeleteRetentionInDays"] && ep["enablePurgeProtection"] == true {
			return arm.BadRequest("BadRequest", "softDeleteRetentionInDays cannot be changed once the vault has purge protection.")
		}
		if p["accessPolicies"] == nil {
			p["accessPolicies"] = ep["accessPolicies"]
		}
	}
	if p["accessPolicies"] == nil {
		p["accessPolicies"] = []any{}
	}
	for k, v := range map[string]any{"enableRbacAuthorization": false, "enabledForDeployment": false, "enabledForDiskEncryption": false, "enabledForTemplateDeployment": false, "publicNetworkAccess": "Enabled"} {
		if _, ok := p[k]; !ok {
			p[k] = v
		}
	}
	if p["networkAcls"] == nil {
		p["networkAcls"] = map[string]any{"bypass": "AzureServices", "defaultAction": "Allow", "ipRules": []any{}, "virtualNetworkRules": []any{}}
	}
	p["vaultUri"] = rp.VaultURI(name)
	return nil
}

// ------------------------------------------------------------ soft delete

func (rp *RP) softDelete(v map[string]any) {
	p := props(v)
	days, _ := p["softDeleteRetentionInDays"].(float64)
	if days == 0 {
		days = 90
	}
	now := time.Now().UTC()
	d := map[string]any{
		"id":   "/subscriptions/" + arm.ScopeSubscription(strOf(v["id"])) + "/providers/Microsoft.KeyVault/locations/" + strOf(v["location"]) + "/deletedVaults/" + strOf(v["name"]),
		"name": v["name"], "type": "Microsoft.KeyVault/deletedVaults",
		"properties": map[string]any{"vaultId": v["id"], "location": v["location"], "deletionDate": now.Format(time.RFC3339),
			"scheduledPurgeDate": now.Add(time.Duration(days) * 24 * time.Hour).Format(time.RFC3339), "tags": v["tags"], "purgeProtectionEnabled": p["enablePurgeProtection"] == true},
		"_vault": v,
	}
	_ = store.PutJSON(rp.s.Store, store.BucketData, deletedKey(strOf(v["name"])), d)
}

func (rp *RP) recoverVault(d, body map[string]any) {
	old, _ := d["_vault"].(map[string]any)
	if old != nil {
		bp, op := props(body), props(old)
		for k, v := range op {
			if _, ok := bp[k]; !ok {
				bp[k] = v
			}
		}
	}
	_, _ = rp.s.Store.Delete(store.BucketData, deletedKey(strOf(d["name"])))
}

func publicDeleted(d map[string]any) map[string]any {
	c := arm.DeepCopy(d)
	delete(c, "_vault")
	return c
}

func (rp *RP) deletedVault(q *arm.Request) {
	if len(q.ID.Names) < 2 {
		q.Error(404, "NotFound", "Use .../locations/{location}/deletedVaults/{name}.")
		return
	}
	name := q.ID.Names[1]
	d, ok := rp.deleted(name)
	if !ok || !strings.EqualFold(arm.ScopeSubscription(strOf(d["id"])), q.SubscriptionID()) {
		q.Error(404, "DeletedVaultNotFound", arm.Errorf("The specified deleted vault '%s' does not exist.", name))
		return
	}
	switch {
	case q.R.Method == http.MethodGet:
		q.JSON(200, publicDeleted(d))
	case q.R.Method == http.MethodPost && strings.EqualFold(q.ActionName(), "purge"):
		if props(d)["purgeProtectionEnabled"] == true {
			q.Error(409, "ConflictError", arm.Errorf("Operation \"purge\" is not allowed because purge protection is enabled for this vault. Key Vault service will automatically purge it after the retention period has passed."))
			return
		}
		rp.purge(name)
		op := q.S.Ops.Start(q.SubscriptionID(), strOf(d["id"]), "Succeeded", nil)
		q.AsyncHeaders(op, ns, strOf(props(d)["location"]))
		q.Empty(202)
	default:
		q.Error(405, "MethodNotAllowed", "GET or POST .../purge.")
	}
}

func (rp *RP) purge(name string) {
	_, _ = rp.s.Store.Delete(store.BucketData, deletedKey(name))
	prefix := "kv/" + strings.ToLower(name) + "/"
	var keys []string
	_ = rp.s.Store.Scan(store.BucketData, prefix, func(k string, _ []byte) bool { keys = append(keys, k); return true })
	for _, k := range keys {
		_, _ = rp.s.Store.Delete(store.BucketData, k)
	}
}

func (rp *RP) listDeleted(q *arm.Request) {
	var out []any
	_ = rp.s.Store.Scan(store.BucketData, "kv-deleted-vault/", func(_ string, v []byte) bool {
		var d map[string]any
		if json.Unmarshal(v, &d) == nil && strings.EqualFold(arm.ScopeSubscription(strOf(d["id"])), q.SubscriptionID()) {
			out = append(out, publicDeleted(d))
		}
		return true
	})
	if out == nil {
		out = []any{}
	}
	q.JSON(200, map[string]any{"value": out})
}

func (rp *RP) checkName(q *arm.Request) {
	in, _ := q.Body()
	name := strOf(in["name"])
	switch {
	case !nameRe.MatchString(name):
		q.JSON(200, map[string]any{"nameAvailable": false, "reason": "AccountNameInvalid", "message": "Vault name must be 3-24 alphanumeric characters or hyphens."})
	case func() bool { _, ok := rp.vaultByName(name); return ok }():
		q.JSON(200, map[string]any{"nameAvailable": false, "reason": "AlreadyExists", "message": "The vault name '" + name + "' is already in use."})
	default:
		q.JSON(200, map[string]any{"nameAvailable": true})
	}
}

// ------------------------------------------------------------ access policies

func (rp *RP) accessPolicies(q *arm.Request) {
	if q.R.Method != http.MethodPut {
		q.Error(405, "MethodNotAllowed", "Use PUT .../accessPolicies/add|replace|remove.")
		return
	}
	op := strings.ToLower(q.ID.Name())
	if op != "add" && op != "replace" && op != "remove" {
		q.Error(400, "BadRequest", "The access policy operation must be add, replace or remove.")
		return
	}
	vaultID := q.ID.ParentPath(ns, q.Reg.Types)
	v, ok := q.S.LoadResource(vaultID)
	if !ok {
		q.NotFound()
		return
	}
	in, ok := q.BodyOrError()
	if !ok {
		return
	}
	incoming, _ := props(in)["accessPolicies"].([]any)
	vp := props(v)
	cur, _ := vp["accessPolicies"].([]any)
	key := func(m map[string]any) string {
		return strings.ToLower(strOf(m["tenantId"]) + "|" + strOf(m["objectId"]) + "|" + strOf(m["applicationId"]))
	}
	switch op {
	case "replace":
		cur = incoming
	case "add":
		for _, x := range incoming {
			nm, _ := x.(map[string]any)
			merged := false
			for _, c := range cur {
				cm, _ := c.(map[string]any)
				if key(cm) == key(nm) {
					cp, _ := cm["permissions"].(map[string]any)
					np, _ := nm["permissions"].(map[string]any)
					for kind, perms := range np {
						cp[kind] = union(cp[kind], perms)
					}
					merged = true
				}
			}
			if !merged {
				cur = append(cur, nm)
			}
		}
	case "remove":
		var keep []any
		for _, c := range cur {
			cm, _ := c.(map[string]any)
			drop := false
			for _, x := range incoming {
				if key(cm) == key(x.(map[string]any)) {
					drop = true
				}
			}
			if !drop {
				keep = append(keep, cm)
			}
		}
		cur = keep
	}
	if cur == nil {
		cur = []any{}
	}
	vp["accessPolicies"] = cur
	v["etag"] = arm.NewETag()
	_ = q.S.SaveResource(v)
	q.JSON(200, map[string]any{"id": vaultID + "/accessPolicies/" + op, "type": ns + "/vaults/accessPolicies", "properties": map[string]any{"accessPolicies": cur}})
}

func union(a, b any) []any {
	seen := map[string]bool{}
	var out []any
	for _, l := range [][]any{asList(a), asList(b)} {
		for _, x := range l {
			s := strings.ToLower(strOf(x))
			if !seen[s] {
				seen[s] = true
				out = append(out, x)
			}
		}
	}
	return out
}

func asList(v any) []any { l, _ := v.([]any); return l }
