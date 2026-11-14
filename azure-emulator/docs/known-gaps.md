# Known gaps and fidelity limits

CloudTyke aims to make **unmodified IaC behave as it does on Azure**, but it is not Azure. This page lists where behaviour
differs, so learners aren't misled and **interview candidates never lose marks to an emulator quirk**. If you hit
something unlisted, it's a bug: please open an issue with the output of `tyke azure requests`.

## General

* **Not every resource type exists.** Anything outside [coverage.md](coverage.md) returns `501 NotImplemented`, naming the
  type and api-version. Real Azure would accept it.
* **api-versions are accepted leniently.** By default any well-formed api-version is accepted for an emulated type, and response
  shapes follow the newest API we model. Set `CLOUDTYKE_STRICT_API_VERSIONS=true` to reject versions we haven't modelled.
* **Error codes and messages** follow Azure's public documentation and observed responses for the paths Terraform uses.
  Wording can differ in rarely-hit branches.
* **Consistency is immediate.** Azure is eventually consistent; CloudTyke is not. Providers still run their own
  consistency pollers, so some operations take longer than you'd expect against a local service. For example, azurerm v5 waits for 20
  consecutive "not found" reads (about 3.5 minutes) after deleting a custom role definition, and azuread re-reads new
  objects several times at 5-second intervals. That time is spent in the provider, not the emulator.
* **Throttling, quotas and regional capacity** only happen when you turn them on through chaos (`tyke azure chaos set`).
* **One tenant is fully modelled.** Multi-tenant apps and cross-tenant (B2B / Lighthouse) scenarios are not.

## Identity (Entra ID / Graph)

* **Only app-only tokens are issued**, through `client_credentials` with a secret or a federated assertion. Interactive sign-in,
  device code, ROPC, certificates (`client_assertion` signed by an app certificate) and `az login` are not supported yet.
* **Graph permissions** are enforced for the application permissions listed in [coverage.md](coverage.md). Delegated
  scopes, consent prompts and admin consent workflows are not modelled. An app role assignment on the Microsoft Graph
  service principal counts as consent.
* **Directory roles are modelled but not enforced.** Graph checks application permissions only.
* **Conditional Access, PIM, MFA, Identity Protection, B2C and access reviews** are out of scope (see [roadmap.md](roadmap.md)).
* **Group-based licensing, dynamic membership rules and mail-enabled groups** are stored but have no effect.
* **External OIDC issuers** (GitHub, GitLab) need outbound network so the emulator can fetch their JWKS to verify federated
  assertions. Offline, set `CLOUDTYKE_FEDERATION_INSECURE=true` to match on issuer, subject and audience only.

## Azure RBAC

* **Enforced on every ARM call** using built-in roles with Azure's GUIDs. Permission lists are a faithful subset of the
  public definitions, not a full copy of the roughly 400 built-in roles.
* **Deny assignments, ABAC conditions (`condition`) and classic administrators** are stored but not evaluated.
* **Data-plane RBAC** (`dataActions`) is modelled in the engine but only used once data planes arrive (Key Vault and
  Storage in phase 2).

## Resource Manager

* **ARM template deployments, what-if, deployment stacks, locks and tags-at-scope (`Microsoft.Resources/tags`)** arrive in phase 2
  or later. Until then they return 501.
* **Management groups** exist for RBAC inheritance (seeded in `seed.yaml`). The `Microsoft.Management` API arrives in phase 3.
* **Subscriptions** come from the seed. Creating subscriptions (`Microsoft.Subscription/aliases`) is not supported yet.

## Planned phases: limits we already know about

* **Networking (phase 2):** VNets map to Docker networks where possible. There is no real data path for Azure Firewall, load
  balancers, VPN or ExpressRoute; those are API models only.
* **VMs (phase 4):** a "VM" is a container, so there is no real kernel, no boot diagnostics fidelity and no Windows images.
  VM sizes only label the container.
* **AKS (phase 4):** clusters are k3d/kind, not AKS. Azure-managed add-ons (Azure Policy add-on, Defender, Key Vault CSI
  managed identity wiring, AGIC) are only modelled where noted in coverage. Node images and upgrade timings are simulated.
* **FinOps (phase 5):** costs are deterministic and synthetic, generated from a YAML price catalogue.
