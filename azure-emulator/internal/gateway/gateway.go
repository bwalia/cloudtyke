// Package gateway is the single HTTPS edge: host-based routing to the ARM,
// token-service and Graph front ends, the cloud metadata endpoint used by
// Terraform's metadata_host, request IDs and the recent-requests ring
// buffer exposed at /_cloudtyke/requests.
package gateway

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/auth"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/config"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
)

// Gateway routes requests by host name.
type Gateway struct {
	Cfg    config.Config
	ARM    http.Handler
	Login  http.Handler
	Graph  http.Handler
	Admin  http.Handler
	Log    *RequestLog
	Logger *slog.Logger
}

// ServeHTTP implements http.Handler.
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rid := entra.NewGUID()
	w.Header().Set("x-ms-request-id", rid)
	w.Header().Set("Server", "CloudTyke")
	rec := &recorder{ResponseWriter: w, status: 200}
	host := strings.ToLower(r.Host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	service := g.dispatch(rec, r, host)
	dur := time.Since(start)
	entry := Entry{
		Time: start.UTC(), RequestID: rid, Service: service, Method: r.Method, Host: host, Path: r.URL.Path,
		APIVersion: r.URL.Query().Get("api-version"), Status: rec.status, DurationMs: dur.Milliseconds(), Caller: callerOf(r),
	}
	if service != "admin" {
		g.Log.Add(entry)
	}
	lvl := slog.LevelInfo
	if rec.status >= 500 {
		lvl = slog.LevelError
	} else if rec.status >= 400 {
		lvl = slog.LevelWarn
	}
	if service != "admin" || rec.status >= 400 {
		g.Logger.Log(r.Context(), lvl, "request", "svc", service, "method", r.Method, "host", host, "path", r.URL.Path,
			"api_version", entry.APIVersion, "status", rec.status, "ms", entry.DurationMs, "caller", entry.Caller, "request_id", rid)
	}
}

func (g *Gateway) dispatch(w http.ResponseWriter, r *http.Request, host string) string {
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/_cloudtyke"):
		g.Admin.ServeHTTP(w, r)
		return "admin"
	case p == "/metadata/endpoints":
		g.metadata(w, r)
		return "metadata"
	}
	cfg := g.Cfg
	switch {
	case host == cfg.Host(config.SubLogin):
		g.Login.ServeHTTP(w, r)
		return "login"
	case host == cfg.Host(config.SubGraph):
		g.Graph.ServeHTTP(w, r)
		return "graph"
	case strings.HasSuffix(host, "."+cfg.Host(config.SubVault)),
		strings.HasSuffix(host, "."+cfg.Host(config.SubStorage)),
		strings.HasSuffix(host, "."+cfg.Host(config.SubACR)):
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(501)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "NotImplemented", "message": "This data plane (" + host + ") arrives in a later CloudTyke phase. See " + cfg.DocsURL}})
		return "dataplane"
	default:
		// management host, localhost and anything else -> ARM
		g.ARM.ServeHTTP(w, r)
		return "arm"
	}
}

