# Labs and interview mode (phase 6)

Labs make CloudTyke a self-contained, gradable DevOps / platform-engineering exercise environment. Each lab lives in
`labs/<id>/`:

| Path | Audience | Purpose |
|---|---|---|
| `README.md` | candidate | brief, constraints, time box |
| `starter/` | candidate | broken or empty Terraform |
| `seed.yaml` | both | tenant, subscriptions and identities preset (deterministic) |
| `verify/` | grader | automated checks against emulator state and the Kubernetes API |
| `rubric.md` | interviewer | scoring guide, kept separate from the brief |
| `../../solutions/<id>/` | interviewer | reference solution, excluded from candidate copies |

`tyke azure labs verify <id>` prints a human-readable report and JSON with pass/fail for each requirement.

## Planned labs

1. **Hub-spoke landing zone with policy guardrails:** management groups, peering, NSGs, private DNS, Deny policies.
2. **Private multi-tenant AKS platform:** namespace isolation, quotas, NetworkPolicy, Gatekeeper.
3. **Workload identity + Key Vault through a Private Endpoint:** a federated credential and a CSI driver or SDK read.
4. **IaC state strategy:** module design, plus drift detection and remediation (`tyke azure chaos drift`).
5. **OpenTelemetry pipeline for AKS:** logs, metrics and traces into Log Analytics or the observability profile.
6. **EPAC-style policy-as-code rollout.**
7. **AKS upgrade strategy:** surge node-pool upgrade without downtime.
8. **FinOps review:** find and fix the cost problems in the seeded estate.
9. **Debugging:** broken private DNS, peering or NSG; find the root cause.
10. **System-design exercise template** with a rubric.

## What phases 0 and 1 already provide

* **Deterministic seeds** (`CLOUDTYKE_SEED_FILE`) and fast reset (`POST /_cloudtyke/reset`).
* **Snapshots** (`tyke azure snapshot save|restore`) to put an estate into a known broken state in seconds.
* **Chaos**: throttling, transient 5xx, latency and slow async operations (`tyke azure chaos set`).
* **The request log** (`tyke azure requests`), so interviewers can see exactly what the candidate's tooling called.
* **Activity log** queries, as evidence for "who changed what".
