#!/usr/bin/env bash
# Runs each example through the full IaC lifecycle against a running emulator:
#   init -> validate -> apply -> plan (must be empty: idempotency/drift check) -> destroy
# Fails on any error or on drift.
#   usage: run.sh <tofu|terraform> [example ...]
set -uo pipefail
BIN=${1:?usage: run.sh <tofu|terraform> [example ...]}
shift
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
EXAMPLES=("$@")
if [ ${#EXAMPLES[@]} -eq 0 ]; then
  EXAMPLES=(quickstart entra)
fi

fail=0
for ex in "${EXAMPLES[@]}"; do
  src="$ROOT/examples/terraform/$ex"
  work=$(mktemp -d "${TMPDIR:-/tmp}/cloudtyke-e2e.XXXXXX")
  # copy only configuration, never local state
  find "$src" -maxdepth 1 -type f -name '*.tf' -exec cp {} "$work"/ \;
  echo "=== [$BIN] $ex"
  (
    cd "$work" || exit 1
    set -e
    echo "--- init";     "$BIN" init -input=false -no-color >/dev/null
    echo "--- validate"; "$BIN" validate -no-color
    echo "--- apply";    "$BIN" apply -auto-approve -input=false -no-color
    echo "--- plan (drift check)"
    set +e
    "$BIN" plan -detailed-exitcode -input=false -no-color
    rc=$?
    set -e
    if [ $rc -eq 2 ]; then echo "FAILED: drift detected after apply in $ex"; exit 1; fi
    if [ $rc -ne 0 ]; then echo "FAILED: plan errored in $ex"; exit 1; fi
    echo "--- destroy";  "$BIN" destroy -auto-approve -input=false -no-color
  )
  if [ $? -ne 0 ]; then
    fail=1
    echo "!!! $ex failed; attempting cleanup"
    (cd "$work" && "$BIN" destroy -auto-approve -input=false -no-color >/dev/null 2>&1) || true
  fi
  rm -rf -- "${work:?}"
done
if [ $fail -ne 0 ]; then echo "E2E FAILED"; else echo "E2E PASSED"; fi
exit $fail
