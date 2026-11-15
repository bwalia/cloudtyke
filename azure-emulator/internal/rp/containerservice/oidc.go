package containerservice

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

// JWKSProvider is implemented by backends that issue real service account
// tokens (k3s), so the OIDC issuer can publish their signing keys.
type JWKSProvider interface {
	JWKS(clusterID string) (json.RawMessage, bool)
}

// OIDCHandler serves each cluster's OIDC issuer on oidc.<domain>:
//
//	/{tenant}/{clusterUID}/.well-known/openid-configuration
//	/{tenant}/{clusterUID}/openid/v1/jwks
//
// Entra ID (the emulator's token service) uses these to validate workload
// identity service account tokens.
func (rp *RP) OIDCHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 3 {
			http.NotFound(w, r)
			return
		}
		c, ok := rp.clusterByUID(parts[0], parts[1])
		if !ok {
			http.NotFound(w, r)
			return
		}
		issuer := strOf(props(c)["oidcIssuerProfile"].(map[string]any)["issuerURL"])
		rest := strings.Join(parts[2:], "/")
		switch rest {
		case ".well-known/openid-configuration":
			writeJSON(w, map[string]any{"issuer": issuer, "jwks_uri": issuer + "openid/v1/jwks", "response_types_supported": []any{"id_token"},
				"subject_types_supported": []any{"public"}, "id_token_signing_alg_values_supported": []any{"RS256"}})
		case "openid/v1/jwks":
			if jp, ok := rp.backend.(JWKSProvider); ok {
				if raw, ok := jp.JWKS(strOf(c["id"])); ok {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write(raw)
					return
				}
			}
			writeJSON(w, map[string]any{"keys": []any{}})
		default:
			http.NotFound(w, r)
		}
	})
}

func (rp *RP) clusterByUID(tenant, uid string) (map[string]any, bool) {
	var found map[string]any
	needle := strings.ToLower("/" + tenant + "/" + uid + "/")
	_ = rp.s.Store.Scan(store.BucketARM, "/subscriptions/", func(k string, v []byte) bool {
		if !strings.Contains(k, "/managedclusters/") || !strings.Contains(strings.ToLower(string(v)), needle) {
			return true
		}
		var m map[string]any
		if json.Unmarshal(v, &m) == nil {
			if o, _ := props(m)["oidcIssuerProfile"].(map[string]any); o != nil && o["enabled"] == true {
				found = m
				return false
			}
		}
		return true
	})
	return found, found != nil
}

// JWKSForIssuer resolves an AKS issuer URL to its signing keys, for the
// token service's workload identity checks.
func (rp *RP) JWKSForIssuer(iss string) ([]map[string]any, bool) {
	prefix := strings.TrimSuffix(rp.s.Cfg.URL("oidc"), "/") + "/"
	if !strings.HasPrefix(iss, prefix) {
		return nil, false
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(iss, prefix), "/"), "/")
	if len(parts) < 2 {
		return nil, false
	}
	c, ok := rp.clusterByUID(parts[0], parts[1])
	if !ok {
		return nil, false
	}
	jp, ok := rp.backend.(JWKSProvider)
	if !ok {
		return []map[string]any{}, true
	}
	raw, ok := jp.JWKS(strOf(c["id"]))
	if !ok {
		return []map[string]any{}, true
	}
	var doc struct {
		Keys []map[string]any `json:"keys"`
	}
	_ = json.Unmarshal(raw, &doc)
	return doc.Keys, true
}
