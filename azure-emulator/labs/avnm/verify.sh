#!/usr/bin/env bash
# Grades a stage of the Virtual Network Manager lab: shows what the network
# manager has deployed per region, then traces packets through the emulator's
# routing model (peerings, connected groups, security admin rules, NSGs).
#
#   ./verify.sh <hub-spoke|regional-mesh|global-mesh|security> [-v]
#
# Run inside the toolbox (make shell) after `tofu apply` for that stage.
set -uo pipefail
STAGE=${1:-}
VERBOSE=${2:-}
API=${CLOUDTYKE_API:-https://management.cloudtyke.local}
AUTH=()
[ -n "${CLOUDTYKE_ADMIN_TOKEN:-}" ] && AUTH=(-H "X-CloudTyke-Admin-Token: $CLOUDTYKE_ADMIN_TOKEN")

UKS1=10.1.1.4; UKS2=10.2.1.4; WEU1=10.101.1.4; WEU2=10.102.1.4
HUB_UKS=10.0.1.4; HUB_WEU=10.100.1.4; BASTION_UKS=10.0.2.4

case "$STAGE" in
  hub-spoke) CASES="uks spoke 1 -> uks hub|$UKS1|$HUB_UKS|443|Delivered
weu spoke 1 -> weu hub|$WEU1|$HUB_WEU|443|Delivered
uks spoke 1 -> uks spoke 2 (no spoke-to-spoke yet)|$UKS1|$UKS2|443|Dropped
uks spoke 1 -> weu spoke 1|$UKS1|$WEU1|443|Dropped
uks spoke 1 -> weu hub (other region's hub)|$UKS1|$HUB_WEU|443|Dropped" ;;
  regional-mesh) CASES="uks spoke 1 -> uks spoke 2 (connected group)|$UKS1|$UKS2|443|Delivered
weu spoke 1 -> weu spoke 2 (connected group)|$WEU1|$WEU2|443|Delivered
uks spoke 1 -> uks hub (still hub-and-spoke)|$UKS1|$HUB_UKS|443|Delivered
uks spoke 1 -> weu spoke 1 (mesh is regional)|$UKS1|$WEU1|443|Dropped" ;;
  global-mesh) CASES="uks spoke 1 -> weu spoke 1 (global mesh)|$UKS1|$WEU1|443|Delivered
weu spoke 2 -> uks spoke 2 (global mesh)|$WEU2|$UKS2|443|Delivered
uks spoke 1 -> weu hub (hubs are not in the mesh)|$UKS1|$HUB_WEU|443|Dropped
uks bastion -> uks spoke 1 SSH (team NSG denies)|$BASTION_UKS|$UKS1|22|Blocked" ;;
  security) CASES="uks spoke 2 -> uks spoke 1 SSH (admin Deny)|$UKS2|$UKS1|22|Blocked
weu spoke 1 -> uks spoke 2 RDP (admin Deny)|$WEU1|$UKS2|3389|Blocked
uks spoke 2 -> uks spoke 1 HTTPS (not a management port)|$UKS2|$UKS1|443|Delivered
uks bastion -> uks spoke 1 SSH (AlwaysAllow beats the team NSG)|$BASTION_UKS|$UKS1|22|Delivered" ;;
  *) echo "usage: $0 <hub-spoke|regional-mesh|global-mesh|security> [-v]"; exit 2 ;;
esac

echo "Stage: $STAGE"
if [ -n "${ARM_CLIENT_ID:-}" ]; then
  T=$(curl -fsS "https://login.cloudtyke.local/$ARM_TENANT_ID/oauth2/v2.0/token" -d grant_type=client_credentials \
    -d client_id="$ARM_CLIENT_ID" -d client_secret="$ARM_CLIENT_SECRET" -d scope="$API/.default" | jq -r .access_token)
  NM="/subscriptions/$ARM_SUBSCRIPTION_ID/resourceGroups/rg-lab-avnm/providers/Microsoft.Network/networkManagers/avnm-contoso"
  echo
  echo "Network manager deployments:"
  curl -fsS -X POST -H "Authorization: Bearer $T" -H 'content-type: application/json' "$API$NM/listDeploymentStatus?api-version=2024-05-01" -d '{}' |
    jq -r '.value[] | "  \(.region)\t\(.deploymentType)\t\(.deploymentStatus)\t\(.configurationIds | map(split("/") | last) | join(", "))"'
fi
echo
pass=0; fail=0
while IFS='|' read -r name from to port want; do
  out=$(curl -fsS "${AUTH[@]}" "$API/_cloudtyke/network/trace?from=$from&to=$to&port=$port") || { echo "cannot reach $API (run inside the toolbox: make shell)"; exit 2; }
  got=$(echo "$out" | jq -r .result)
  if [ "$got" = "$want" ]; then mark="PASS"; pass=$((pass+1)); else mark="FAIL"; fail=$((fail+1)); fi
  printf '%s  %-60s expect %-9s got %s\n' "$mark" "$name" "$want" "$got"
  if [ "$VERBOSE" = "-v" ] || [ "$mark" = "FAIL" ]; then
    echo "$out" | jq -r '.hops[] | "        \(.step). [\(.verdict)] \(.action): \(.detail)"'
  fi
done <<< "$CASES"
echo
echo "$pass passed, $fail failed"
[ $fail -eq 0 ]
