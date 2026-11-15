#!/usr/bin/env bash
# Grades a stage of the hub-and-spoke lab by tracing packets through the
# emulator's routing model (effective routes, peering rules, NSGs).
#
#   ./verify.sh <stage> [-v]
#   stages: none | hub-only | direct-peering | hub-firewall | no-forwarding | nsg
#
# Run it inside the toolbox (make shell), after `tofu apply` for that stage.
set -uo pipefail
STAGE=${1:-}
VERBOSE=${2:-}
API=${CLOUDTYKE_API:-https://management.cloudtyke.local}
AUTH=()
[ -n "${CLOUDTYKE_ADMIN_TOKEN:-}" ] && AUTH=(-H "X-CloudTyke-Admin-Token: $CLOUDTYKE_ADMIN_TOKEN")

A=10.10.1.4; A_DATA=10.10.2.4; B=10.20.1.4; HUB=10.0.1.4; NET=8.8.8.8

# name | from | to | port | expected result
case "$STAGE" in
  none) CASES="spoke A app -> spoke A data|$A|$A_DATA|443|Delivered
spoke A -> hub shared services|$A|$HUB|443|Dropped
spoke A -> spoke B|$A|$B|443|Dropped
spoke A -> internet|$A|$NET|443|Internet" ;;
  hub-only) CASES="spoke A -> hub shared services|$A|$HUB|443|Delivered
spoke B -> hub shared services|$B|$HUB|443|Delivered
spoke A -> spoke B (peering is not transitive)|$A|$B|443|Dropped
spoke B -> spoke A|$B|$A|443|Dropped" ;;
  direct-peering) CASES="spoke A -> hub shared services|$A|$HUB|443|Delivered
spoke A -> spoke B over the direct peering|$A|$B|443|Delivered
spoke B -> spoke A (return path)|$B|$A|443|Delivered" ;;
  hub-firewall) CASES="spoke A -> hub shared services|$A|$HUB|443|Delivered
spoke A -> spoke B through the hub firewall|$A|$B|443|Delivered
spoke B -> spoke A (return path, symmetric UDR)|$B|$A|443|Delivered
spoke A -> internet (no 0.0.0.0/0 UDR yet)|$A|$NET|443|Internet" ;;
  no-forwarding) CASES="spoke A -> hub shared services (still direct)|$A|$HUB|443|Delivered
spoke A -> spoke B (spoke B refuses forwarded traffic)|$A|$B|443|Dropped" ;;
  nsg) CASES="spoke A -> spoke B HTTPS (NSG deny)|$A|$B|443|Blocked
spoke A -> spoke B port 8080 (allowed)|$A|$B|8080|Delivered
spoke B -> spoke A HTTPS (rule is inbound on B only)|$B|$A|443|Delivered" ;;
  *) echo "usage: $0 <none|hub-only|direct-peering|hub-firewall|no-forwarding|nsg> [-v]"; exit 2 ;;
esac

pass=0; fail=0
echo "Stage: $STAGE"
echo
while IFS='|' read -r name from to port want; do
  out=$(curl -fsS "${AUTH[@]}" "$API/_cloudtyke/network/trace?from=$from&to=$to&port=$port") || { echo "cannot reach $API (run inside the toolbox: make shell)"; exit 2; }
  got=$(echo "$out" | jq -r .result)
  if [ "$got" = "$want" ]; then mark="PASS"; pass=$((pass+1)); else mark="FAIL"; fail=$((fail+1)); fi
  printf '%s  %-55s expect %-9s got %s\n' "$mark" "$name" "$want" "$got"
  if [ "$VERBOSE" = "-v" ] || [ "$mark" = "FAIL" ]; then
    echo "$out" | jq -r '.hops[] | "        \(.step). [\(.verdict)] \(.action): \(.detail)"'
  fi
done <<< "$CASES"
echo
echo "$pass passed, $fail failed"
[ $fail -eq 0 ]
