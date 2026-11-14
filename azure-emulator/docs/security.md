# Security and threat model

CloudTyke is a **local development and assessment tool**. It deliberately ships well-known credentials and a local CA,
so it must never be reachable from untrusted networks.

## Defaults

* **Bound to 127.0.0.1 only** (`127.0.0.1:443:443` in `docker-compose.yml`). Changing that binding exposes an
  Owner-at-root credential to your network.
* **Non-root, distroless image.** Port 443 is bound through `net.ipv4.ip_unprivileged_port_start=0` inside the container's
  own network namespace, not with extra capabilities.
* **No Docker socket in phases 0 and 1.** Later phases (VMs, AKS) mount `/var/run/docker.sock` **only when you opt in**
  through a separate compose profile. Anything with the Docker socket is root-equivalent on the host. The emulator logs a
  prominent warning when it is enabled.
* **No outbound network needed**, except when verifying federated assertions from external OIDC issuers (GitHub, GitLab),
  which fetches their JWKS. Disable that with `CLOUDTYKE_FEDERATION_INSECURE=true`, which also skips the signature check;
  use it offline only.

## Secrets

* **Seeded credentials** (`cloudtyke-bootstrap-secret`, `cloudtyke-reader-secret`, the demo user password) are public by
  design and documented. Override them with your own `seed.yaml` (`CLOUDTYKE_SEED_FILE`) for shared environments, such as
  interview machines.
* **Client secrets created through Graph** are stored as SHA-256 hashes. `secretText` is returned exactly once, by
  `addPassword`, as in Entra.
* **Token signing key** (RSA 2048) lives in `/data/keys`; with `CLOUDTYKE_EPHEMERAL=true` it exists only in memory.
  Tokens are valid for one hour and are accepted only by this emulator instance.
* **Never commit anything from a real subscription** (golden files are built from the public OpenAPI specs).

## Local CA

* **Generated on first start** into the data volume (`/data/certs`): an ECDSA P-256 CA valid for 10 years
  (`MaxPathLen 0`, so it cannot sign intermediate CAs), and a server certificate valid for 397 days.
* **Trusting it** lets the emulator present a certificate for any name it signs. It only signs `*.cloudtyke.local`
  names and localhost, but the CA key could sign anything, so treat `/data/certs/ca-key.pem` like any other private key.
  Remove the trust when you are done (`security delete-certificate -c "CloudTyke Local CA…"` on macOS, or delete
  `/usr/local/share/ca-certificates/cloudtyke-ca.crt` and run `update-ca-certificates` on Linux).
* **`tyke azure trust` downloads the CA without verification** from `https://localhost/_cloudtyke/ca.pem` because it can't
  verify a CA it doesn't have yet. This trust-on-first-use step happens over loopback only. `make up` instead copies the file
  straight out of the container with `docker compose cp`.

## Admin API

* **`/_cloudtyke/*` can reset, replace or export all state.** It is unauthenticated by default because the endpoint is
  loopback-only. Set `CLOUDTYKE_ADMIN_TOKEN` to require an `X-CloudTyke-Admin-Token` header, for example on shared
  interview hosts where candidates must not reset or read state. Health stays open.
* **The request ring buffer** (`/_cloudtyke/requests`) records method, host, path, api-version, status and caller. It
  never records request bodies or tokens.

## Interview use

* **Separate grading material.** Run candidates with a custom seed, `CLOUDTYKE_ADMIN_TOKEN` set, and the `solutions/`
  directory excluded from their copy.
* **Fair verification.** Verifiers query state through the admin export, not through candidate credentials, so candidates can't
  tamper with grading by changing RBAC.
