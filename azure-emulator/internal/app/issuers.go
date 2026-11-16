package app

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/auth"
)

// loadTrustedIssuers registers the signing keys of OIDC issuers listed in
// CLOUDTYKE_TRUSTED_ISSUERS_FILE ({"<issuer>": {"keys": [JWK, ...]}}), so
// federated credentials can name issuers that aren't reachable for discovery.
func loadTrustedIssuers(path string, svc *auth.Service) error {
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("trusted issuers: %w", err)
	}
	var issuers map[string]struct {
		Keys []map[string]any `json:"keys"`
	}
	if err := json.Unmarshal(b, &issuers); err != nil {
		return fmt.Errorf("trusted issuers %s: %w", path, err)
	}
	for iss, jwks := range issuers {
		if len(jwks.Keys) == 0 {
			return fmt.Errorf("trusted issuer %s has no keys", iss)
		}
		key, err := auth.ParseJWK(jwks.Keys[0])
		if err != nil {
			return fmt.Errorf("trusted issuer %s: %w", iss, err)
		}
		svc.RegisterIssuer(iss, key)
	}
	return nil
}
