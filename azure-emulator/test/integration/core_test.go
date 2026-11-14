// Package integration exercises the whole emulator in-process over HTTPS:
// token service, ARM (resource groups, RBAC, async delete), Graph and admin.
package integration

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/app"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/config"
)

const (
	tenant    = "7c10d7e4-0000-4000-8000-000000000001"
	sandbox   = "5b5c0000-0000-4000-8000-000000000003"
	bootID    = "a99c0000-0000-4000-8000-000000000001"
	bootSec   = "cloudtyke-bootstrap-secret"
	readerID  = "a99c0000-0000-4000-8000-000000000002"
	readerSec = "cloudtyke-reader-secret"
)

type env struct {
	t      *testing.T
	cfg    config.Config
	client *http.Client
}

func start(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{Domain: "cloudtyke.local", DataDir: dir, CertExportDir: dir + "/export", Ephemeral: true, PageSize: 100,
		AsyncDelay: 300 * time.Millisecond, DocsURL: "docs"}
	a, err := app.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(a.Handler)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{a.PKI.ServerTLS}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	pool := x509.NewCertPool()
	pool.AddCert(a.PKI.CACert)
	addr := srv.Listener.Addr().String()
	dialer := &net.Dialer{}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool},
		// resolve every *.cloudtyke.local name to the test server
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
	}
	return &env{t: t, cfg: cfg, client: &http.Client{Transport: tr, Timeout: 10 * time.Second}}
}

func (e *env) url(sub, path string) string { return "https://" + e.cfg.Host(sub) + path }

func (e *env) token(clientID, secret, resource string) string {
	e.t.Helper()
	form := url.Values{"client_id": {clientID}, "client_secret": {secret}, "grant_type": {"client_credentials"}, "scope": {resource + "/.default"}}
	resp, err := e.client.PostForm(e.url("login", "/"+tenant+"/oauth2/v2.0/token"), form)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != 200 {
		e.t.Fatalf("token: %d %v", resp.StatusCode, out)
	}
	return out["access_token"].(string)
}

func (e *env) do(method, u, tok string, body any) (int, http.Header, map[string]any) {
	e.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, u, rdr)
	req.Header.Set("Content-Type", "application/json")
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, resp.Header, out
}

