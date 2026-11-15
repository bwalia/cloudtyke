package integration

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestStorage(t *testing.T) {
	e := start(t)
	arm := e.url("management", "")
	tok := e.token(bootID, bootSec, arm+"/")
	stTok := e.token(bootID, bootSec, "https://storage.cloudtyke.local")
	sub := "/subscriptions/" + sandbox
	rg := sub + "/resourceGroups/rg-st"
	const api = "?api-version=2023-05-01"
	e.do("PUT", arm+rg+"?api-version=2024-11-01", tok, map[string]any{"location": "uksouth"})
	acct := rg + "/providers/Microsoft.Storage/storageAccounts/stpayreceipts"
	if code, _, b := e.do("PUT", arm+rg+"/providers/Microsoft.Storage/storageAccounts/Bad_Name"+api, tok, map[string]any{"location": "uksouth", "kind": "StorageV2", "sku": map[string]any{"name": "Standard_LRS"}}); code != 400 || errCode(b) != "AccountNameInvalid" {
		t.Fatalf("bad name: %d %v", code, b)
	}
	code, _, b := e.do("PUT", arm+acct+api, tok, map[string]any{"location": "uksouth", "kind": "StorageV2", "sku": map[string]any{"name": "Standard_ZRS"}})
	if code >= 300 || !strings.HasPrefix(b["properties"].(map[string]any)["primaryEndpoints"].(map[string]any)["blob"].(string), "https://stpayreceipts.blob.storage.cloudtyke.local") {
		t.Fatalf("account: %d %v", code, b)
	}
	if code, _, b := e.do("PUT", arm+acct+"/blobServices/default/containers/public"+api, tok, map[string]any{"properties": map[string]any{"publicAccess": "Blob"}}); code != 409 || errCode(b) != "PublicAccessNotPermitted" {
		t.Fatalf("public container: %d %v", code, b)
	}
	e.do("PUT", arm+acct+"/blobServices/default/containers/receipts"+api, tok, map[string]any{"properties": map[string]any{}})
	e.do("PUT", arm+acct+"/fileServices/default/shares/settlement"+api, tok, map[string]any{"properties": map[string]any{"shareQuota": 100}})

	raw := func(method, url, tk string, body []byte, hdr map[string]string) (int, string) {
		req, _ := http.NewRequest(method, url, bytes.NewReader(body))
		if tk != "" {
			req.Header.Set("Authorization", "Bearer "+tk)
		}
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := e.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(out)
	}
	blob := "https://stpayreceipts.blob.storage.cloudtyke.local/receipts/2026/11/pay_123.json"
	// Owner has no data role: 403, like Azure
	if code, body := raw("PUT", blob, stTok, []byte(`{"id":"pay_123"}`), map[string]string{"x-ms-blob-type": "BlockBlob"}); code != 403 || !strings.Contains(body, "AuthorizationPermissionMismatch") {
		t.Fatalf("no data role: %d %s", code, body)
	}
	e.do("PUT", arm+acct+"/providers/Microsoft.Authorization/roleAssignments/3c6c6a7e-0000-4000-8000-000000000001?api-version=2022-04-01", tok, map[string]any{"properties": map[string]any{
		"roleDefinitionId": sub + "/providers/Microsoft.Authorization/roleDefinitions/ba92f5b4-2d11-453d-a403-e96b0029c9fe", "principalId": "0b100000-0000-4000-8000-000000000001", "principalType": "ServicePrincipal"}})
	if code, body := raw("PUT", blob, stTok, []byte(`{"id":"pay_123"}`), map[string]string{"x-ms-blob-type": "BlockBlob", "Content-Type": "application/json"}); code != 201 {
		t.Fatalf("put blob: %d %s", code, body)
	}
	if code, body := raw("GET", blob, stTok, nil, nil); code != 200 || body != `{"id":"pay_123"}` {
		t.Fatalf("get blob: %d %s", code, body)
	}
	if code, body := raw("GET", "https://stpayreceipts.blob.storage.cloudtyke.local/receipts?restype=container&comp=list&prefix=2026/", stTok, nil, nil); code != 200 || !strings.Contains(body, "pay_123.json") {
		t.Fatalf("list: %d %s", code, body)
	}
	if code, _ := raw("GET", blob, "", nil, nil); code != 401 {
		t.Fatalf("anonymous: %d", code)
	}

	// Azure Files: create directory, file, write range, read back
	e.do("PUT", arm+acct+"/providers/Microsoft.Authorization/roleAssignments/3c6c6a7e-0000-4000-8000-000000000002?api-version=2022-04-01", tok, map[string]any{"properties": map[string]any{
		"roleDefinitionId": sub + "/providers/Microsoft.Authorization/roleDefinitions/69566ab7-960f-475b-8e7c-b3118f30c6bd", "principalId": "0b100000-0000-4000-8000-000000000001", "principalType": "ServicePrincipal"}})
	files := "https://stpayreceipts.file.storage.cloudtyke.local/settlement"
	content := []byte("merchant,amount\nnorthwind,1200.50\n")
	if code, body := raw("PUT", files+"/2026-11-15?restype=directory", stTok, nil, map[string]string{"x-ms-file-request-intent": "backup"}); code != 201 {
		t.Fatalf("mkdir: %d %s", code, body)
	}
	raw("PUT", files+"/2026-11-15/batch-001.csv", stTok, nil, map[string]string{"x-ms-type": "file", "x-ms-content-length": "34"})
	if code, body := raw("PUT", files+"/2026-11-15/batch-001.csv?comp=range", stTok, content, map[string]string{"x-ms-range": "bytes=0-33", "x-ms-write": "update"}); code != 201 {
		t.Fatalf("put range: %d %s", code, body)
	}
	if code, body := raw("GET", files+"/2026-11-15/batch-001.csv", stTok, nil, nil); code != 200 || body != string(content) {
		t.Fatalf("read file: %d %q", code, body)
	}
	if code, body := raw("GET", files+"/2026-11-15?restype=directory&comp=list", stTok, nil, nil); code != 200 || !strings.Contains(body, "batch-001.csv") {
		t.Fatalf("list dir: %d %s", code, body)
	}
}
