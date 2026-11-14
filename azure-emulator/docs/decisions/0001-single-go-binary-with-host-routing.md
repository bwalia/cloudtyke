# ADR 0001: one Go binary with in-process host routing

* Status: accepted (phase 0)

## Context

The prompt asks for "a single edge gateway with host-based routing". Terraform needs ARM, the token service and Graph
on different host names, all over HTTPS with one trusted CA.

## Decision

Serve every endpoint from one Go process (`cmd/emulator`). `internal/gateway` routes by `Host`: `management.` goes to
ARM, `login.` to the token service, `graph.` to Graph, and data-plane wildcards to their services. There is no separate
reverse proxy container. A CoreDNS sidecar resolves `*.<domain>` to the emulator for containers. Compose network aliases
cover the three fixed names even without the sidecar.

## Consequences

* **Simpler to ship:** one image, one certificate, one log stream, shared state without RPC. Request IDs and the
  request ring buffer see all traffic.
* **Later data planes run in the same process when they're simple.** Key Vault and the ARM side of Storage do. Heavier
  backends (Azurite, registry:2, k3d) run as sidecars, and the gateway proxies to them.
