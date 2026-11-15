# hello-web: the smallest end-to-end CloudTyke demo

One web page on AKS that proves the whole chain works:

```
browser ──► localhost:4740 ──► AKS (k3s) pod ──► Entra ID: service-account token → access token (workload identity)
                                              ├─► Key Vault: read secret "greeting"
                                              └─► Blob storage: read + write visits/count.txt
```

16 resources: a resource group, a policy definition and assignment, a Key Vault and its secret, a storage account and
container, an AKS cluster, a user-assigned identity with a federated credential, and five role assignments.

```bash
cd azure-emulator
make hello-web          # ~5 minutes the first time; opens nothing, prints the URL
open http://localhost:4740
make hello-web-down     # remove it all
```

### From other machines on your network, by name

```bash
HELLO_BIND=0.0.0.0 HELLO_PORT=80 make hello-web   # http://hello.cloudtyke.local, behind a password
```

On each machine that should open it, add one line to its hosts file (`/etc/hosts`, or
`C:\Windows\System32\drivers\etc\hosts` on Windows), using this machine's LAN IP (the script prints it):

```
192.168.1.177  hello.cloudtyke.local
```

LAN mode replaces the localhost forwarder with a Caddy proxy that requires basic auth. The user (`intern`, or
`HELLO_USER`) and a generated password (or `HELLO_PASSWORD`) are printed and saved in
`reports/.hello-web-lan-credentials`. Only the app is exposed: the emulator stays on localhost, so don't map
`vault.`, `management.` or `login.cloudtyke.local` to this machine on other computers. The browser never calls them; the
pod does. Notes: plain HTTP (the password crosses your LAN unencrypted, fine for a demo network); `HELLO_HOSTNAME`
changes the name; and macOS may resolve `.local` names via Bonjour first, so a hosts entry can take a few seconds
on the first lookup.

#### Troubleshooting: the name doesn't open on another machine

First test by IP from that machine: `curl -s -o /dev/null -w '%{http_code}\n' http://192.168.1.177/`.

| Result | Meaning | Fix |
|---|---|---|
| `401` by IP, but the name fails | The app is reachable, but the name doesn't resolve on that machine | Add the hosts line (above). On macOS, check it with `dscacheutil -q host -a name hello.cloudtyke.local` (it should print the IP), then flush the cache: `sudo dscacheutil -flushcache; sudo killall -HUP mDNSResponder` |
| `curl` gets `401` by name, but the browser fails | The browser is switching to `https://`, and nothing serves HTTPS | Type the full `http://hello.cloudtyke.local`. If needed, turn off *Always use secure connections* (Chrome) or *HTTPS-Only Mode* (Firefox) for this site |
| The name resolves, but only sometimes | macOS asks Bonjour (mDNS) about `.local` names before it reads the hosts file | Use another suffix: `HELLO_HOSTNAME=hello.cloudtyke.test`, plus a hosts line with that name. Any name works, because the proxy doesn't check it |
| Timeout or connection refused, even by IP | The request never reaches the app | On the host: check that `docker ps` shows `hello-web-proxy` on `0.0.0.0:80`, that the macOS firewall allows incoming connections, and that both machines are on the same network (guest Wi-Fi often isolates clients) |
| `curl: (6)` with `--resolve` working | The same as the first row: only the name lookup fails | Use `curl --resolve hello.cloudtyke.local:80:192.168.1.177 -u intern:<password> http://hello.cloudtyke.local/` to prove the server side works before you edit hosts files |

On this machine itself, use `127.0.0.1  hello.cloudtyke.local` in `/etc/hosts`, or just open http://localhost.

## Where things run

| Piece | Runs where | Why |
|---|---|---|
| CloudTyke (Azure, Entra ID, Key Vault, Storage, AKS APIs) | `emulator` container | the fake Azure |
| `tofu` / `terraform` | the **toolbox** container (`docker compose run toolbox`) | it already trusts the emulator's CA and resolves `*.cloudtyke.local`; nothing to install on your machine |
| AKS cluster | a `rancher/k3s` container per cluster, started by the emulator (`make up-k3s`) | a real Kubernetes API behind the AKS resource |
| The app | pods in that k3s cluster | like on real AKS |
| http://localhost:4740 | a tiny `socat` container (`hello-web-proxy`) publishing the app's NodePort on `127.0.0.1` (or Caddy with a password in LAN mode) | reachable from your browser; from your network only with `HELLO_BIND` and a password |

You do not need `tofu`, `kubectl` or Node on your machine, only Docker. To run Terraform from your own machine
instead, see *Using your own host tools* in the [emulator README](../../../README.md): trust the CA, add the host
names, and `eval "$(bin/tyke azure env)"`.

## Things to try

1. **Change the greeting.** `TF_VAR_greeting="Hi interns" make hello-web`. Terraform changes one resource (the Key
   Vault secret) and the running page shows it on the next refresh, without a redeploy.
2. **Hit the guardrail.** `TF_VAR_allow_public_blob=true make hello-web`. The apply fails with
   `RequestDisallowedByPolicy`, quoting the assignment's non-compliance message. Read `azurerm_policy_definition` in
   `main.tf` to see why.
3. **Break the identity.** Delete the `app_kv` role assignment from `main.tf` and apply. The page turns red with
   Key Vault's `403 ForbiddenByRbac`. Put it back.
4. **Look inside the cluster.** `make shell`, then
   `kubectl --kubeconfig /work/reports/.hello-web-kubeconfig get pods -o wide`
   (local accounts are disabled: that kubeconfig carries an Entra token, and Azure RBAC authorizes each request).
5. **Watch it from a pipeline.** The [hello-web workflow](../../../../.github/workflows/hello-web.yml) runs plan,
   apply, a change, the guardrail and destroy in GitHub Actions, and writes each step to the run summary.

If the emulator also runs the Contoso Pay scenario, its PCI baseline policy applies to this subscription too. That's
why `hello-web` tags its resource group and turns on Key Vault purge protection: a team inherits the
company's guardrails.