func errCode(m map[string]any) string {
	e, _ := m["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func TestMetadata(t *testing.T) {
	e := start(t)
	code, _, md := e.do("GET", e.url("management", "/metadata/endpoints?api-version=2022-09-01"), "", nil)
	if code != 200 || md["name"] != "CloudTyke" || !strings.HasPrefix(md["resourceManager"].(string), "https://management.cloudtyke.local") {
		t.Fatalf("metadata: %d %v", code, md)
	}
	if md["microsoftGraphResourceId"] == "" {
		t.Fatal("microsoftGraphResourceId required by go-azure-sdk")
	}
}

func TestResourceGroupLifecycleAndRBAC(t *testing.T) {
	e := start(t)
	arm := e.url("management", "")
	tok := e.token(bootID, bootSec, arm+"/")
	rg := arm + "/subscriptions/" + sandbox + "/resourcegroups/rg-int?api-version=2023-07-01"

	if code, _, b := e.do("GET", rg, tok, nil); code != 404 || errCode(b) != "ResourceGroupNotFound" {
		t.Fatalf("expected ResourceGroupNotFound, got %d %v", code, b)
	}
	if code, _, b := e.do("GET", rg[:strings.Index(rg, "?")], tok, nil); code != 400 || errCode(b) != "MissingApiVersionParameter" {
		t.Fatalf("expected MissingApiVersionParameter, got %d %v", code, b)
	}
	if code, _, b := e.do("PUT", rg, tok, map[string]any{"location": "Mars Central"}); code != 400 || errCode(b) != "LocationNotAvailableForResourceGroup" {
		t.Fatalf("expected location error, got %d %v", code, b)
	}
	code, _, b := e.do("PUT", rg, tok, map[string]any{"location": "UK South", "tags": map[string]any{"env": "test"}})
	if code != 201 || b["location"] != "uksouth" || b["id"] != "/subscriptions/"+sandbox+"/resourceGroups/rg-int" {
		t.Fatalf("create: %d %v", code, b)
	}
	if code, _, _ := e.do("PUT", rg, tok, map[string]any{"location": "uksouth"}); code != 200 {
		t.Fatalf("idempotent PUT should be 200, got %d", code)
	}

	// reader cannot write, can read
	rtok := e.token(readerID, readerSec, arm+"/")
	if code, _, b := e.do("PUT", rg, rtok, map[string]any{"location": "uksouth"}); code != 403 || errCode(b) != "AuthorizationFailed" {
		t.Fatalf("reader write: expected 403 AuthorizationFailed, got %d %v", code, b)
	}
	if code, _, _ := e.do("GET", rg, rtok, nil); code != 200 {
		t.Fatalf("reader read: %d", code)
	}
	// reader only sees the sandbox subscription
	_, _, subs := e.do("GET", arm+"/subscriptions?api-version=2022-12-01", rtok, nil)
	if n := len(subs["value"].([]any)); n != 1 {
		t.Fatalf("reader should see 1 subscription, saw %d", n)
	}

	// role assignment: grant reader Contributor on the RG, then the write succeeds
	ra := arm + "/subscriptions/" + sandbox + "/resourceGroups/rg-int/providers/Microsoft.Authorization/roleAssignments/11111111-2222-4333-8444-555555555555?api-version=2022-04-01"
	code, _, b = e.do("PUT", ra, tok, map[string]any{"properties": map[string]any{
		"roleDefinitionId": "/subscriptions/" + sandbox + "/providers/Microsoft.Authorization/roleDefinitions/b24988ac-6180-42a0-ab88-20f7382dd24c",
		"principalId":      "0b100000-0000-4000-8000-000000000002",
	}})
	if code != 201 || b["properties"].(map[string]any)["principalType"] != "ServicePrincipal" {
		t.Fatalf("role assignment: %d %v", code, b)
	}
	if code, _, b := e.do("PATCH", rg, rtok, map[string]any{"tags": map[string]any{"owner": "reader"}}); code != 200 {
		t.Fatalf("reader with contributor should patch: %d %v", code, b)
	}
	// unknown principal
	code, _, b = e.do("PUT", strings.Replace(ra, "555555555555", "555555555556", 1), tok, map[string]any{"properties": map[string]any{
		"roleDefinitionId": "/providers/Microsoft.Authorization/roleDefinitions/acdd72a7-3385-48ef-bd42-f606fba81ae7",
		"principalId":      "00000000-0000-4000-8000-00000000dead",
	}})
	if code != 400 || errCode(b) != "PrincipalNotFound" {
		t.Fatalf("expected PrincipalNotFound, got %d %v", code, b)
	}

	// async delete with Location polling
	code, hdr, _ := e.do("DELETE", rg, tok, nil)
	if code != 202 || hdr.Get("Location") == "" || hdr.Get("Retry-After") == "" {
		t.Fatalf("delete: %d %v", code, hdr)
	}
	loc := hdr.Get("Location")
	if code, _, _ := e.do("GET", loc, tok, nil); code != 202 {
		t.Fatalf("operation should still be in progress, got %d", code)
	}
	time.Sleep(350 * time.Millisecond)
	if code, _, _ := e.do("GET", loc, tok, nil); code != 200 {
		t.Fatalf("operation should be done, got %d", code)
	}
	if code, _, _ := e.do("GET", rg, tok, nil); code != 404 {
		t.Fatalf("rg should be gone, got %d", code)
	}
	if code, _, _ := e.do("GET", ra, tok, nil); code != 404 {
		t.Fatalf("role assignment should be cascade-deleted, got %d", code)
	}

	// activity log recorded the writes
	_, _, al := e.do("GET", arm+"/subscriptions/"+sandbox+"/providers/Microsoft.Insights/eventtypes/management/values?api-version=2015-04-01&$filter="+url.QueryEscape("resourceGroupName eq 'rg-int'"), tok, nil)
	if n := len(al["value"].([]any)); n < 4 {
		t.Fatalf("expected activity log events, got %d", n)
	}

	// unknown type -> 501
	if code, _, b := e.do("GET", arm+"/subscriptions/"+sandbox+"/providers/Microsoft.Web/sites?api-version=2023-01-01", tok, nil); code != 501 || errCode(b) != "NotImplemented" {
		t.Fatalf("expected 501, got %d %v", code, b)
	}
}

func TestTokenErrors(t *testing.T) {
	e := start(t)
	form := url.Values{"client_id": {bootID}, "client_secret": {"wrong"}, "grant_type": {"client_credentials"}, "scope": {"https://management.cloudtyke.local/.default"}}
	resp, err := e.client.PostForm(e.url("login", "/"+tenant+"/oauth2/v2.0/token"), form)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("bad secret should be 401, got %d", resp.StatusCode)
	}
	// graph token used against ARM -> audience error
	gtok := e.token(bootID, bootSec, e.url("graph", ""))
	code, _, b := e.do("GET", e.url("management", "/subscriptions?api-version=2022-12-01"), gtok, nil)
	if code != 401 || errCode(b) != "InvalidAuthenticationTokenAudience" {
		t.Fatalf("expected audience error, got %d %v", code, b)
	}
}

