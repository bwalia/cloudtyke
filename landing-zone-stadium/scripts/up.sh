#!/usr/bin/env bash
# CloudTyke Arena, end to end:
#   1. start the CloudTyke emulator with the stadium seed (adds sub-events)
#   2. re-seed the emulator so sub-events appears under ct-landingzones
#   3. build the stadium with OpenTofu (or TOOL=terraform): gates, control
#      room, safety code, seats for every event and each event's pitch
#
# Heads up: step 2 calls POST /_cloudtyke/reset, which WIPES everything in the
# emulator (for example a Contoso Pay deployment) and re-seeds it. To keep
# what is there and only add sub-events, run with KEEP_STATE=1.
#
# Re-running is safe: Terraform converges to the same stadium.
set -euo pipefail
HERE=$(cd "$(dirname "$0")/.." && pwd)
EMU=$(cd "$HERE/../azure-emulator" && pwd)
TOOL=${TOOL:-tofu}
TF=/stadium/terraform

cd "$EMU"
# Keep the k3s backend if it is on (make up-k3s), so AKS is a real cluster.
COMPOSE=(docker compose -f docker-compose.yml)
[ -f .k3s-mode ] && COMPOSE+=(-f docker-compose.k3s.yml)
COMPOSE+=(-f ../landing-zone-stadium/docker-compose.stadium.yml)

echo "==> starting the emulator with the stadium seed"
"${COMPOSE[@]}" up -d --build --wait >/dev/null
mkdir -p certs && "${COMPOSE[@]}" cp emulator:/data/certs/ca.pem certs/ca.pem >/dev/null
"${COMPOSE[@]}" --profile tools build toolbox >/dev/null

if [ "${KEEP_STATE:-0}" = "1" ]; then
  action=seed
  echo "==> adding the stadium seed (KEEP_STATE=1: existing resources are kept)"
else
  action=reset
  echo "==> re-seeding the emulator (POST /_cloudtyke/reset: wipes old state)"
fi
"${COMPOSE[@]}" --profile tools run --rm -T \
  -e CLOUDTYKE_ADMIN_TOKEN="${CLOUDTYKE_ADMIN_TOKEN:-}" -e CLOUDTYKE_DOMAIN="${CLOUDTYKE_DOMAIN:-cloudtyke.local}" toolbox sh -c \
  "curl -fsS -X POST -H \"X-CloudTyke-Admin-Token: \${CLOUDTYKE_ADMIN_TOKEN:-}\" https://management.\${CLOUDTYKE_DOMAIN:-cloudtyke.local}/_cloudtyke/$action" \
  >/dev/null
# A reset starts from an empty store, so old Terraform state no longer matches.
[ "$action" = reset ] && rm -f "$HERE/terraform/terraform.tfstate" "$HERE/terraform/terraform.tfstate.backup"

echo "==> building the stadium with $TOOL (first run downloads providers)"
log=$(mktemp "${TMPDIR:-/tmp}/arena-apply.XXXXXX")
if ! "${COMPOSE[@]}" --profile tools run --rm -T -w "$TF" toolbox sh -c \
  "$TOOL init -input=false -no-color >/dev/null && $TOOL apply -auto-approve -input=false -no-color" >"$log" 2>&1; then
  grep -A12 '^Error' "$log" | head -80
  echo "apply failed; full log: $log" >&2
  exit 1
fi
grep -E '^Apply complete' "$log" || tail -3 "$log"
rm -f "$log"

cat <<EOF

CloudTyke Arena is built (all names are invented):
  Gates          sub-connectivity   rg-ct-gates-uks (hub VNet, firewall subnet + IP, Bastion subnet, DNS, kit room)
  Control room   sub-management     rg-ct-control-room-uks (Log Analytics)
  Safety code    ct-landingzones    policy assignment arena-safety-code
  Events         sub-events         rg-ct-<event>-seats-uks + rg-ct-<event>-pitch-uks

Next:
  bash $HERE/verify.sh                 # PASS / FAIL checks, including a kit swap
  cd $EMU && make dashboard            # see it on http://localhost:4310
EOF
