package integration

import (
	"strings"
	"testing"
)

func TestKeyVault(t *testing.T) {
	e := start(t)
	arm := e.url("management", "")
	tok := e.token(bootID, bootSec, arm+"/")
	kvTok := e.token(bootID, bootSec, "https://vault.cloudtyke.local")
	rdTok := e.token(readerID, readerSec, "https://vault.cloudtyke.local")
	sub := "/subscriptions/" + sandbox
	rg := sub + "/resourceGroups/rg-kv"
	const api = "?api-version=2023-07-01"
	do := func(m, u, tk string, body map[string]any) (int, map[string]any) {
		t.Helper()
		code, _, b := e.do(m, u, tk, body)
		return code, b
	}
	e.do("PUT", arm+rg+"?api-version=2024-11-01", tok, map[string]any{"location": "uksouth"})
	vault := func(name string, rbac bool, policies []any) string {
		code, b := do("PUT", arm+rg+"/providers/Microsoft.KeyVault/vaults/"+name+api, tok, map[string]any{"location": "uksouth", "properties": map[string]any{
			"tenantId": tenant, "sku": map[string]any{"family": "A", "name": "standard"}, "enableRbacAuthorization": rbac, "accessPolicies": policies, "enablePurgeProtection": false}})
		if code >= 300 {
			t.Fatalf("vault: %d %v", code, b)
		}
		return b["properties"].(map[string]any)["vaultUri"].(string)
	}

	// RBAC vault: Owner of the subscription still cannot read secrets
	uri := vault("kv-pay-rbac", true, nil)
	if !strings.HasPrefix(uri, "https://kv-pay-rbac.vault.cloudtyke.local") {
		t.Fatalf("vaultUri: %s", uri)
	}
	secret := uri + "secrets/db-password?api-version=7.4"
	if code, b := do("PUT", secret, kvTok, map[string]any{"value": "s3cr3t"}); code != 403 || errCode(b) != "Forbidden" {
		t.Fatalf("Owner without a data role should get 403: %d %v", code, b)
	}
	if code, b := do("PUT", arm+rg+"/providers/Microsoft.KeyVault/vaults/kv-pay-rbac/providers/Microsoft.Authorization/roleAssignments/2b6c6a7e-0000-4000-8000-000000000001?api-version=2022-04-01", tok, map[string]any{"properties": map[string]any{
		"roleDefinitionId": sub + "/providers/Microsoft.Authorization/roleDefinitions/b86a8fe4-44ce-4948-aee5-eccb2c155cd7", "principalId": "0b100000-0000-4000-8000-000000000001", "principalType": "ServicePrincipal"}}); code >= 300 {
		t.Fatalf("role assignment: %d %v", code, b)
	}
	if code, b := do("PUT", secret, kvTok, map[string]any{"value": "s3cr3t", "contentType": "text/plain"}); code != 200 || b["value"] != "s3cr3t" {
		t.Fatalf("set secret: %d %v", code, b)
	}
	do("PUT", secret, kvTok, map[string]any{"value": "rotated"})
	if code, b := do("GET", secret, kvTok, nil); code != 200 || b["value"] != "rotated" {
		t.Fatalf("get latest: %d %v", code, b)
	}
	if code, b := do("GET", uri+"secrets/db-password/versions?api-version=7.4", kvTok, nil); code != 200 || len(b["value"].([]any)) != 2 {
		t.Fatalf("versions: %d %v", code, b)
	}
	if code, _ := do("GET", secret, rdTok, nil); code != 403 {
		t.Fatalf("reader should be denied: %d", code)
	}
	if code, _ := do("GET", secret, "", nil); code != 401 {
		t.Fatalf("no token should be 401: %d", code)
	}
	// soft delete -> recover -> delete -> purge
	if code, b := do("DELETE", secret, kvTok, nil); code != 200 || b["recoveryId"] == nil {
		t.Fatalf("delete: %d %v", code, b)
	}
	if code, b := do("PUT", secret, kvTok, map[string]any{"value": "x"}); code != 409 {
		t.Fatalf("name reuse while deleted: %d %v", code, b)
	}
	if code, b := do("POST", uri+"deletedsecrets/db-password/recover?api-version=7.4", kvTok, nil); code != 200 {
		t.Fatalf("recover: %d %v", code, b)
	}
	do("DELETE", secret, kvTok, nil)
	if code, _ := do("DELETE", uri+"deletedsecrets/db-password?api-version=7.4", kvTok, nil); code != 204 {
		t.Fatalf("purge: %d", code)
	}

	// access-policy vault: the reader gets get/list only
	uri2 := vault("kv-pay-ap", false, []any{
		map[string]any{"tenantId": tenant, "objectId": "0b100000-0000-4000-8000-000000000001", "permissions": map[string]any{"secrets": []any{"Get", "List", "Set", "Delete", "Purge"}}},
		map[string]any{"tenantId": tenant, "objectId": "0b100000-0000-4000-8000-000000000002", "permissions": map[string]any{"secrets": []any{"Get", "List"}}}})
	do("PUT", uri2+"secrets/psp-api-key?api-version=7.4", kvTok, map[string]any{"value": "pk_test_123"})
	if code, b := do("GET", uri2+"secrets/psp-api-key?api-version=7.4", rdTok, nil); code != 200 || b["value"] != "pk_test_123" {
		t.Fatalf("reader with get policy: %d %v", code, b)
	}
	if code, b := do("PUT", uri2+"secrets/psp-api-key?api-version=7.4", rdTok, map[string]any{"value": "x"}); code != 403 || !strings.Contains(b["error"].(map[string]any)["message"].(string), "does not have secrets set permission") {
		t.Fatalf("reader set should be denied: %d %v", code, b)
	}

	// soft-deleted vault blocks the name until purged
	do("DELETE", arm+rg+"/providers/Microsoft.KeyVault/vaults/kv-pay-ap"+api, tok, nil)
	if code, b := do("PUT", arm+rg+"/providers/Microsoft.KeyVault/vaults/kv-pay-ap"+api, tok, map[string]any{"location": "uksouth", "properties": map[string]any{"tenantId": tenant, "sku": map[string]any{"family": "A", "name": "standard"}}}); code != 409 {
		t.Fatalf("recreate over a soft-deleted vault: %d %v", code, b)
	}
	if code, b := do("POST", arm+sub+"/providers/Microsoft.KeyVault/locations/uksouth/deletedVaults/kv-pay-ap/purge"+api, tok, nil); code != 202 {
		t.Fatalf("purge vault: %d %v", code, b)
	}
}