func TestGraphUsersGroupsApps(t *testing.T) {
	e := start(t)
	g := e.url("graph", "/v1.0")
	tok := e.token(bootID, bootSec, e.url("graph", ""))

	code, _, u := e.do("POST", g+"/users", tok, map[string]any{
		"accountEnabled": true, "displayName": "Grace", "mailNickname": "grace", "userPrincipalName": "grace@cloudtyke.local",
		"passwordProfile": map[string]any{"password": "Sup3r-secret!"},
	})
	if code != 201 || u["passwordProfile"] != nil {
		t.Fatalf("create user: %d %v", code, u)
	}
	if code, _, b := e.do("POST", g+"/users", tok, map[string]any{
		"accountEnabled": true, "displayName": "X", "mailNickname": "x", "userPrincipalName": "x@example.com",
		"passwordProfile": map[string]any{"password": "Sup3r-secret!"},
	}); code != 400 {
		t.Fatalf("unverified domain should fail: %d %v", code, b)
	}
	code, _, grp := e.do("POST", g+"/groups", tok, map[string]any{"displayName": "eng", "mailEnabled": false, "mailNickname": "eng", "securityEnabled": true,
		"members@odata.bind": []string{g + "/directoryObjects/" + u["id"].(string)}})
	if code != 201 {
		t.Fatalf("create group: %d %v", code, grp)
	}
	_, _, members := e.do("GET", g+"/groups/"+grp["id"].(string)+"/members", tok, nil)
	if len(members["value"].([]any)) != 1 {
		t.Fatalf("expected 1 member: %v", members)
	}
	_, _, filtered := e.do("GET", g+"/groups?$filter="+url.QueryEscape("displayName eq 'eng'")+"&$select=id,displayName", tok, nil)
	if len(filtered["value"].([]any)) != 1 {
		t.Fatalf("filter: %v", filtered)
	}

	code, _, appObj := e.do("POST", g+"/applications", tok, map[string]any{"displayName": "deployer"})
	if code != 201 || appObj["appId"] == "" {
		t.Fatalf("create app: %d %v", code, appObj)
	}
	appID := appObj["appId"].(string)
	if code, _, sp := e.do("POST", g+"/servicePrincipals", tok, map[string]any{"appId": appID}); code != 201 {
		t.Fatalf("create sp: %d %v", code, sp)
	}
	code, _, pw := e.do("POST", g+"/applications/"+appObj["id"].(string)+"/addPassword", tok, map[string]any{"passwordCredential": map[string]any{"displayName": "ci"}})
	if code != 200 || pw["secretText"] == "" {
		t.Fatalf("addPassword: %d %v", code, pw)
	}
	// the new credential authenticates
	_ = e.token(appID, pw["secretText"].(string), e.url("management", "")+"/")

	// reader app cannot write to Graph
	rtok := e.token(readerID, readerSec, e.url("graph", ""))
	if code, _, b := e.do("POST", g+"/groups", rtok, map[string]any{"displayName": "x", "mailEnabled": false, "mailNickname": "x", "securityEnabled": true}); code != 403 || errCode(b) != "Authorization_RequestDenied" {
		t.Fatalf("reader graph write: %d %v", code, b)
	}
	// alternate key + manager 404
	if code, _, _ := e.do("GET", g+"/applications(appId='"+appID+"')", tok, nil); code != 200 {
		t.Fatalf("alternate key lookup: %d", code)
	}
	if code, _, b := e.do("GET", g+"/users/"+u["id"].(string)+"/manager", tok, nil); code != 404 || errCode(b) != "Request_ResourceNotFound" {
		t.Fatalf("manager: %d %v", code, b)
	}
}
