#!/usr/bin/env bash
# CloudTyke Arena: PASS / FAIL checks for the runnable stadium.
#
#   bash landing-zone-stadium/verify.sh        # after scripts/up.sh
#
# Checks:
#   1. the hierarchy has sub-events under ct-landingzones
#   2. the safety code is assigned at the ct-landingzones management group
#   3. today's stadium matches the code (a plan with no changes)
#   4. swapping one event's kit (derby: football -> concert by default) only
#      changes that event's pitch: never the gates, the seats, the control
#      room or the safety code
#
# Change the swap with SWAP_EVENT=summer SWAP_TO=concert. Use TOOL=terraform
# for HashiCorp Terraform. Nothing is applied: the swap is a plan only.
set -uo pipefail
TOOL=${TOOL:-tofu}
SWAP_EVENT=${SWAP_EVENT:-derby}
SWAP_TO=${SWAP_TO:-concert}

# ------------------------------------------------------------------ run inside the toolbox
if [ ! -d /stadium/terraform ]; then
  HERE=$(cd "$(dirname "$0")" && pwd)
  cd "$HERE/../azure-emulator" || exit 2
  COMPOSE=(docker compose -f docker-compose.yml)
  [ -f .k3s-mode ] && COMPOSE+=(-f docker-compose.k3s.yml)
  COMPOSE+=(-f ../landing-zone-stadium/docker-compose.stadium.yml)
  exec "${COMPOSE[@]}" --profile tools run --rm -T \
    -e TOOL="$TOOL" -e SWAP_EVENT="$SWAP_EVENT" -e SWAP_TO="$SWAP_TO" \
    -e CLOUDTYKE_ADMIN_TOKEN="${CLOUDTYKE_ADMIN_TOKEN:-}" \
    -w /stadium/terraform toolbox bash /stadium/verify.sh
fi

cd /stadium/terraform || exit 2
DOMAIN=${CLOUDTYKE_DOMAIN:-cloudtyke.local}
ARM=https://management.$DOMAIN
EVENTS_SUB=5b5c0000-0000-4000-8000-000000000004
MG=ct-landingzones
pass=0
fail=0
ok() { printf 'PASS  %s\n' "$1"; pass=$((pass + 1)); }
bad() { printf 'FAIL  %s\n' "$1"; [ -n "${2:-}" ] && printf '        %s\n' "$2"; fail=$((fail + 1)); }

token=$(curl -fsS "https://login.$DOMAIN/$ARM_TENANT_ID/oauth2/v2.0/token" \
  -d grant_type=client_credentials -d client_id="$ARM_CLIENT_ID" \
  --data-urlencode client_secret="$ARM_CLIENT_SECRET" \
  --data-urlencode scope="$ARM/.default" | jq -r .access_token)
if [ -z "$token" ] || [ "$token" = null ]; then
  echo "cannot get a token from the emulator: is it running? (bash scripts/up.sh)"
  exit 2
fi
arm() { curl -fsS -H "Authorization: Bearer $token" "$ARM$1"; }

echo "CloudTyke Arena checks"
echo

# 1. hierarchy ----------------------------------------------------------------
children=$(arm "/providers/Microsoft.Management/managementGroups/$MG?api-version=2021-04-01&\$expand=children" |
  jq -r '.properties.children[]? | "\(.name) \(.displayName)"')
if echo "$children" | grep -q "^$EVENTS_SUB "; then
  ok "sub-events ($EVENTS_SUB) sits under the $MG management group"
else
  bad "sub-events is not under $MG" "children found: ${children:-none}. Did up.sh re-seed with seed.yaml?"
fi

# 2. safety code at the management group ---------------------------------------
scope=$(arm "/providers/Microsoft.Management/managementGroups/$MG/providers/Microsoft.Authorization/policyAssignments/arena-safety-code?api-version=2023-04-01" 2>/dev/null |
  jq -r '.properties.scope // empty')
