#!/usr/bin/env bash
# Runs the Virtual Network Manager lab end to end: every stage is applied,
# checked for drift and graded with verify.sh.
#   usage: test/labs/avnm.sh <tofu|terraform>
set -euo pipefail
BIN=${1:-tofu}
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/cloudtyke-lab.XXXXXX")
cp "$ROOT"/labs/avnm/*.tf "$ROOT"/labs/avnm/verify.sh "$work"/
cd "$work"
trap '"$BIN" destroy -auto-approve -input=false -no-color >/dev/null 2>&1 || true; rm -rf -- "${work:?}"' EXIT
"$BIN" init -input=false -no-color >/dev/null

stage() {
  local name=$1; shift
  echo "=== stage: $name"
  "$BIN" apply -auto-approve -input=false -no-color "$@" | grep -E "^Apply complete"
  "$BIN" plan -detailed-exitcode -input=false -no-color "$@" >/dev/null || { echo "FAILED: drift after $name"; "$BIN" plan -input=false -no-color "$@" | grep -E "^\s+[~+-] |will be" | head -20; exit 1; }
  ./verify.sh "$name"
}

stage hub-spoke     -var stage=hub-spoke
stage regional-mesh -var stage=regional-mesh
stage global-mesh   -var stage=global-mesh
stage security      -var stage=global-mesh -var security_admin=true
echo "=== destroy"
"$BIN" destroy -auto-approve -input=false -no-color -var stage=global-mesh -var security_admin=true | grep -E "^Destroy complete"
trap 'rm -rf -- "${work:?}"' EXIT
echo "LAB PASSED"
