// Package admin serves the emulator's own API under /_cloudtyke/: health,
// version, coverage, recent requests, environment helper, state export /
// import, named snapshots ("time travel"), reset/re-seed and chaos.
package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/arm"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/chaos"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/config"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/gateway"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/seed"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/version"
)

// Server is the admin API.
type Server struct {
	Cfg      config.Config
	Store    store.Store
	Registry *arm.Registry
	Log      *gateway.RequestLog
	Chaos    *chaos.Engine
	CAPEM    []byte
	Seed     seed.File
	// WriteMu is the ARM write lock, held while state is swapped.
	WriteMu *sync.Mutex

	memSnaps sync.Map
}

var snapName = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if tok := os.Getenv("CLOUDTYKE_ADMIN_TOKEN"); tok != "" && r.Header.Get("X-CloudTyke-Admin-Token") != tok && !strings.HasSuffix(r.URL.Path, "/health") {
		writeJSON(w, 401, map[string]any{"error": "missing or invalid X-CloudTyke-Admin-Token"})
		return
	}
	p := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/_cloudtyke"), "/")
	switch {
	case p == "" || p == "/health":
		writeJSON(w, 200, map[string]any{"status": "ok", "version": version.Version})
	case p == "/version":
		writeJSON(w, 200, map[string]any{"version": version.Version, "commit": version.Commit, "date": version.Date})
	case p == "/ca.pem":
		w.Header().Set("Content-Type", "application/x-pem-file")
		_, _ = w.Write(s.CAPEM)
	case p == "/coverage":
		if r.URL.Query().Get("format") == "markdown" {
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_, _ = w.Write([]byte(CoverageMarkdown(s.Registry)))
			return
		}
		writeJSON(w, 200, coverageJSON(s.Registry))
	case p == "/requests":
		n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if n == 0 {
			n = 100
		}
		writeJSON(w, 200, map[string]any{"value": s.Log.Recent(n)})
	case p == "/env":
		writeJSON(w, 200, s.env())
	case p == "/metadata":
		writeJSON(w, 200, gateway.Metadata(s.Cfg))
	case p == "/chaos":
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, 200, s.Chaos.Get())
		case http.MethodPut, http.MethodPost:
			var c chaos.Config
			if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
				writeJSON(w, 400, map[string]any{"error": err.Error()})
				return
			}
			s.Chaos.Set(c)
			writeJSON(w, 200, c)
		case http.MethodDelete:
			s.Chaos.Set(chaos.Config{})
			writeJSON(w, 200, chaos.Config{})
		}
	case p == "/state":
		switch r.Method {
		case http.MethodGet:
			snap, err := s.Store.Dump()
			if err != nil {
				writeJSON(w, 500, map[string]any{"error": err.Error()})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Disposition", `attachment; filename="cloudtyke-state.json"`)
			_ = store.WriteSnapshot(w, snap)
		case http.MethodPut, http.MethodPost:
			snap, err := store.ReadSnapshot(r.Body)
			if err != nil {
				writeJSON(w, 400, map[string]any{"error": err.Error()})
				return
			}
			s.load(snap)
			writeJSON(w, 200, map[string]any{"status": "imported"})
		}
	case p == "/reset" && r.Method == http.MethodPost:
		s.WriteMu.Lock()
		err := s.Store.Load(store.Snapshot{})
		if err == nil {
			err = seed.Apply(s.Store, s.Seed)
		}
		s.WriteMu.Unlock()
		if err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"status": "reset"})
	case p == "/seed" && r.Method == http.MethodPost:
		s.WriteMu.Lock()
		err := seed.Apply(s.Store, s.Seed)
		s.WriteMu.Unlock()
		if err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"status": "seeded"})
	case p == "/snapshots":
		writeJSON(w, 200, map[string]any{"value": s.listSnapshots()})
	case strings.HasPrefix(p, "/snapshots/"):
		s.snapshot(w, r, strings.TrimPrefix(p, "/snapshots/"))
	default:
		writeJSON(w, 404, map[string]any{"error": "unknown admin endpoint " + r.URL.Path})
	}
}

