# CloudTyke Azure emulator: architecture

CloudTyke is a local emulator for Azure APIs, built so you can learn and test infrastructure as code without a subscription.
The goal is not a byte-for-byte clone of Azure. It is a **control plane faithful enough that unmodified Terraform / OpenTofu
providers, the Azure CLI and SDKs behave as they do against the real cloud**, with honest, documented gaps
(see [known-gaps.md](known-gaps.md)).

> Not affiliated with or endorsed by Microsoft. Emulation is partial and for learning/testing only; never a substitute for real Azure.

## Runtime topology

```
                    host (127.0.0.1:443 only)
                              │
 ┌────────────────────────────┼──────────────── docker network "cloudtyke" (10.253.53.0/24) ─┐
 │                            ▼                                                               │
 │   ┌──────────────── emulator (10.253.53.10, single Go binary, non-root, distroless) ─────┐ │
 │   │ gateway: TLS (local CA) · request ids · ring buffer · host routing                    │ │
 │   │   management.<d> ─► ARM front end ─► authn ─► RBAC ─► pre-write hooks ─► RP handler  │ │
 │   │   login.<d>      ─► token service (OAuth2/OIDC, RS256 JWT, JWKS)                      │ │
 │   │   graph.<d>      ─► Microsoft Graph subset (Entra directory)                          │ │
 │   │   *.vault/*.blob/*.azurecr.<d> ─► data planes (phase 2+, 501 today)                   │ │
 │   │   /metadata/endpoints, /_cloudtyke/* on every host                                    │ │
 │   │ store: bbolt (/data/cloudtyke.db) or in-memory (CLOUDTYKE_EPHEMERAL=true)              │ │
 │   └───────────────────────────────────────────────────────────────────────────────────────┘ │
 │   dns (CoreDNS, 10.253.53.53): *.<d> ─► emulator; everything else ─► Docker DNS             │
 │   toolbox (profile "tools"): tofu + terraform, trusts the CA, uses the DNS sidecar          │
 │   phase 4+: k3d/kind clusters (AKS), VM containers, Azurite, registry:2, OTel collector     │
 └─────────────────────────────────────────────────────────────────────────────────────────────┘
```

`<d>` is `CLOUDTYKE_DOMAIN` (default `cloudtyke.local`).

## How Terraform finds the emulator

Verified against the provider sources (`hashicorp/go-azure-sdk/sdk` v0.20261106.1110203, used by azurerm v5.14.0; azuread v3.10.0
uses the same `environments` package):

1. `metadata_host = "management.cloudtyke.local"` (or `ARM_METADATA_HOSTNAME`) makes `environments.FromEndpoint` call
   `GET https://management.cloudtyke.local/metadata/endpoints?api-version=2022-09-01`.
2. The response is **one JSON object** (older api-versions return an array; we serve both). `name`, `resourceManager` and
   `microsoftGraphResourceId` are mandatory. The Graph *endpoint* is taken from `microsoftGraphResourceId`.
3. Tokens are requested with `POST {authentication.loginEndpoint}/{tenant}/oauth2/v2.0/token`,
   `grant_type=client_credentials` and `scope={resource}/.default`. The providers parse the JWT claims (`oid`, `tid`, `appid`)
   for `azurerm_client_config` / `azuread_client_config`.
4. Long-running operations follow `Azure-AsyncOperation` / `Location` headers. We always send `Retry-After: 1`, because the
   SDK's default first poll is 10 s.
5. azurerm lists `/subscriptions/{id}/providers` and only registers namespaces it sees as `NotRegistered`. We report every
   namespace as `Registered`, so `resource_provider_registrations` works with any setting.

