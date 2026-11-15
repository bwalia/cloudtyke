package app

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/auth"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/config"
)

func TestLoadTrustedIssuers(t *testing.T) {
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwk := map[string]any{"kty": "RSA", "n": base64.RawURLEncoding.EncodeToString(k.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.E)).Bytes())}
	b, _ := json.Marshal(map[string]any{"https://kubernetes.default.svc.cluster.local": map[string]any{"keys": []any{jwk}}})
	p := filepath.Join(t.TempDir(), "issuers.json")
	_ = os.WriteFile(p, b, 0o600)
	svc := auth.NewService(config.Config{Domain: "cloudtyke.dev"}, nil, nil)
	if err := loadTrustedIssuers(p, svc); err != nil {
		t.Fatal(err)
	}
	if err := loadTrustedIssuers(filepath.Join(t.TempDir(), "missing.json"), svc); err == nil {
		t.Fatal("expected an error for a missing file")
	}
	_ = os.WriteFile(p, []byte(`{"https://x": {"keys": []}}`), 0o600)
	if err := loadTrustedIssuers(p, svc); err == nil {
		t.Fatal("expected an error for an issuer without keys")
	}
}
