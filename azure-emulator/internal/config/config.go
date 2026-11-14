// Package config holds runtime configuration, read from CLOUDTYKE_* env vars.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the emulator runtime configuration.
type Config struct {
	Domain            string        // base DNS domain, default cloudtyke.local
	Listen            string        // HTTPS listen address inside the container
	PublicPort        int           // port clients use (appears in endpoint URLs)
	DataDir           string        // persistent state (db, keys, certs)
	CertExportDir     string        // where ca.pem is exported for users
	Ephemeral         bool          // in-memory store, nothing persisted
	SeedFile          string        // optional seed.yaml override
	StrictAPIVersions bool          // reject api-versions not in the type's list
	AsyncDelay        time.Duration // how long async operations stay InProgress
	PageSize          int           // max items per list page before nextLink
	ExtraHosts        []string      // extra SANs for the server certificate
	DocsURL           string        // coverage matrix link in 501 errors
}

// FromEnv reads the configuration from the environment.
func FromEnv() Config {
	c := Config{
		Domain:            env("CLOUDTYKE_DOMAIN", "cloudtyke.local"),
		Listen:            env("CLOUDTYKE_LISTEN", ":443"),
		PublicPort:        envInt("CLOUDTYKE_PUBLIC_PORT", 443),
		DataDir:           env("CLOUDTYKE_DATA_DIR", "/data"),
		CertExportDir:     env("CLOUDTYKE_CERT_EXPORT_DIR", "/certs"),
		Ephemeral:         envBool("CLOUDTYKE_EPHEMERAL", false),
		SeedFile:          env("CLOUDTYKE_SEED_FILE", ""),
		StrictAPIVersions: envBool("CLOUDTYKE_STRICT_API_VERSIONS", false),
		AsyncDelay:        envDuration("CLOUDTYKE_ASYNC_DELAY", 0),
		PageSize:          envInt("CLOUDTYKE_PAGE_SIZE", 100),
		DocsURL:           env("CLOUDTYKE_DOCS_URL", "https://github.com/bwalia/cloudtyke/blob/main/azure-emulator/docs/coverage.md"),
	}
	if v := os.Getenv("CLOUDTYKE_EXTRA_HOSTS"); v != "" {
		for _, h := range strings.Split(v, ",") {
			if h = strings.TrimSpace(h); h != "" {
				c.ExtraHosts = append(c.ExtraHosts, h)
			}
		}
	}
	return c
}

// Host returns the fully qualified name for a service sub-domain,
// e.g. Host("management") == "management.cloudtyke.local".
func (c Config) Host(sub string) string {
	if sub == "" {
		return c.Domain
	}
	return sub + "." + c.Domain
}

// URL returns the public https base URL (no trailing slash) for a sub-domain.
func (c Config) URL(sub string) string {
	if c.PublicPort == 443 || c.PublicPort == 0 {
		return "https://" + c.Host(sub)
	}
	return fmt.Sprintf("https://%s:%d", c.Host(sub), c.PublicPort)
}

// Well-known service sub-domains.
const (
	SubManagement = "management"
	SubLogin      = "login"
	SubGraph      = "graph"
	SubVault      = "vault"
	SubStorage    = "storage"
	SubACR        = "azurecr"
	SubOIDC       = "oidc"
)

// CertHosts lists every SAN the server certificate must cover.
func (c Config) CertHosts() []string {
	d := c.Domain
	h := []string{
		d, "*." + d,
		c.Host(SubManagement), c.Host(SubLogin), c.Host(SubGraph), c.Host(SubOIDC),
		"*." + c.Host(SubVault),
		"*.blob." + c.Host(SubStorage), "*.queue." + c.Host(SubStorage), "*.table." + c.Host(SubStorage), "*.file." + c.Host(SubStorage),
		"*." + c.Host(SubACR),
		"localhost", "127.0.0.1", "::1", "emulator", "cloudtyke-emulator",
	}
	return append(h, c.ExtraHosts...)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func envBool(k string, def bool) bool {
	if v := os.Getenv(k); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func envDuration(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