HTTPS is mandatory. A local CA is generated on first start (`/data/certs`, 10 years), and a server certificate (397 days,
under Apple's 825-day limit) covers every service name, including wildcards for vault, storage and ACR data planes.
Go programs on macOS ignore `SSL_CERT_FILE`. Either trust the CA in the keychain (`tyke azure trust --install`) or run
the tools inside the Linux toolbox container.

## Request pipeline (ARM)

1. **Gateway** assigns `x-ms-request-id`, records the call in the ring buffer (`/_cloudtyke/requests`) and routes by host.
2. **api-version** is mandatory (`MissingApiVersionParameter`). It is accepted leniently by default; with
   `CLOUDTYKE_STRICT_API_VERSIONS=true` only versions declared by the resource type are accepted (`NoRegisteredProviderFound`).
3. **Authentication**: RS256 signature, expiry and audience host checks. The subscription's tenant must match `tid`
   (`InvalidAuthenticationTokenTenant`).
4. **Routing**: fixed routes for `/subscriptions`, `/tenants`, `/providers`, resource groups and resource lists. Everything else is
   parsed generically (`{scope}/providers/{ns}/{type}/{name}[/{child}/{name}…]`, with the last `/providers/` segment
   deciding the scope, so extension resources such as role assignments work at any scope). An odd-length POST is an action.
   Unregistered types return `501 NotImplemented`, naming the type, api-version and coverage matrix.
5. **Scope validation**: the resource group, management group or parent resource must exist (`ResourceGroupNotFound`,
   `ResourceNotFound`, `ParentResourceNotFound`).
6. **RBAC**: the action is derived as `{Namespace}/{type…}/{read|write|delete}` or `…/{action}/action`. Authorization walks
   the scope chain (resource → parents → resource group → subscription → management groups → `/`) and evaluates every
   assignment held by the caller or any group they transitively belong to. Built-in roles use Azure's well-known GUIDs.
   Denials return Azure's exact `AuthorizationFailed` message.
7. **Pre-write hooks**: the extension point for Azure Policy `deny`/`modify` (phase 3), locks and chaos surprises.
8. **Resource provider handler**. Most types use the `arm.Generic` CRUD engine: PUT is create-or-update (201/200),
   PATCH is a merge, DELETE of a missing resource returns 204, plus ETags with `If-Match`/`If-None-Match`, `systemData`,
   tracked locations, child-resource parent checks, cascade delete, optional async 201/202 + operation polling, and
   paging with `nextLink`.
9. **Activity log**: successful mutations are recorded with caller, action, status and correlation id, and can be queried through
   `Microsoft.Insights/eventtypes/management/values`.

Mutations are serialised by one write lock. The emulator favours determinism (labs, CI idempotency checks) over write
throughput. Async operations apply their effect immediately but report `InProgress` until `CLOUDTYKE_ASYNC_DELAY` (or
chaos `asyncDelaySeconds`) elapses, so polling code paths are exercised without nondeterminism.

## Identity

* **Directory** (`internal/entra`): tenants, users, groups, applications and service principals stored as Graph JSON.
  Fields starting with `_` (owners, members, secret hashes, federated credentials, app role assignments) are internal and
  never returned. Client secrets are stored as SHA-256 hashes; `secretText` is only returned once, by `addPassword`.
* **Token service** (`internal/auth`): `client_credentials` with a client secret, or a federated client assertion matched
  against the app's federated identity credentials (issuer, subject and audience). The assertion signature is checked with
  emulator keys for emulator issuers (AKS OIDC in phase 4) and through OIDC discovery for external issuers.
  `CLOUDTYKE_FEDERATION_INSECURE=true` skips the signature check when working offline. Tokens carry `roles` resolved from
  app role assignments on the resource service principal (for example the seeded *Microsoft Graph* SP), plus `groups`.
* **Graph** (`internal/graph`): the azuread-provider subset (see [coverage.md](coverage.md)). Application permissions are
  **enforced** (`Authorization_RequestDenied`), with `Application.ReadWrite.OwnedBy` honoured through ownership.

## Storage

One bucketed key/value store (`internal/store`), with bbolt for persistence or an in-memory implementation:

| bucket | key | value |
|---|---|---|
| `arm` | lower-cased resource ID | ARM JSON (role assignments included) |
| `graph` | `{tid}/{collection}/{id}`, `{tid}/deleted/…`, `_tenants/{tid}` | Graph JSON |
| `meta` | `subscriptions/…`, `mgs/…`, `providers/{sub}/{ns}`, `seeded` | hierarchy + flags |
| `roledefs` | custom role GUID | role definition |
| `ops` | operation id | async operation |
| `activity` | `{sub}/{nanos}/{id}` | activity log event |

Prefix scans over lower-cased IDs give cheap "children of scope" queries. `/_cloudtyke/state` exports or imports the whole
store, and `/_cloudtyke/snapshots/{name}` saves or restores named snapshots ("time travel" for labs).

## Adding a resource provider

1. Create `internal/rp/<namespace>/`, then register one or more `arm.TypeRegistration`s with api-versions, scope kinds, phase and notes.
2. Use `arm.Generic(arm.GenericOptions{…})` and add `Prepare` (defaults and validation), `AfterWrite`, `OnDelete`,
   `Actions` and `Present` hooks only where Azure behaves specially.
3. Add unit tests and a Terraform example that applies it, then run `make coverage` to regenerate `docs/coverage.md`.

See [decisions/](decisions/) for the ADRs behind these choices.
