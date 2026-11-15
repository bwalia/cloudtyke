# CloudTyke Academy: becoming an Azure DevOps engineer

> **Status: draft outline for review.** Modules are listed with their goals, what interns build and what already
> exists. Nothing below is written up as a full lesson yet.

A hands-on course for interns, run entirely on the CloudTyke emulator: no Azure subscription, no cost, and every
mistake is safe. Error messages, status codes and resource shapes match Azure's, so what interns learn carries over.

## Who it's for

Interns with basic Linux, Git and some programming, but no cloud experience. By the end they can build, secure,
deploy to, observe and debug a small Azure platform with Infrastructure as Code and a pipeline, and explain
*how, where and what* at every step.

## How every module works

Each module is a folder `academy/NN-name/` with the same five parts:

1. **Why it matters:** the real-world problem, in one paragraph.
2. **How it works:** the mental model, with a diagram that shows *what talks to what, and where it runs*.
3. **Do it:** a guided exercise with copy-paste commands, building on the previous module.
4. **Break it:** deliberate failures (wrong role, missing tag, bad route) to read the error, find the cause and fix
   it. This is where most of the learning happens.
5. **Check yourself:** a `verify.sh` that grades the result (PASS / FAIL with the reason), plus 5–8 questions.

Every module ends with **"In real Azure"**: what is the same, what CloudTyke simplifies, and the portal or `az`
equivalent.

## The map: where everything runs

The first diagram every intern sees, reused in every module:

```
your laptop
└── Docker
    ├── emulator ─────────── "Azure": ARM (management.), Entra ID (login.), Graph, Key Vault, Storage, AKS APIs
    ├── dns ──────────────── resolves *.cloudtyke.local to the emulator
    ├── toolbox ──────────── where you run tofu / terraform / kubectl / az-style curl (trusts the emulator's CA)
    ├── cloudtyke-aks-* ──── one k3s container per AKS cluster (make up-k3s): your pods run here
    └── apps / observability your workloads and Grafana, Jaeger, Prometheus, Loki
```

## Modules

