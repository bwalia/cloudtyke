// Package storage implements Azure Storage: the Microsoft.Storage control
// plane (accounts, blob / file / queue / table services, containers,
// shares, lifecycle policies, keys) and Entra ID-authorised Blob and Files
// data planes at https://<account>.blob.storage.<domain> and
// https://<account>.file.storage.<domain>.
package storage

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/arm"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

const ns = "Microsoft.Storage"

// AccountType is the full type of a storage account.
const AccountType = ns + "/storageAccounts"

var versions = []string{"2024-01-01", "2023-05-01", "2023-01-01", "2022-09-01", "2021-09-01"}

var (
	nameRe      = regexp.MustCompile(`^[a-z0-9]{3,24}$`)
	containerRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9]|-[a-z0-9]){2,62}$`)
	skus        = map[string]bool{"Standard_LRS": true, "Standard_GRS": true, "Standard_RAGRS": true, "Standard_ZRS": true, "Standard_GZRS": true, "Standard_RAGZRS": true, "Premium_LRS": true, "Premium_ZRS": true}
	kinds       = map[string]bool{"StorageV2": true, "Storage": true, "BlobStorage": true, "BlockBlobStorage": true, "FileStorage": true}
)

// RP is the storage resource provider.
type RP struct{ s *arm.Server }

// Register wires the RP.
func Register(s *arm.Server) *RP {
	rp := &RP{s: s}
	reg := func(types []string, scope arm.ScopeKind, notes string, h arm.Handler) {
		s.Registry.Register(&arm.TypeRegistration{Namespace: ns, Types: types, APIVersions: versions, Scopes: []arm.ScopeKind{scope}, Phase: 2, Notes: notes, Handler: h})
	}
	reg([]string{"storageAccounts"}, arm.KindResourceGroup, "globally unique 3-24 lowercase names, SKU / kind rules, TLS, HTTPS-only, public blob access, shared key access, network ACLs, endpoints for blob/file/queue/table/dfs/web, listKeys / regenerateKey",
		arm.Generic(arm.GenericOptions{Tracked: true, Async: true, CreateStatus: 202, DeleteStatus: 200, Prepare: rp.prepareAccount, Present: presentAccount,
			Actions: map[string]arm.Handler{"listKeys": rp.listKeys, "regenerateKey": rp.regenerateKey},
			AfterWrite: func(q *arm.Request, b map[string]any) {
				for _, svc := range []string{"blobServices", "fileServices", "queueServices", "tableServices"} {
					id := strOf(b["id"]) + "/" + svc + "/default"
					if _, ok := q.S.LoadResource(id); !ok {
						_ = q.S.SaveResource(map[string]any{"id": id, "name": "default", "type": ns + "/storageAccounts/" + svc, "properties": serviceDefaults(svc)})
					}
				}
			},
			OnDelete: func(q *arm.Request, existing map[string]any) error { rp.dropData(strOf(existing["name"])); return nil }}))
	for _, svc := range []string{"blobServices", "fileServices", "queueServices", "tableServices"} {
		svc := svc
		reg([]string{"storageAccounts", svc}, arm.KindResourceGroup, svc+"/default: service properties (soft delete, versioning, CORS, SMB settings)",
			arm.Generic(arm.GenericOptions{Prepare: func(q *arm.Request, body, existing map[string]any) error {
				if q.ID.Name() != "default" {
					return arm.BadRequest("InvalidResourceName", "The only service name is 'default'.")
				}
				p := body["properties"].(map[string]any)
				for k, v := range serviceDefaults(svc) {
					if _, ok := p[k]; !ok {
						p[k] = v
					}
				}
				return nil
			}}))
	}
	reg([]string{"storageAccounts", "blobServices", "containers"}, arm.KindResourceGroup, "containers: name rules, publicAccess blocked when the account disallows public blob access, metadata",
		arm.Generic(arm.GenericOptions{Prepare: rp.prepareContainer,
			OnDelete: func(q *arm.Request, existing map[string]any) error {
				rp.dropPrefix("blob/" + accountOf(q) + "/" + strings.ToLower(q.ID.Name()) + "/")
				return nil
			}}))
	reg([]string{"storageAccounts", "fileServices", "shares"}, arm.KindResourceGroup, "Azure Files shares: quota 1-102400 GiB, access tier, SMB / NFS",
		arm.Generic(arm.GenericOptions{Prepare: rp.prepareShare,
			OnDelete: func(q *arm.Request, existing map[string]any) error {
				rp.dropPrefix("file/" + accountOf(q) + "/" + strings.ToLower(q.ID.Name()) + "/")
				return nil
			}}))
	reg([]string{"storageAccounts", "queueServices", "queues"}, arm.KindResourceGroup, "queues (control plane)", arm.Generic(arm.GenericOptions{}))
	reg([]string{"storageAccounts", "tableServices", "tables"}, arm.KindResourceGroup, "tables (control plane)", arm.Generic(arm.GenericOptions{}))
	reg([]string{"storageAccounts", "managementPolicies"}, arm.KindResourceGroup, "lifecycle management rules (tier to cool/archive, delete after N days)",
		arm.Generic(arm.GenericOptions{CreateStatus: 200, Prepare: func(q *arm.Request, body, existing map[string]any) error {
			if q.ID.Name() != "default" {
				return arm.BadRequest("InvalidResourceName", "The only management policy name is 'default'.")
			}
			pol, _ := body["properties"].(map[string]any)["policy"].(map[string]any)
			if pol == nil || len(asList(pol["rules"])) == 0 {
				return arm.BadRequest("InvalidManagementPolicy", "properties.policy.rules must contain at least one rule.")
			}
			body["properties"].(map[string]any)["lastModifiedTime"] = time.Now().UTC().Format(time.RFC3339Nano)
			return nil
		}}))
	reg([]string{"checkNameAvailability"}, arm.KindSubscription, "POST: storage account names are global", rp.checkName)
	// accountMigrations/default: redundancy migrations (LRS <-> ZRS). The emulator
	// applies SKU changes at once, so the latest migration is always complete.
	reg([]string{"storageAccounts", "accountMigrations"}, arm.KindResourceGroup, "GET default: migration status (always Complete; SKU changes apply immediately)",
		func(q *arm.Request) {
			if q.R.Method != http.MethodGet {
				q.Error(405, "MethodNotAllowed", "Start a migration by changing the account SKU.")
				return
			}
			acct, ok := q.S.LoadResource(q.ID.Scope + "/providers/" + ns + "/storageAccounts/" + q.ID.Names[0])
			if !ok || q.ID.Name() != "default" {
				q.NotFound()
				return
			}
			sku, _ := acct["sku"].(map[string]any)
			q.JSON(200, map[string]any{"id": strOf(acct["id"]) + "/accountMigrations/default", "name": "default", "type": ns + "/storageAccounts/accountMigrations",
				"properties": map[string]any{"targetSkuName": strOf(sku["name"]), "migrationStatus": "Complete"}})
		})
	return rp
}

func strOf(v any) string { s, _ := v.(string); return s }

func asList(v any) []any { l, _ := v.([]any); return l }

func props(b map[string]any) map[string]any {
	p, _ := b["properties"].(map[string]any)
	if p == nil {
		p = map[string]any{}
		b["properties"] = p
	}
	return p
}

func accountOf(q *arm.Request) string { return strings.ToLower(q.ID.Names[0]) }

func serviceDefaults(svc string) map[string]any {
	switch svc {
	case "blobServices":
		return map[string]any{"cors": map[string]any{"corsRules": []any{}}, "deleteRetentionPolicy": map[string]any{"enabled": false, "allowPermanentDelete": false}, "isVersioningEnabled": false}
	case "fileServices":
		return map[string]any{"cors": map[string]any{"corsRules": []any{}}, "shareDeleteRetentionPolicy": map[string]any{"enabled": true, "days": 7}, "protocolSettings": map[string]any{"smb": map[string]any{}}}
	}
	return map[string]any{"cors": map[string]any{"corsRules": []any{}}}
}

func newKey() string {
	b := make([]byte, 64)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func (rp *RP) accountByName(name string) (map[string]any, bool) {
	var found map[string]any
	suffix := "/providers/microsoft.storage/storageaccounts/" + strings.ToLower(name)
	_ = rp.s.Store.Scan(store.BucketARM, "/subscriptions/", func(k string, v []byte) bool {
		if strings.HasSuffix(k, suffix) {
			_ = json.Unmarshal(v, &found)
			return false
		}
		return true
	})
	return found, found != nil
}

func (rp *RP) prepareAccount(q *arm.Request, body, existing map[string]any) error {
	name := q.ID.Name()
	if !nameRe.MatchString(name) {
		return arm.BadRequest("AccountNameInvalid", arm.Errorf("%s is not a valid storage account name. Storage account name must be between 3 and 24 characters in length and use numbers and lower-case letters only.", name))
	}
	if existing == nil {
		if other, ok := rp.accountByName(name); ok && !strings.EqualFold(strOf(other["id"]), strOf(body["id"])) {
			return &arm.APIError{Status: 409, Code: "StorageAccountAlreadyTaken", Message: arm.Errorf("The storage account named %s is already taken.", name)}
		}
	}
	sku, _ := body["sku"].(map[string]any)
	if sku == nil || !skus[strOf(sku["name"])] {
		return arm.BadRequest("InvalidSku", "sku.name must be one of Standard_LRS, Standard_GRS, Standard_RAGRS, Standard_ZRS, Standard_GZRS, Standard_RAGZRS, Premium_LRS, Premium_ZRS.")
	}
	sku["tier"] = strings.SplitN(strOf(sku["name"]), "_", 2)[0]
	kind := strOf(body["kind"])
	if kind == "" {
		kind = "StorageV2"
		body["kind"] = kind
	}
	if !kinds[kind] {
		return arm.BadRequest("InvalidKind", "kind must be StorageV2, Storage, BlobStorage, BlockBlobStorage or FileStorage.")
	}
	if kind == "FileStorage" && sku["tier"] != "Premium" {
		return arm.BadRequest("InvalidKindForSku", "FileStorage accounts require a Premium SKU.")
	}
	p := props(body)
	defaults := map[string]any{"minimumTlsVersion": "TLS1_2", "supportsHttpsTrafficOnly": true, "allowBlobPublicAccess": false, "allowSharedKeyAccess": true,
		"publicNetworkAccess": "Enabled", "isHnsEnabled": false, "isSftpEnabled": false, "isNfsV3Enabled": false, "allowCrossTenantReplication": false,
		"defaultToOAuthAuthentication": false, "dnsEndpointType": "Standard"}
	for k, v := range defaults {
		if _, ok := p[k]; !ok {
			p[k] = v
		}
	}
	switch strOf(p["minimumTlsVersion"]) {
	case "TLS1_0", "TLS1_1", "TLS1_2", "TLS1_3":
	default:
		return arm.BadRequest("InvalidMinimumTlsVersion", "minimumTlsVersion must be TLS1_0, TLS1_1, TLS1_2 or TLS1_3.")
	}
	if kind != "FileStorage" && kind != "BlockBlobStorage" && strOf(p["accessTier"]) == "" {
		p["accessTier"] = "Hot"
	}
	if p["isHnsEnabled"] == true && kind != "StorageV2" && kind != "BlockBlobStorage" {
		return arm.BadRequest("HnsNotSupportedForKind", "Hierarchical namespace (Data Lake Gen2) requires StorageV2 or BlockBlobStorage.")
	}
	if p["networkAcls"] == nil {
		p["networkAcls"] = map[string]any{"bypass": "AzureServices", "defaultAction": "Allow", "ipRules": []any{}, "virtualNetworkRules": []any{}}
	}
	if p["encryption"] == nil {
		p["encryption"] = map[string]any{"keySource": "Microsoft.Storage", "requireInfrastructureEncryption": false,
			"services": map[string]any{"blob": map[string]any{"enabled": true, "keyType": "Account"}, "file": map[string]any{"enabled": true, "keyType": "Account"}}}
	}
	host := func(svc string) string {
		return "https://" + strings.ToLower(name) + "." + svc + "." + rp.s.Cfg.Host("storage") + "/"
	}
	p["primaryEndpoints"] = map[string]any{"blob": host("blob"), "file": host("file"), "queue": host("queue"), "table": host("table"), "dfs": host("dfs"), "web": host("web")}
	p["primaryLocation"] = body["location"]
	p["statusOfPrimary"] = "available"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if existing != nil {
		ep := props(existing)
		p["creationTime"], p["_keys"], p["keyCreationTime"] = ep["creationTime"], ep["_keys"], ep["keyCreationTime"]
	} else {
		p["creationTime"] = now
		p["_keys"] = []any{newKey(), newKey()}
		p["keyCreationTime"] = map[string]any{"key1": now, "key2": now}
	}
	return nil
}

func presentAccount(q *arm.Request, b map[string]any) map[string]any {
	b = arm.DeepCopy(b)
	delete(props(b), "_keys")
	return b
}

func (rp *RP) listKeys(q *arm.Request) {
	a, _ := q.S.LoadResource(q.ID.Path(ns, q.Reg.Types))
	p := props(a)
	if p["allowSharedKeyAccess"] == false {
		// Azure still returns keys; they just cannot be used. Mirror that.
	}
	keys := asList(p["_keys"])
	q.JSON(200, map[string]any{"keys": []any{
		map[string]any{"keyName": "key1", "value": keys[0], "permissions": "FULL", "creationTime": p["creationTime"]},
		map[string]any{"keyName": "key2", "value": keys[1], "permissions": "FULL", "creationTime": p["creationTime"]},
	}})
}

func (rp *RP) regenerateKey(q *arm.Request) {
	path := q.ID.Path(ns, q.Reg.Types)
	a, _ := q.S.LoadResource(path)
	in, _ := q.Body()
	p := props(a)
	keys := asList(p["_keys"])
	if strOf(in["keyName"]) == "key2" {
		keys[1] = newKey()
	} else {
		keys[0] = newKey()
	}
	p["_keys"] = keys
	_ = q.S.SaveResource(a)
	rp.listKeys(q)
}

func (rp *RP) checkName(q *arm.Request) {
	in, _ := q.Body()
	name := strOf(in["name"])
	switch {
	case !nameRe.MatchString(name):
		q.JSON(200, map[string]any{"nameAvailable": false, "reason": "AccountNameInvalid", "message": name + " is not a valid storage account name."})
	case func() bool { _, ok := rp.accountByName(name); return ok }():
		q.JSON(200, map[string]any{"nameAvailable": false, "reason": "AlreadyExists", "message": "The storage account named " + name + " is already taken."})
	default:
		q.JSON(200, map[string]any{"nameAvailable": true})
	}
}

func (rp *RP) prepareContainer(q *arm.Request, body, existing map[string]any) error {
	name := q.ID.Name()
	if !containerRe.MatchString(name) && name != "$web" && name != "$logs" {
		return arm.BadRequest("ContainerNameInvalid", arm.Errorf("The container name '%s' is invalid: use 3-63 lowercase letters, numbers and single hyphens.", name))
	}
	p := props(body)
	if strOf(p["publicAccess"]) == "" {
		p["publicAccess"] = "None"
	}
	acct, _ := q.S.LoadResource(q.ID.Scope + "/providers/" + ns + "/storageAccounts/" + q.ID.Names[0])
	if strOf(p["publicAccess"]) != "None" && props(acct)["allowBlobPublicAccess"] != true {
		return &arm.APIError{Status: 409, Code: "PublicAccessNotPermitted", Message: "Public access is not permitted on this storage account."}
	}
	if existing == nil {
		p["lastModifiedTime"] = time.Now().UTC().Format(time.RFC3339)
	}
	for k, v := range map[string]any{"leaseStatus": "Unlocked", "leaseState": "Available", "hasImmutabilityPolicy": false, "hasLegalHold": false, "defaultEncryptionScope": "$account-encryption-key", "denyEncryptionScopeOverride": false} {
		if _, ok := p[k]; !ok {
			p[k] = v
		}
	}
	return nil
}

func (rp *RP) prepareShare(q *arm.Request, body, existing map[string]any) error {
	name := q.ID.Name()
	if !containerRe.MatchString(name) {
		return arm.BadRequest("ShareNameInvalid", arm.Errorf("The share name '%s' is invalid: use 3-63 lowercase letters, numbers and single hyphens.", name))
	}
	p := props(body)
	quota, ok := p["shareQuota"].(float64)
	if !ok {
		quota = 5120
		p["shareQuota"] = quota
	}
	if quota < 1 || quota > 102400 {
		return arm.BadRequest("InvalidShareQuota", "shareQuota must be between 1 and 102400 GiB.")
	}
	if strOf(p["enabledProtocols"]) == "" {
		p["enabledProtocols"] = "SMB"
	}
	if strOf(p["accessTier"]) == "" {
		p["accessTier"] = "TransactionOptimized"
	}
	if existing == nil {
		p["lastModifiedTime"] = time.Now().UTC().Format(time.RFC3339)
	}
	return nil
}

func (rp *RP) dropData(account string) {
	rp.dropPrefix("blob/" + strings.ToLower(account) + "/")
	rp.dropPrefix("file/" + strings.ToLower(account) + "/")
}

func (rp *RP) dropPrefix(prefix string) {
	var keys []string
	_ = rp.s.Store.Scan(store.BucketData, prefix, func(k string, _ []byte) bool { keys = append(keys, k); return true })
	for _, k := range keys {
		_, _ = rp.s.Store.Delete(store.BucketData, k)
	}
}

// ContainerExists reports whether an ARM container exists for account/container.
func (rp *RP) container(acct map[string]any, name string) (map[string]any, bool) {
	return rp.s.LoadResource(strOf(acct["id"]) + "/blobServices/default/containers/" + name)
}

func (rp *RP) share(acct map[string]any, name string) (map[string]any, bool) {
	return rp.s.LoadResource(strOf(acct["id"]) + "/fileServices/default/shares/" + name)
}

var _ = http.MethodGet
