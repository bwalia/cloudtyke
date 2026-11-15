# CloudTyke console

A web console for the [CloudTyke Azure emulator](../azure-emulator/README.md), built with Next.js. It reads and changes
everything through the emulator's real ARM, Entra ID and Microsoft Graph APIs, so what you see is exactly what Terraform
and the SDKs see.

> Local tool. Not affiliated with or endorsed by Microsoft.

## Run it

```bash
# with Docker (emulator + console)
make -C azure-emulator dashboard          # http://localhost:4310

# from source, hot reload (emulator must be running: make -C azure-emulator up)
cd dashboard && npm install && npm run dev   # http://localhost:4310
```

Both bind to `127.0.0.1` only, because the console acts as the seeded admin identities.

### From another device on your LAN

LAN mode is opt-in and always protected by a sign-in prompt. `CLOUDTYKE_DASHBOARD_USER` is optional; if you leave it unset, any user name is accepted:

```bash
export CLOUDTYKE_DASHBOARD_USER=lab-admin CLOUDTYKE_DASHBOARD_PASSWORD='pick-something'
make -C azure-emulator dashboard-lan   # Docker
npm run start:lan                      # or from source (after npm run build)
```

Then open `http://<your-mac's-LAN-IP>:4310`. You can find the IP with `ipconfig getifaddr en0`. Only the console is
exposed; the emulator itself stays on localhost. Use it on trusted networks only, because traffic is plain HTTP.

## What's in it

| Page | What you can do |
|---|---|
| **Overview** | live self-test score, object counts, service health grid, management-group hierarchy, request traffic chart, a Terraform provider snippet |
| **Services** | every coverage-matrix row with its end-to-end check: each HTTP call, expected vs actual status, assertions, request/response bodies, re-run |
| **Guided tour** | one service per slide with autoplay (`space`), arrow keys and full screen (`f`), made for demos and interviews |
| **Resources** | subscriptions → resource groups tree; create (real `PUT`, request preview), edit tags (`PATCH`), async delete with polling; IAM, activity and generated Terraform `import` blocks per group |
| **Network** | every VNet across subscriptions drawn as a live hub-and-spoke topology (peerings are green only when both sides exist), subnets with NSG / route table / AKS usage, peering details |
| **Kubernetes** | AKS clusters with node pools, networking, identities and add-ons; stop / start, upgrade (one minor at a time), admin kubeconfig download, and live `kubectl get nodes` through the cluster's own credential |
| **Identity** | users, groups (members / owners / nesting), app registrations (client secrets that really authenticate, federated credentials), enterprise apps (Graph app-role grants), directory roles, deleted items (restore / purge), organization |
| **Access control** | role assignments at any scope with inheritance, add / remove assignments, built-in and custom roles, and *Check my access*, which shows effective permissions |
| **Activity** | the Azure Activity Log in plain English, plus a live, filterable stream of every HTTP request the emulator serves |
| **Tokens & OIDC** | the current access token decoded claim by claim, the OpenID configuration and the JWKS signing keys |
| **Emulator** | health, environment variables for Terraform / az / SDKs, fault injection (throttling, 5xx, latency, async delay), snapshots, reset, CA download |

Across the app:
- **⌘K** opens a command palette that searches subscriptions, resource groups, users, groups, apps and services, and runs actions.
- `g` followed by a key jumps to a page (`g r` for Resources, `g i` for Identity, and so on).
- The **identity switcher** in the top bar re-runs every call as `cloudtyke-reader`, so you can watch RBAC and Graph permission enforcement happen live.
- Dark and light themes are both supported.

## How it works

```
browser ──► Next.js route /api/emu/{arm|graph|admin|login}/… ──► https://{management|graph|login}.cloudtyke.local
                       (adds a client-credentials token for the chosen identity; the browser never sees secrets)
```

The server gets tokens from the emulator's own login endpoint with the seeded service principals. It verifies TLS against
the emulator CA, found in this order: `CLOUDTYKE_CA_FILE`, then `../azure-emulator/certs/ca.pem`, then a one-time download
from `/_cloudtyke/ca.pem`.

| Variable | Default | |
|---|---|---|
| `CLOUDTYKE_URL` | `https://localhost` | where the emulator listens (Docker: `https://management.cloudtyke.local`) |
| `CLOUDTYKE_DOMAIN` | `cloudtyke.local` | emulator base domain |
| `CLOUDTYKE_CA_FILE` | see above | emulator CA certificate |
| `CLOUDTYKE_ADMIN_TOKEN` | empty | sent to `/_cloudtyke/*` when the emulator requires it |
| `CLOUDTYKE_CLIENT_ID` / `_SECRET`, `CLOUDTYKE_READER_CLIENT_ID` / `_SECRET` | seeded values | identities to act as |

## Develop

```bash
npm run lint && npm run typecheck && npm run build
```

Pages live in `src/app/*/page.tsx` and render client views from `src/views/`. Shared UI is in `src/components/` and the
emulator client and proxy are in `src/lib/server/` and `src/app/api/`.
