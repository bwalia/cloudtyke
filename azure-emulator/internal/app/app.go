// Package app wires every emulator component into one http.Handler. It is
// used by cmd/emulator and by the integration tests.
package app

import (
	"fmt"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/network"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/activitylog"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/admin"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/arm"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/asyncop"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/auth"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/chaos"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/config"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/gateway"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/graph"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/hierarchy"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/pki"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rbac"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/authorization"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/insights"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/resources"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/seed"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/selftest"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

// App is a fully wired emulator.
type App struct {
	Cfg     config.Config
	Store   store.Store
	PKI     *pki.Bundle
	Signer  *auth.Signer
	Handler http.Handler
	ARM     *arm.Server
	Seed    seed.File
}

// Registry builds the resource-provider registry without any state (used
// to render docs/coverage.md).
func Registry() *arm.Registry {
	s := &arm.Server{Registry: arm.NewRegistry()}
	resources.Register(s)
	authorization.Register(s)
	insights.Register(s)
	registerPhase2(s)
	return s.Registry
}

// New builds the emulator. With cfg.Ephemeral the store and signing key
// live in memory; certificates are always written to cfg.DataDir/certs.
func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	var st store.Store
	var signer *auth.Signer
	var err error
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, err
	}
	if cfg.Ephemeral {
		st = store.NewMemory()
		signer, err = auth.NewEphemeralSigner()
	} else {
		st, err = store.OpenBolt(filepath.Join(cfg.DataDir, "cloudtyke.db"))
		if err != nil {
			return nil, err
		}
		signer, err = auth.LoadOrCreateSigner(filepath.Join(cfg.DataDir, "keys"))
	}
	if err != nil {
		return nil, err
	}
	bundle, err := pki.LoadOrCreate(filepath.Join(cfg.DataDir, "certs"), cfg.CertExportDir, cfg.CertHosts())
	if err != nil {
		return nil, err
	}

	seed.SetDomain(cfg.Domain)
	seedFile, _, err := seed.Load(cfg.SeedFile)
	if err != nil {
		return nil, err
	}
	if !seed.IsSeeded(st) {
		if err := seed.Apply(st, seedFile); err != nil {
			return nil, fmt.Errorf("seeding: %w", err)
		}
		logger.Info("seeded store", "tenants", len(seedFile.Tenants))
	}

	dir := entra.New(st)
	h := hierarchy.New(st)
	authSvc := auth.NewService(cfg, signer, dir)
	ch := chaos.New(cfg.AsyncDelay)
	armSrv := &arm.Server{
		Cfg: cfg, Store: st, Auth: authSvc, Dir: dir, H: h,
		RBAC:     rbac.New(st, h, dir),
		Ops:      asyncop.New(st, ch.AsyncDelay),
		Activity: activitylog.New(st),
		Registry: arm.NewRegistry(),
	}
	resources.Register(armSrv)
	authorization.Register(armSrv)
	insights.Register(armSrv)
	aks, dataPlanes := registerPhase2(armSrv, aksBackend(cfg, logger))
	aks.CAPEM = bundle.CAPEM
	authSvc.IssuerJWKS = aks.JWKSForIssuer

	services := admin.Services(armSrv.Registry)
	selfTest := selftest.Handler(cfg.Domain, services, func() (http.Handler, func(), error) {
		iso, done, err := NewIsolated(cfg)
		if err != nil {
			return nil, nil, err
		}
		return iso.Handler, done, nil
	})

	reqLog := gateway.NewRequestLog(500)
	gw := &gateway.Gateway{
		Cfg:        cfg,
		ARM:        ch.Middleware(armSrv),
		Login:      authSvc,
		Graph:      &graph.Server{Cfg: cfg, Auth: authSvc, Dir: dir},
		Kube:       aks.KubeAPI,
		DataPlanes: dataPlanes,
		OIDC:       aks.OIDCHandler(),
		Exists:     dataPlaneExists(cfg, st),
		Admin:      &admin.Server{Cfg: cfg, Store: st, Registry: armSrv.Registry, Log: reqLog, Chaos: ch, CAPEM: bundle.CAPEM, Seed: seedFile, WriteMu: &armSrv.WriteMu, SelfTest: selfTest, Network: network.LabHandler(armSrv)},
		Log:        reqLog,
		Logger:     logger,
	}
	return &App{Cfg: cfg, Store: st, PKI: bundle, Signer: signer, Handler: gw, ARM: armSrv, Seed: seedFile}, nil
}

// NewIsolated builds a throwaway in-memory emulator with cfg's domain and
// the default seed, used by the self-test so it never touches real state.
// Call the returned func to release its temporary directory.
func NewIsolated(cfg config.Config) (*App, func(), error) {
	cfg.AKSBackend = "stub" // never start containers for a self-test
	dir, err := os.MkdirTemp("", "cloudtyke-selftest-")
	if err != nil {
		return nil, nil, err
	}
	done := func() { _ = os.RemoveAll(dir) }
	c := cfg
	c.DataDir, c.CertExportDir, c.Ephemeral, c.SeedFile = dir, "", true, ""
	c.StrictAPIVersions = false
	c.AsyncDelay = 50 * time.Millisecond
	if c.PageSize == 0 {
		c.PageSize = 100
	}
	a, err := New(c, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		done()
		return nil, nil, err
	}
	return a, func() { _ = a.Store.Close(); done() }, nil
}
