#!/usr/bin/env bash
# Runs the hub-and-spoke lab end to end, like a learner would: for every
# stage apply the change, check there is no drift, and grade it with verify.sh.
#   usage: test/labs/hub-spoke.sh <tofu|terraform>
set -euo pipefail
BIN=${1:-tofu}
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/cloudtyke-lab.XXXXXX")
cp "$ROOT"/labs/hub-spoke/*.tf "$ROOT"/labs/hub-spoke/verify.sh "$work"/
cd "$work"
trap '"$BIN" destroy -auto-approve -input=false -no-color >/dev/null 2>&1 || true; rm -rf -- "${work:?}"' EXIT
"$BIN" init -input=false -no-color >/dev/null

stage() { # name, then -var flags
  local name=$1; shift
  echo "=== stage: $name"
  "$BIN" apply -auto-approve -input=false -no-color "$@" | grep -E "^Apply complete"
  "$BIN" plan -detailed-exitcode -input=false -no-color "$@" >/dev/null || { echo "FAILED: drift after $name"; exit 1; }
  ./verify.sh "$name"
}

stage none           -var connectivity=none
stage hub-only       -var connectivity=hub-only
stage direct-peering -var connectivity=direct-peering
stage hub-firewall   -var connectivity=hub-firewall
stage no-forwarding  -var connectivity=hub-firewall -var allow_forwarded_traffic=false
stage nsg            -var connectivity=hub-firewall -var block_https_between_spokes=true
echo "=== destroy"
"$BIN" destroy -auto-approve -input=false -no-color -var connectivity=hub-firewall -var block_https_between_spokes=true | grep -E "^Destroy complete"
trap 'rm -rf -- "${work:?}"' EXIT
echo "LAB PASSED"
