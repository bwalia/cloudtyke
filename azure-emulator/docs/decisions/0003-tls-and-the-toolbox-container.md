# ADR 0003: run IaC tools from a Linux toolbox container by default

* Status: accepted (phase 0)

## Context

The Terraform providers require HTTPS. They are Go binaries, and on macOS Go uses the platform verifier, which
**ignores `SSL_CERT_FILE`**. Asking every user to add a CA to their login keychain is invasive, and interview machines
may not allow it. Per-resource data-plane host names (`<vault>.vault.cloudtyke.local`) also need wildcard DNS, which
`/etc/hosts` cannot express.

## Decision

Ship a `toolbox` compose service (profile `tools`) with OpenTofu, Terraform, curl and jq. It joins the emulator network,
uses the CoreDNS sidecar, and installs the CA from the shared data volume at start-up. `make shell` and `make tf-e2e`
use it. Host usage remains supported through `tyke azure trust --install` and `tyke azure hosts`.

## Consequences

* **Works the same on macOS, Linux, Windows (WSL), Codespaces and CI.**
* **Provider plugins are cached** in a named volume (`plugin-cache`).
* **The workspace is mounted at `/work`**, so local paths in examples must be relative.