func (s *Server) load(snap store.Snapshot) {
	s.WriteMu.Lock()
	defer s.WriteMu.Unlock()
	_ = s.Store.Load(snap)
}

func (s *Server) snapDir() string { return filepath.Join(s.Cfg.DataDir, "snapshots") }

func (s *Server) listSnapshots() []string {
	var out []string
	if s.Cfg.Ephemeral {
		s.memSnaps.Range(func(k, _ any) bool { out = append(out, k.(string)); return true })
	} else {
		entries, _ := os.ReadDir(s.snapDir())
		for _, e := range entries {
			out = append(out, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	sort.Strings(out)
	return out
}

// snapshot: POST /snapshots/{name} saves, POST /snapshots/{name}/restore restores, DELETE removes.
func (s *Server) snapshot(w http.ResponseWriter, r *http.Request, rest string) {
	name, action, _ := strings.Cut(rest, "/")
	if !snapName.MatchString(name) {
		writeJSON(w, 400, map[string]any{"error": "invalid snapshot name"})
		return
	}
	path := filepath.Join(s.snapDir(), name+".json")
	switch {
	case action == "" && r.Method == http.MethodPost:
		snap, err := s.Store.Dump()
		if err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		var buf bytes.Buffer
		_ = store.WriteSnapshot(&buf, snap)
		if s.Cfg.Ephemeral {
			s.memSnaps.Store(name, buf.Bytes())
		} else {
			_ = os.MkdirAll(s.snapDir(), 0o700)
			if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
				writeJSON(w, 500, map[string]any{"error": err.Error()})
				return
			}
		}
		writeJSON(w, 201, map[string]any{"name": name, "status": "saved"})
	case action == "restore" && r.Method == http.MethodPost:
		var raw []byte
		if s.Cfg.Ephemeral {
			v, ok := s.memSnaps.Load(name)
			if !ok {
				writeJSON(w, 404, map[string]any{"error": "snapshot not found"})
				return
			}
			raw = v.([]byte)
		} else {
			b, err := os.ReadFile(path)
			if err != nil {
				writeJSON(w, 404, map[string]any{"error": "snapshot not found"})
				return
			}
			raw = b
		}
		snap, err := store.ReadSnapshot(bytes.NewReader(raw))
		if err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		s.load(snap)
		writeJSON(w, 200, map[string]any{"name": name, "status": "restored"})
	case action == "" && r.Method == http.MethodDelete:
		s.memSnaps.Delete(name)
		_ = os.Remove(path)
		w.WriteHeader(204)
	default:
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
	}
}

// env returns ARM_* / AZURE_* settings for the first seeded service principal.
func (s *Server) env() map[string]any {
	out := map[string]any{}
	if len(s.Seed.Tenants) == 0 {
		return out
	}
	t := s.Seed.Tenants[0]
	sub := ""
	for _, sb := range t.Subscriptions {
		if sub == "" || strings.Contains(sb.DisplayName, "sandbox") {
			sub = sb.ID
		}
	}
	mgmt := s.Cfg.Host(config.SubManagement)
	if s.Cfg.PublicPort != 443 {
		mgmt = fmt.Sprintf("%s:%d", mgmt, s.Cfg.PublicPort)
	}
	vars := map[string]string{
		"ARM_METADATA_HOSTNAME": mgmt,
		"ARM_TENANT_ID":         t.ID,
		"ARM_SUBSCRIPTION_ID":   sub,
		"AZURE_TENANT_ID":       t.ID,
		"AZURE_SUBSCRIPTION_ID": sub,
		"AZURE_AUTHORITY_HOST":  s.Cfg.URL(config.SubLogin),
	}
	if len(t.SPs) > 0 {
		vars["ARM_CLIENT_ID"] = t.SPs[0].AppID
		vars["ARM_CLIENT_SECRET"] = t.SPs[0].ClientSecret
		vars["AZURE_CLIENT_ID"] = t.SPs[0].AppID
		vars["AZURE_CLIENT_SECRET"] = t.SPs[0].ClientSecret
	}
	out["env"] = vars
	out["note"] = "Local emulator credentials only. Trust certs/ca.pem (SSL_CERT_FILE / REQUESTS_CA_BUNDLE / NODE_EXTRA_CA_CERTS or the OS trust store)."
	return out
}

func coverageJSON(reg *arm.Registry) map[string]any {
	var items []map[string]any
	for _, t := range reg.All() {
		items = append(items, map[string]any{"type": t.FullType(), "apiVersions": t.APIVersions, "phase": t.Phase, "notes": t.Notes})
	}
	return map[string]any{"value": items}
}

// CoverageMarkdown renders docs/coverage.md from the registry.
func CoverageMarkdown(reg *arm.Registry) string {
	var b strings.Builder
	b.WriteString("# CloudTyke Azure emulator: coverage matrix\n\n")
	b.WriteString("<!-- Generated by `make coverage` from the registered resource providers. Do not edit by hand. -->\n\n")
	b.WriteString("Anything not listed here returns `501 NotImplemented` with the resource type and api-version.\n")
	b.WriteString("With `CLOUDTYKE_STRICT_API_VERSIONS=true` only the api-versions listed are accepted.\n\n")
	b.WriteString("## Azure Resource Manager\n\n| Resource type | API versions | Phase | Notes |\n|---|---|---|---|\n")
	for _, t := range reg.All() {
		fmt.Fprintf(&b, "| `%s` | %s | %d | %s |\n", t.FullType(), strings.Join(t.APIVersions, ", "), t.Phase, t.Notes)
	}
	b.WriteString(graphCoverage)
	return b.String()
}

const graphCoverage = `
## Entra ID token service (` + "`login.<domain>`" + `)

| Endpoint | Notes |
|---|---|
| ` + "`/{tenant}/oauth2/v2.0/token`, `/{tenant}/oauth2/token`" + ` | client_credentials with client secret or federated client assertion (workload identity); RS256 JWTs with tid/oid/appid/roles/groups |
| ` + "`/{tenant}/v2.0/.well-known/openid-configuration`, `/{tenant}/discovery/v2.0/keys`" + ` | OIDC discovery + JWKS |
| ` + "`/common/discovery/instance`" + ` | MSAL instance discovery |

## Microsoft Graph subset (` + "`graph.<domain>/v1.0` and `/beta`" + `)

| Resource | Operations |
|---|---|
| users | list/get/create/update/delete (soft delete), lookup by UPN, memberOf/transitiveMemberOf, password complexity, verified-domain check |
| groups | list/get/create/update/delete, owners & members (` + "`$ref`" + `, ` + "`@odata.bind`" + `), transitiveMembers |
| applications | list/get/create/update/delete, ` + "`applications(appId='…')`" + `, owners, addPassword/removePassword, federatedIdentityCredentials |
| servicePrincipals | list/get/create/update/delete, ` + "`servicePrincipals(appId='…')`" + `, owners, addPassword, appRoleAssignedTo, appRoleAssignments |
| oauth2PermissionGrants | list/get/create/update/delete |
| directoryRoles, directoryRoleTemplates | list, activate, members |
| directoryObjects | get, getByIds |
| directory/deletedItems | list by type, get, restore, permanent delete |
| organization, domains | read |

OData: ` + "`$filter`" + ` (eq, ne, startswith, in, any(), and/or/not), ` + "`$select`, `$top`, `$count`, `$orderby`, `$search`" + ` (simple), paging via ` + "`@odata.nextLink`" + `.
Application permissions (app roles on the seeded *Microsoft Graph* service principal) are enforced and returned as ` + "`Authorization_RequestDenied`" + ` when missing.
`

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
