# Compatibility

## Tested tool and provider versions

| Tool / provider | Version tested | How |
|---|---|---|
| OpenTofu | 1.13.9 | `make tf-e2e` (toolbox image) |
| Terraform | 1.17.2 | `make tf-e2e-terraform` (toolbox image) |
| `hashicorp/azurerm` | 5.14.0 (`~> 5.14`) | examples `quickstart` |
| `hashicorp/azuread` | 3.10.0 (`~> 3.10`) | examples `quickstart`, `entra` |
| `hashicorp/random` | 3.7.x | example `entra` |
| `Azure/azapi` | not yet tested (2.13.0 is current); planned for phase 2 | — |
| go-azure-sdk (inside the providers) | `sdk v0.20261106.1110203` (azurerm), `v0.20260212.1143955` (azuread) | source review of `environments`, `auth`, `client/resourcemanager` |

CI runs every example through `init → validate → apply → plan (must be empty) → destroy` with both OpenTofu and
Terraform against a freshly started emulator. If a provider release needs an API shape we don't emulate, **the
emulator is fixed, not the example.**

## Provider configuration

No provider code changes are needed: only `metadata_host` and credentials change.

```hcl
provider "azurerm" {
  features {}
  # metadata_host   = "management.cloudtyke.local"       # or ARM_METADATA_HOSTNAME
  # tenant_id       = "7c10d7e4-0000-4000-8000-000000000001"
  # subscription_id = "5b5c0000-0000-4000-8000-000000000003"
  # client_id       = "a99c0000-0000-4000-8000-000000000001"
  # client_secret   = "cloudtyke-bootstrap-secret"
}

provider "azuread" {} # reads the same ARM_* variables, including ARM_METADATA_HOSTNAME
```

Do **not** set `environment` together with `metadata_host`.

## Environment variables

| Variable | Value (default seed) |
|---|---|
| `ARM_METADATA_HOSTNAME` | `management.cloudtyke.local` |
| `ARM_TENANT_ID` | `7c10d7e4-0000-4000-8000-000000000001` |
| `ARM_SUBSCRIPTION_ID` | `5b5c0000-0000-4000-8000-000000000003` (sub-sandbox) |
| `ARM_CLIENT_ID` | `a99c0000-0000-4000-8000-000000000001` (cloudtyke-bootstrap, Owner at `/`) |
| `ARM_CLIENT_SECRET` | `cloudtyke-bootstrap-secret` |
| `SSL_CERT_FILE` | path to `certs/ca.pem` (Linux; ignored by Go on macOS) |

`tyke azure env` prints all of these. A second principal, `cloudtyke-reader` (`a99c0000-0000-4000-8000-000000000002` /
`cloudtyke-reader-secret`), holds only Reader on sub-sandbox and Graph `Directory.Read.All`, so you can watch RBAC
enforcement in action.

## Trusting the CA

| Client | How |
|---|---|
| Terraform / OpenTofu on macOS | `tyke azure trust --install` (login keychain) **or** use `make shell` (Linux toolbox) |
| Terraform / OpenTofu on Linux | `export SSL_CERT_FILE=$PWD/certs/ca.pem` or the OS trust store |
| Windows | `tyke azure trust --install` (current-user Root store) |
| curl | `--cacert certs/ca.pem` |
| Python SDK / az CLI | `export REQUESTS_CA_BUNDLE=$PWD/certs/ca.pem` |
| Node SDKs | `export NODE_EXTRA_CA_CERTS=$PWD/certs/ca.pem` |

## Name resolution

Inside the compose network, the CoreDNS sidecar answers every `*.cloudtyke.local` name, including per-resource
data-plane names such as `myvault.vault.cloudtyke.local`. On the host, add this to `/etc/hosts` (`tyke azure hosts`):

```
127.0.0.1 cloudtyke.local management.cloudtyke.local login.cloudtyke.local graph.cloudtyke.local
```

Per-resource data-plane names (Key Vault, Storage, ACR) need either one line each in `/etc/hosts`, or pointing your resolver at
the sidecar. Using the toolbox avoids this entirely.
