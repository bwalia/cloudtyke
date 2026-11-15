# Roadmap

Every phase can ship on its own, is merged with `main` green, and has an executable "done when" test.

| Phase | Scope | Done when | Status |
|---|---|---|---|
| 0 | Skeleton: Go module, gateway, local CA/PKI, DNS sidecar, `/metadata/endpoints`, health, docs, CI | `docker compose up` serves HTTPS metadata and `tofu init` with `metadata_host` succeeds | **done** |
| 1 | Token service + JWKS, Graph subset, tenants/subscriptions, resource groups, providers registry, enforced RBAC, async operations, activity log | `azurerm_resource_group` and `azuread_user/group/application/service_principal` apply, a second plan is clean, and destroy succeeds | **done** (see `examples/terraform/quickstart`, `entra`) |
| 2 | Networking (VNets, subnets, peering state machine, NSGs, route tables, public IPs, NICs, private endpoints, private DNS zones + links + records), Key Vault control plane + data plane, Storage control plane + Azurite, `azurerm` remote state with blob lease locking, managed identities, locks | `examples/terraform/landing-zone` (minus policy) and `remote-state` pass | **partly done**: networking, private DNS, managed identities, Log Analytics and ACR control plane ship with `landing-zone`; Key Vault, Storage + remote state and locks are next |
| 3 | Management groups RP, Azure Policy (definitions, initiatives, assignments, exemptions, evaluator for Deny/Audit/Modify/Append/DINE-stub, aliases, compliance via PolicyInsights), EPAC-compatible shapes | A Deny policy blocks a non-compliant resource with Azure's `RequestDisallowedByPolicy` message | planned |
| 4 | VMs as containers (+ IMDS), AKS via k3d/kind (node pools, credentials, upgrades, OIDC issuer, workload identity end to end), ACR attach | `examples/terraform/aks-platform` applies, `kubectl get nodes` works, and a pod reads a Key Vault secret with workload identity | **partly done**: AKS control plane, node pools, credentials, upgrades, OIDC issuer, `kubectl` via API stub or k3s; Key Vault data plane and VMs next |
| 5 | OTel collector, Log Analytics + KQL-lite, diagnostic settings, alerts, App Insights, ACR (registry:2), Cost Management stubs | observability and FinOps examples apply; telemetry is queryable | planned |
| 6 | Ten labs with verifiers and rubrics, `tyke azure labs`, chaos drift injection, devcontainer, polished README with quickstart GIF | `tyke azure labs verify <id>` scores each lab's reference solution at 100% | planned |

## Explicitly out of scope until further notice

* Conditional Access, PIM, MFA and Identity Protection (roadmap after phase 6)
* Interactive user sign-in flows (auth code, device code). `az login` via `az cloud register` is a stretch goal after phase 2.
* Data-plane networking fidelity beyond Docker networks (no real Azure Firewall or Load Balancer data path)
* Billing accuracy: FinOps data is deterministic and synthetic
