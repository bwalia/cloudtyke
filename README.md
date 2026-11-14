# CloudTyke

**CloudTyke: a small cloud for big mistakes.** This repository contains the
[CloudTyke Azure emulator](azure-emulator/README.md), a Docker-based local emulator for Azure APIs (ARM, Entra ID and a
Microsoft Graph subset). Point Terraform or OpenTofu at it to build "Azure" infrastructure on your own machine with no
subscription, no bill and no blast radius.

> **Not affiliated with or endorsed by Microsoft.** Emulation is partial and for learning and testing only.

## Quickstart

```bash
cd azure-emulator
make up      # start the emulator
make shell   # toolbox shell with tofu + terraform, already configured
```

See [azure-emulator/README.md](azure-emulator/README.md) for the full guide, coverage and configuration reference.
