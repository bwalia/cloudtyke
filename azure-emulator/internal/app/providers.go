package app

import (
	"log/slog"
	"net/http"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/arm"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/config"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/containerservice"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/databases"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/keyvault"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/managedidentity"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/monitoring"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/network"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/policy"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/registry"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/storage"
)

// registerPhase2 wires the networking, identity, monitoring, registry and
// AKS resource providers and returns the AKS RP (nil backend = API stub).
func registerPhase2(s *arm.Server, backend ...containerservice.Backend) (*containerservice.RP, map[string]http.Handler) {
	dataPlanes := map[string]http.Handler{}
	policy.Register(s)
	kv := keyvault.Register(s)
	dataPlanes["."+s.Cfg.Host("vault")] = kv.DataPlane()
	databases.Register(s)
	st := storage.Register(s)
	dataPlanes[".blob."+s.Cfg.Host("storage")] = st.BlobDataPlane()
	dataPlanes[".file."+s.Cfg.Host("storage")] = st.FileDataPlane()
	network.Register(s)
	managedidentity.Register(s)
	monitoring.Register(s)
	registry.Register(s)
	var b containerservice.Backend = containerservice.StubBackend{}
	if len(backend) > 0 && backend[0] != nil {
		b = backend[0]
	}
	return containerservice.Register(s, b), dataPlanes
}

// aksBackend picks what answers kubectl for AKS clusters:
// CLOUDTYKE_AKS_BACKEND=k3s runs a k3s container per cluster (needs the
// Docker socket), anything else uses the built-in API stub.
func aksBackend(cfg config.Config, logger *slog.Logger) containerservice.Backend {
	if cfg.AKSBackend == "k3s" {
		b, err := containerservice.NewK3sBackend(cfg, logger)
		if err != nil {
			logger.Error("AKS k3s backend unavailable, falling back to the API stub", "err", err)
			return nil
		}
		logger.Info("AKS backend: k3s (one container per cluster)")
		return b
	}
	return nil
}
