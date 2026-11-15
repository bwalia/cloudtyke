package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/config"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/gateway"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

func TestDataPlaneExists(t *testing.T) {
	cfg := config.Config{Domain: "cloudtyke.dev"}
	st := store.NewMemory()
	_ = st.Put(store.BucketARM, "/subscriptions/s/resourcegroups/rg/providers/microsoft.keyvault/vaults/kv-hello", []byte("{}"))
	_ = st.Put(store.BucketARM, "/subscriptions/s/resourcegroups/rg/providers/microsoft.storage/storageaccounts/sthello", []byte("{}"))
	exists := dataPlaneExists(cfg, st)
	for host, want := range map[string]bool{
		"kv-hello.vault.cloudtyke.dev":          true,
		"KV-Hello.vault.cloudtyke.dev":          true,
		"kv-other.vault.cloudtyke.dev":          false,
		"sthello.blob.storage.cloudtyke.dev":    true,
		"sthello.file.storage.cloudtyke.dev":    true,
		"sthello.vault.cloudtyke.dev":           false,
		"a.kv-hello.vault.cloudtyke.dev":        false,
		"vault.cloudtyke.dev":                   false,
		"kv-hello.vault.cloudtyke.local":        false,
		"sthello.blob.storage.cloudtyke.dev.ev": false,
	} {
		if got := exists(host); got != want {
			t.Errorf("exists(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestEdgeAllowAndForwardedHost(t *testing.T) {
	cfg := config.Config{Domain: "cloudtyke.dev", TrustForwardedHost: true}
	st := store.NewMemory()
	_ = st.Put(store.BucketARM, "/subscriptions/s/resourcegroups/rg/providers/microsoft.keyvault/vaults/kv-hello", []byte("{}"))
	seen := ""
	vault := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen = r.Host })
	g := &gateway.Gateway{
		Cfg: cfg, ARM: http.NotFoundHandler(), Login: http.NotFoundHandler(), Graph: http.NotFoundHandler(),
		Admin: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }),
		DataPlanes: map[string]http.Handler{".vault.cloudtyke.dev": vault},
		Exists:     dataPlaneExists(cfg, st),
		Log:        gateway.NewRequestLog(10), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	for domain, want := range map[string]int{
		"management.cloudtyke.dev":     200,
		"login.cloudtyke.dev":          200,
		"kv-hello.vault.cloudtyke.dev": 200,
		"kv-nope.vault.cloudtyke.dev":  404,
		"evil.example.com":             404,
		"":                             404,
	} {
		rec := httptest.NewRecorder()
		g.ServeHTTP(rec, httptest.NewRequest("GET", "http://management.cloudtyke.dev/_cloudtyke/edge/allow?domain="+domain, nil))
		if rec.Code != want {
			t.Errorf("allow %q = %d, want %d", domain, rec.Code, want)
		}
	}
	// The edge sends its own server name as Host and the client's in X-Original-Host.
	req := httptest.NewRequest("GET", "http://data.cloudtyke.dev/secrets/greeting", nil)
	req.Header.Set("X-Original-Host", "kv-hello.vault.cloudtyke.dev")
	g.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "kv-hello.vault.cloudtyke.dev" {
		t.Fatalf("vault data plane saw host %q", seen)
	}
}