| # | Module | Interns learn | Builds on (already in the repo) | Time |
|---|---|---|---|---|
| 0 | **Orientation** | What a cloud is: tenant → management groups → subscriptions → resource groups → resources. Control plane vs data plane. Running CloudTyke, the toolbox, the dashboard. | `make up`, `tyke tour`, dashboard | ½ day |
| 1 | **Identity (Entra ID)** | Users, groups, app registrations vs service principals vs managed identities. How a token is issued; decoding a JWT (`aud`, `oid`, `roles`). Client secret vs federated credential. | `examples/terraform/entra`, dashboard *Identity* and *Tokens* | 1 day |
| 2 | **Authorization (RBAC)** | Role = actions at a scope; inheritance down the hierarchy; control-plane actions vs data actions (why *Owner* can't read a blob). Reading `AuthorizationFailed`. Least privilege with groups. | `examples/terraform/quickstart`, seeded Reader principal, dashboard *Access* | 1 day |
| 3 | **Infrastructure as Code** | `init / plan / apply / destroy`, state and why it matters, variables and outputs, providers per subscription, drift and idempotency (a no-change plan). | `quickstart`, `landing-zone`, `make tf-e2e`, `tyke azure chaos drift` | 1 day |
| 4 | **Networking** | VNets, subnets, NSGs, route tables, peering (and why it isn't transitive), hub-and-spoke, private DNS. Tracing a packet hop by hop. | `labs/hub-spoke`, `labs/avnm`, dashboard *Network*, `/_cloudtyke/network/trace` | 1½ days |
| 5 | **Governance (Azure Policy)** | Definitions, initiatives, assignments, effects (deny, audit, modify, denyAction), exemptions with expiry, compliance reports. Guardrails as a product for other teams. | `hello-web` guardrail, Contoso Pay `governance.tf` | 1 day |
| 6 | **Secrets and data** | Key Vault (RBAC vs access policies, soft delete, purge protection), Storage (Entra vs shared key, containers, file shares, lifecycle rules), private databases. | `hello-web`, Contoso Pay `payments.tf`, `settlement.tf` | 1 day |
| 7 | **AKS** | Cluster anatomy (node pools, `MC_` resource group, kubelet identity), getting credentials, Entra-only clusters and Azure RBAC for Kubernetes, workload identity end to end. | `aks-platform`, `hello-web`, `make aks-k3s-smoke`, dashboard *Kubernetes* | 1½ days |
| 8 | **CI/CD** | A pipeline that plans on PRs and applies on merge, GitHub OIDC (no stored secrets), policy denials as a failing check, promoting a change safely. | `.github/workflows/hello-web.yml`, Contoso Pay CI identities (`identity.tf`) | 1 day |
| 9 | **Observability** | Traces, metrics and logs with OpenTelemetry; RED metrics; following one request across services; dashboards that answer questions. | Contoso Pay apps, Grafana, Jaeger, Loki | 1 day |
| 10 | **Operations and incidents** | Throttling and outages (`chaos`: 429, 5xx, latency), the activity log, snapshots and restore, a runbook, a blameless post-incident review. | `tyke azure chaos`, dashboard *Activity*, snapshots | 1 day |
| 11 | **Capstone** | Build a small platform for a fictional new team from a written brief: identity, network, guardrails, secrets, AKS, pipeline, dashboards. Graded by `verify.sh` plus a short review. | everything above | 2 days |

About 13 days in total, at roughly one module a day.

### Module detail (draft)

**0 Orientation.** Start the emulator, run the tour, open the dashboard. Exercise: find the three seeded
subscriptions and which management group each belongs to. Break it: call the API without a token and read the 401.

**1 Identity.** Get a token with `curl` in the toolbox and decode it; create a user, a group and an app with
Terraform; add a federated credential and exchange a token. Break it: the wrong `aud`, an expired secret, a federated
subject that doesn't match.

**2 RBAC.** Give a group *Reader* on one resource group and prove it can't write; give a service principal
*Storage Blob Data Reader* and compare control-plane and data-plane access. Break it: grant at the wrong scope;
use *Contributor* where a data role is needed.

**3 IaC.** Write a module from scratch, plan and apply, then change something in the "portal" (an API call) and
detect the drift. Break it: delete the state file and see what happens; a hard-coded name that collides.

**4 Networking.** Hub-and-spoke with a firewall route; trace spoke-to-spoke traffic; fix asymmetric routing. Break it:
peering only one side; a missing `allowForwardedTraffic`; an NSG that blocks the return path.

**5 Governance.** Write a deny policy and a modify policy, assign them, and watch Terraform fail with the exact
message; grant a time-boxed exemption. Break it: an alias that doesn't match, `DoNotEnforce` left on.

**6 Secrets and data.** Store and read a secret with a managed identity; write blobs with Entra auth after disabling
shared keys; set a lifecycle rule. Break it: delete a vault and recreate it (soft delete), try to purge with
protection on.

**7 AKS.** Build a cluster with workload identity, deploy `hello-web`, follow each token exchange. Break it: a service
account that doesn't match the federated subject; a missing role assignment (the page shows the 403).

**8 CI/CD.** Fork the `hello-web` workflow: plan on PR with the plan in the summary, apply on merge, a guardrail test.
Break it: a pipeline identity without rights; a change the policy denies; a failed apply mid-way.

**9 Observability.** Run Contoso Pay; find a slow payment's trace, the log lines for it, and the metric that shows
the pattern; build one dashboard panel. Break it: the collector down, a wrong exporter option.

**10 Operations.** Turn on chaos, watch retries and failures, read the activity log, restore a snapshot, write the
incident timeline.

**11 Capstone.** A brief for a fictional team, e.g. "Fabrikam Hotels wants a booking API on our platform by
Friday". Interns deliver Terraform, a pipeline and a dashboard; `verify.sh` checks the requirements; a mentor
reviews the design and the trade-offs.

## Reference material (shared by all modules)

- **Glossary**: tenant, subscription, scope, principal, role, data action, federated credential, workload identity,
  control plane, data plane, LRO, ...
- **Cheat sheets**: Terraform/OpenTofu, kubectl, decoding tokens, reading Azure error codes.
- **"How CloudTyke maps to Azure"**: what is emulated, what is simplified, what doesn't exist
  ([coverage](../docs/coverage.md), [known gaps](../docs/known-gaps.md)).
- **Instructor notes** per module: timings, common mistakes, the answers, how to run the session.

## Open questions for review

1. **Pipelines:** GitHub Actions only, or also **Azure DevOps Pipelines** (YAML)? "Azure DevOps engineer" can
   mean either; the concepts are the same, the syntax differs.
2. **Certification alignment:** map modules to **AZ-104** (administrator) and **AZ-400** (DevOps engineer) skills?
3. **Format:** self-paced (each intern alone with `verify.sh`) or cohort (instructor-led sessions)?
4. **Length:** about 13 days as drafted. Shorter (a 5-day bootcamp) or longer (with weekly real-Azure sessions)?
5. **Assessment:** automated checks only, or checks plus a mentor review and a short presentation?
