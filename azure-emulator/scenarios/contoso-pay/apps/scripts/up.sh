#!/usr/bin/env bash
# Contoso Pay live demo, end to end:
#   1. start the CloudTyke emulator
#   2. deploy the Terraform scenario (teams, policies, network, AKS, Key Vault,
#      Storage, PostgreSQL, Cosmos DB) with OpenTofu, or TOOL=terraform
#   3. write apps/.env from the outputs and Key Vault
#   4. build and start the apps and the OpenTelemetry stack
# Re-running is safe: Terraform converges and compose only recreates what changed.
set -euo pipefail
APPS=$(cd "$(dirname "$0")/.." && pwd)
EMU=$(cd "$APPS/../../.." && pwd)
TOOL=${TOOL:-tofu}
TF=/work/scenarios/contoso-pay/terraform

cd "$EMU"
# Keep the k3s backend if it is on (make up-k3s), so AKS stays a real cluster.
if [ -f .k3s-mode ]; then
  COMPOSE=(docker compose -f docker-compose.yml -f docker-compose.k3s.yml); UP=up-k3s
else
  COMPOSE=(docker compose); UP=up
fi
echo "==> starting the emulator ($UP)"
make "$UP" >/dev/null
"${COMPOSE[@]}" --profile tools build toolbox >/dev/null

echo "==> deploying the Contoso Pay estate with $TOOL (first run takes a few minutes)"
log=$(mktemp "${TMPDIR:-/tmp}/contoso-pay-apply.XXXXXX")
if ! "${COMPOSE[@]}" run --rm -T -w "$TF" toolbox sh -c "$TOOL init -input=false -no-color >/dev/null && $TOOL apply -auto-approve -input=false -no-color" >"$log" 2>&1; then
  grep -A12 '^Error' "$log" | head -80
  echo "apply failed; full log: $log" >&2
  exit 1
fi
grep -E '^Apply complete' "$log" || tail -3 "$log"
rm -f "$log"

echo "==> reading outputs and Key Vault secrets"
grafana_pw=$(grep -s '^GRAFANA_ADMIN_PASSWORD=' "$APPS/.env" | cut -d= -f2- || true)
[ -n "$grafana_pw" ] || grafana_pw=$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 20)
umask 077
{
  "${COMPOSE[@]}" run --rm -T -w "$TF" -e BIN="$TOOL" toolbox bash -s <"$APPS/scripts/render-env.sh"
  echo "GRAFANA_ADMIN_PASSWORD=$grafana_pw"
} >"$APPS/.env.new"
mv "$APPS/.env.new" "$APPS/.env"

echo "==> starting the apps and the OpenTelemetry stack"
cd "$APPS"
docker compose up -d --build --remove-orphans

cat <<EOF

Contoso Pay is running (all names and data are invented):
  Northwind Airways storefront   http://localhost:${STOREFRONT_PORT:-4701}
  Processor console              http://localhost:${PORTAL_PORT:-4703}
  Payment gateway API            http://localhost:${GATEWAY_PORT:-4702}
  Grafana                        http://localhost:${GRAFANA_PORT:-4730}   (admin / see GRAFANA_ADMIN_PASSWORD in apps/.env)
  Jaeger                         http://localhost:${JAEGER_PORT:-4731}
  Prometheus                     http://localhost:${PROMETHEUS_PORT:-4732}
EOF