// Metadata returns the cloud metadata document consumed by go-azure-sdk
// (Terraform azurerm / azuread / azapi metadata_host), az cloud register and SDKs.
func Metadata(cfg config.Config) map[string]any {
	d := cfg.Domain
	arm := cfg.URL(config.SubManagement) + "/"
	return map[string]any{
		"portal": cfg.URL("portal"),
		"authentication": map[string]any{
			"loginEndpoint":    cfg.URL(config.SubLogin),
			"audiences":        []string{arm, strings.TrimSuffix(arm, "/")},
			"tenant":           "common",
			"identityProvider": "AAD",
		},
		"media":         cfg.URL("media") + "/",
		"graphAudience": cfg.URL(config.SubGraph),
		"graph":         cfg.URL(config.SubGraph) + "/",
		"name":          "CloudTyke",
		"suffixes": map[string]any{
			"azureDataLakeStoreFileSystem":        "datalake." + d,
			"acrLoginServer":                      cfg.Host(config.SubACR),
			"sqlServerHostname":                   "database." + d,
			"azureDataLakeAnalyticsCatalogAndJob": "datalakeanalytics." + d,
			"keyVaultDns":                         cfg.Host(config.SubVault),
			"storage":                             cfg.Host(config.SubStorage),
			"azureFrontDoorEndpointSuffix":        "afd." + d,
			"storageSyncEndpointSuffix":           "afs." + d,
			"mhsmDns":                             "managedhsm." + d,
			"mysqlServerEndpoint":                 "mysql." + d,
			"postgresqlServerEndpoint":            "postgres." + d,
			"mariadbServerEndpoint":               "mariadb." + d,
			"synapseAnalytics":                    "synapse." + d,
			"attestationEndpoint":                 "attest." + d,
		},
		"batch":                                 cfg.URL("batch") + "/",
		"resourceManager":                       arm,
		"vmImageAliasDoc":                       cfg.URL(config.SubManagement) + "/_cloudtyke/vm-image-aliases.json",
		"activeDirectoryDataLake":               cfg.URL("datalake") + "/",
		"sqlManagement":                         cfg.URL("sqlmanagement") + "/",
		"microsoftGraphResourceId":              cfg.URL(config.SubGraph) + "/",
		"appInsightsResourceId":                 cfg.URL("appinsights"),
		"appInsightsTelemetryChannelResourceId": cfg.URL("dc.appinsights") + "/v2/track",
		"attestationResourceId":                 cfg.URL("attest"),
		"synapseAnalyticsResourceId":            cfg.URL("synapse"),
		"logAnalyticsResourceId":                cfg.URL("loganalytics"),
		"ossrDbmsResourceId":                    cfg.URL("ossrdbms"),
		"gallery":                               cfg.URL("gallery") + "/",
	}
}

func (g *Gateway) metadata(w http.ResponseWriter, r *http.Request) {
	md := Metadata(g.Cfg)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// api-version 2022-09-01 returns the current cloud as an object; older
	// versions (2019-05-01 / 2020-06-01) return an array of clouds.
	v := r.URL.Query().Get("api-version")
	if v != "" && v < "2022-09-01" {
		_ = json.NewEncoder(w).Encode([]any{md})
		return
	}
	_ = json.NewEncoder(w).Encode(md)
}

// callerOf extracts a display identity from the bearer token without
// verifying it (verification happens in the service handlers).
func callerOf(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) < 8 {
		return ""
	}
	claims, _, _, _, err := auth.Split(strings.TrimSpace(h[7:]))
	if err != nil {
		return ""
	}
	for _, k := range []string{"upn", "app_displayname", "appid", "oid"} {
		if s, _ := claims[k].(string); s != "" {
			return s
		}
	}
	return ""
}

type recorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *recorder) WriteHeader(code int) {
	if !r.wrote {
		r.status, r.wrote = code, true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	r.wrote = true
	return r.ResponseWriter.Write(b)
}

// Entry is one recorded API call.
type Entry struct {
	Time       time.Time `json:"time"`
	RequestID  string    `json:"requestId"`
	Service    string    `json:"service"`
	Method     string    `json:"method"`
	Host       string    `json:"host"`
	Path       string    `json:"path"`
	APIVersion string    `json:"apiVersion,omitempty"`
	Status     int       `json:"status"`
	DurationMs int64     `json:"durationMs"`
	Caller     string    `json:"caller,omitempty"`
}

// RequestLog is a fixed-size ring buffer of recent requests.
type RequestLog struct {
	mu   sync.Mutex
	buf  []Entry
	next int
	full bool
}

// NewRequestLog creates a ring buffer of size n.
func NewRequestLog(n int) *RequestLog { return &RequestLog{buf: make([]Entry, n)} }

// Add records an entry.
func (l *RequestLog) Add(e Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf[l.next] = e
	l.next = (l.next + 1) % len(l.buf)
	if l.next == 0 {
		l.full = true
	}
}

// Recent returns up to n newest entries, newest first.
func (l *RequestLog) Recent(n int) []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	size := l.next
	if l.full {
		size = len(l.buf)
	}
	if n <= 0 || n > size {
		n = size
	}
	out := make([]Entry, 0, n)
	for i := 1; i <= n; i++ {
		idx := (l.next - i + len(l.buf)) % len(l.buf)
		out = append(out, l.buf[idx])
	}
	return out
}
