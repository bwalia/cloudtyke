# CloudTyke Azure emulator

**CloudTyke: a small cloud for big mistakes.** A Docker-based local emulator for Azure APIs, in the spirit of LocalStack.
Point **Terraform / OpenTofu** (and soon the Azure CLI and SDKs) at it, and build "Azure" infrastructure on your own
machine with no subscription, no bill and no blast radius.

> **Not affiliated with or endorsed by Microsoft.** Emulation is partial and for learning and testing only. It is never a
> substitute for real Azure. "Azure", "Entra" and "Microsoft Graph" are used only to describe which APIs are emulated.

It is built for two audiences:

* **Learners:** practise Azure RBAC, Entra ID, landing zones and IaC safely and for free.
* **Interviewers and candidates:** a self-contained, gradable platform-engineering test environment (labs arrive in phase 6).

## Five-minute quickstart

You need Docker with Compose v2. Nothing else is required on the host.

```bash
cd azure-emulator
make up        # builds and starts the emulator + DNS sidecar, exports certs/ca.pem
make shell     # a toolbox shell with tofu + terraform, already trusting the CA and logged in

# inside the toolbox
cd examples/terraform/quickstart
tofu init && tofu apply
tofu plan      # "No changes": the emulator is idempotent
tofu destroy
```

Want to see what Terraform actually called? Run `curl -s --cacert certs/ca.pem https://localhost/_cloudtyke/requests | jq`,
or `bin/tyke azure requests` after `make build`.

<!-- asciinema / GIF placeholder: quickstart apply + requests view -->

### Using your own host tools

```bash
make build
bin/tyke azure trust --install          # macOS: the Go-based providers ignore SSL_CERT_FILE
bin/tyke azure hosts | sudo tee -a /etc/hosts
eval "$(bin/tyke azure env)"            # ARM_METADATA_HOSTNAME, ARM_CLIENT_ID, ...
cd examples/terraform/quickstart && terraform init && terraform apply
```

## What works today (phases 0 and 1)

| Area | Highlights |
|---|---|
| Cloud discovery | `/metadata/endpoints` for `metadata_host`; HTTPS with a generated local CA; wildcard DNS sidecar |
| Entra ID token service | client secret + federated (workload identity) assertions, RS256 JWTs with real claims, OIDC discovery, JWKS |
| Microsoft Graph | users, groups, owners/members, applications, service principals, passwords, federated credentials, app role assignments, directory roles, deleted items; **application permissions enforced** |
| ARM core | subscriptions, tenants, locations, provider registration, resource groups (async delete, cascade), resource listing |
| Azure RBAC | built-in roles (real GUIDs), custom roles, assignments at MG, subscription, RG and resource scope with inheritance; **enforced on every call** (`AuthorizationFailed`) |
| Platform | activity log, async operations, paging, ETags, structured logs, request ring buffer, snapshots, reset, chaos (429/5xx/latency/slow LROs) |

Everything else returns `501 NotImplemented`, naming the resource type, so you always know where the edge is. See
[docs/coverage.md](docs/coverage.md) (generated) and the honest [docs/known-gaps.md](docs/known-gaps.md).

## Seeded identities (local-only secrets)

| Principal | Client ID | Secret | Access |
|---|---|---|---|
| cloudtyke-bootstrap | `a99c0000-0000-4000-8000-000000000001` | `cloudtyke-bootstrap-secret` | Owner at `/`, Graph Directory/Application/User/Group ReadWrite |
| cloudtyke-reader | `a99c0000-0000-4000-8000-000000000002` | `cloudtyke-reader-secret` | Reader on sub-sandbox, Graph Directory.Read.All |
| ada@cloudtyke.local | (user) | `CloudTyke-Dev-1!` | Contributor on sub-sandbox |