if [ "${scope##*/}" = "$MG" ]; then
  ok "the safety code (arena-safety-code) is assigned at the $MG management group"
else
  bad "the safety code is not assigned at $MG" "got scope: ${scope:-not found}"
fi

# Plans need providers; init is quick when they are cached.
$TOOL init -input=false -no-color >/dev/null 2>&1 || { bad "$TOOL init failed"; echo; echo "$pass passed, $fail failed"; exit 1; }
work=$(mktemp -d)

# helper: list "actions address" for every real change in a saved plan
changes() {
  $TOOL show -json "$1" | jq -r '.resource_changes[]?
    | select((.change.actions | index("no-op")) == null and (.change.actions | index("read")) == null)
    | "\(.change.actions | join("+"))  \(.address)"'
}

# 3. no drift -----------------------------------------------------------------
if $TOOL plan -input=false -no-color -out="$work/now.tfplan" >"$work/now.log" 2>&1; then
  drift=$(changes "$work/now.tfplan")
  if [ -z "$drift" ]; then
    ok "today's stadium matches the code (plan: no changes)"
  else
    bad "the plan for today's stadium is not empty (run scripts/up.sh first)" "$(echo "$drift" | head -5)"
  fi
else
  bad "$TOOL plan failed" "$(grep -m3 -A3 '^Error' "$work/now.log")"
fi

# 4. swap one event's kit -----------------------------------------------------
current=$(echo 'jsonencode(var.events)' | $TOOL console 2>/dev/null | jq -r 'fromjson? // .' 2>/dev/null)
was=$(echo "$current" | jq -r --arg e "$SWAP_EVENT" '.[$e].kit // empty' 2>/dev/null)
if [ -z "$was" ]; then
  bad "cannot read var.events (or event '$SWAP_EVENT' does not exist)"
else
  echo "$current" | jq --arg e "$SWAP_EVENT" --arg k "$SWAP_TO" '{events: (.[$e].kit = $k)}' >"$work/swap.tfvars.json"
  echo "      swapping $SWAP_EVENT: $was -> $SWAP_TO (plan only, nothing is applied)"
  if $TOOL plan -input=false -no-color -var-file="$work/swap.tfvars.json" -out="$work/swap.tfplan" >"$work/swap.log" 2>&1; then
    swap=$(changes "$work/swap.tfplan")
    pitch=0
    wrong=""
    while read -r action address; do
      [ -z "$address" ] && continue
      case "$address" in
        *.pitch*"[\"$SWAP_EVENT\"]"*) pitch=$((pitch + 1)) ;;
        *.pitch*) wrong+="another event's pitch: $action $address"$'\n' ;;
        *.seats*) wrong+="seats: $action $address"$'\n' ;;
        *.gates* | *.kit_room*) wrong+="gates: $action $address"$'\n' ;;
        *.control_room*) wrong+="control room: $action $address"$'\n' ;;
        *.safety_code*) wrong+="safety code: $action $address"$'\n' ;;
        *) wrong+="other: $action $address"$'\n' ;;
      esac
    done <<<"$swap"
    if [ "$pitch" -gt 0 ] && [ -z "$wrong" ]; then
      ok "swapping $SWAP_EVENT to $SWAP_TO changes only its pitch ($pitch pitch changes)"
      echo "$swap" | sed 's/^/        /'
    elif [ "$pitch" -eq 0 ] && [ -z "$wrong" ]; then
      bad "swapping $SWAP_EVENT to $SWAP_TO changed nothing" "is $SWAP_EVENT already a $SWAP_TO?"
    else
      bad "swapping $SWAP_EVENT to $SWAP_TO touched parts that must never change" "$(printf '%s' "$wrong" | sed '2,$s/^/        /')"
    fi
  else
    bad "$TOOL plan for the swap failed" "$(grep -m3 -A3 '^Error' "$work/swap.log")"
  fi
fi

rm -rf "$work"
echo
echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
