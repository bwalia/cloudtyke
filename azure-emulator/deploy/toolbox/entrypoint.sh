#!/usr/bin/env bash
# Trust the emulator CA (shared via the emulator data volume) and run the command.
set -euo pipefail
CA=${CLOUDTYKE_CA_FILE:-/cloudtyke-data/certs/ca.pem}
for i in $(seq 1 30); do [ -f "$CA" ] && break; sleep 1; done
if [ -f "$CA" ]; then
  cp "$CA" /usr/local/share/ca-certificates/cloudtyke-ca.crt
  update-ca-certificates >/dev/null 2>&1 || true
  export SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt
else
  echo "warning: CloudTyke CA not found at $CA" >&2
fi
exec "$@"