Tenant `7c10d7e4-0000-4000-8000-000000000001`. Subscriptions: `sub-connectivity` (`…0001`), `sub-management` (`…0002`) and
`sub-sandbox` (`…0003`), under the management groups `ct-platform` and `ct-sandbox`. Change any of this with your own
seed file (`CLOUDTYKE_SEED_FILE`, same schema as [internal/seed/default.yaml](internal/seed/default.yaml)).

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `CLOUDTYKE_DOMAIN` | `cloudtyke.local` | base DNS name (`management.`, `login.`, `graph.` …) |
| `CLOUDTYKE_HTTPS_PORT` | `443` | host port (compose) |
| `CLOUDTYKE_EPHEMERAL` | `false` | in-memory state |
| `CLOUDTYKE_SEED_FILE` | embedded | custom tenants, subscriptions and identities |
| `CLOUDTYKE_ASYNC_DELAY` | `0s` | how long async operations stay `InProgress` |
| `CLOUDTYKE_STRICT_API_VERSIONS` | `false` | reject api-versions that aren't modelled |
| `CLOUDTYKE_ADMIN_TOKEN` | empty | protects `/_cloudtyke/*` (except health) |
| `CLOUDTYKE_SUBNET` / `CLOUDTYKE_EMULATOR_IP` / `CLOUDTYKE_DNS_IP` | `10.253.53.0/24`, `.10`, `.53` | compose network |

## Admin API (`/_cloudtyke/`)

`health`, `version`, `ca.pem`, `env`, `metadata`, `coverage[?format=markdown]`, `requests?limit=N`,
`state` (GET export / PUT import), `snapshots`, `snapshots/{name}` (POST save, DELETE), `snapshots/{name}/restore`,
`reset`, `seed`, `chaos` (GET / PUT / DELETE) and `selftest[?format=json&service=FILTER]`.

## Testing every service

Each row of the [coverage matrix](docs/coverage.md) has one self-test check that drives the service over its real HTTP API:
the happy path, the error codes Terraform and the SDKs depend on, and RBAC / Graph permission enforcement.

| Command | What it does | Time |
|---|---|---|
| `make test-services` | every service check as a Go subtest (in-process, no Docker) | ~1 s |
| `make test-all SKIP_E2E=1` | unit, integration and service tests plus a report in `reports/` | ~15 s |
| `make test-all` | the same plus the Terraform lifecycle with OpenTofu and Terraform | ~12 min |
| `make selftest` / `bin/tyke azure selftest [FILTER]` | runs the checks inside the running emulator and prints ✅/❌ per service | <1 s |
| `bin/tyke azure tour [-v] [-all] [FILTER]` | walks through each service step by step in the terminal, pausing after each (`-v` shows request/response bodies) | interactive |

`make test-all` writes `reports/index.html` (open it in a browser), `reports/junit.xml` and `reports/summary.md`, prints a
per-service table and exits non-zero if anything failed.

**Live page for interviews and lab demos:** open <https://localhost/_cloudtyke/selftest> while the emulator is running.
Every load runs the suite against a fresh in-memory emulator, so it never changes your state. Expand a service to see each
HTTP call it made, or click **Take the tour** (`?tour=1`) to step through the services one at a time with ← →: each step
shows what it proves, the expected and actual result, the extra assertions and the request and response bodies. Add `?service=graph` to filter, or `?format=json` for scripts; the page returns HTTP 417 when
anything fails, so `curl -f` works in scripts.

**Web console:** `make dashboard` starts a full console on <http://localhost:4310>. It has resources, identity, access
control, the activity log, tokens and fault injection, plus the same per-service checks and a guided tour with autoplay.
See [../dashboard/README.md](../dashboard/README.md).

## Development

```bash
make lint test-race       # unit + in-process HTTPS integration tests
make test-all             # everything above + Terraform e2e, with an HTML report in reports/
make coverage             # regenerate docs/coverage.md
make tf-e2e               # every example: init/validate/apply/plan(no drift)/destroy with OpenTofu
make tf-e2e-terraform     # the same with Terraform
```

Read [docs/architecture.md](docs/architecture.md) for the design and how to add a resource provider,
[docs/roadmap.md](docs/roadmap.md) for the phases, [docs/compatibility.md](docs/compatibility.md) for tested versions and
[docs/security.md](docs/security.md) before exposing anything.

## Licence

Apache-2.0. See [LICENSE](LICENSE).
