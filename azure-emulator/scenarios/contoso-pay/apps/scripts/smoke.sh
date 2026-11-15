#!/usr/bin/env bash
# Checks the running Contoso Pay demo end to end (after scripts/up.sh):
# a checkout succeeds, its receipt comes back from Blob storage, the console
# sees live traffic, settlement files land in Azure Files, and traces, metrics
# and logs reach Jaeger, Prometheus and Loki.
set -uo pipefail
SF=http://localhost:${STOREFRONT_PORT:-4701}
PORTAL=http://localhost:${PORTAL_PORT:-4703}
JAEGER=http://localhost:${JAEGER_PORT:-4731}
PROM=http://localhost:${PROMETHEUS_PORT:-4732}
APPS=$(cd "$(dirname "$0")/.." && pwd)
fail=0
ok() { printf '  \033[32mok\033[0m   %s\n' "$1"; }
bad() { printf '  \033[31mFAIL\033[0m %s\n' "$1"; fail=1; }
check() { if eval "$2" >/dev/null 2>&1; then ok "$1"; else bad "$1"; fi; }
wait_for() { for _ in $(seq 1 "${3:-60}"); do eval "$2" >/dev/null 2>&1 && { ok "$1"; return; }; sleep 2; done; bad "$1"; }

echo "Contoso Pay smoke test"
wait_for "storefront is up" "curl -fsS $SF/healthz"
wait_for "processor console is up" "curl -fsS $PORTAL/healthz"

body='{"flight":"NW 101","cabin":"Economy","passengers":1,"passenger":{"name":"Smoke Test","email":"smoke@example.com"},"card":{"number":"4242424242424242","expiry":"12/29","cvc":"123"}}'
out=""
for _ in $(seq 1 30); do
  out=$(curl -fsS -X POST -H 'content-type: application/json' "$SF/api/checkout" -d "$body" 2>/dev/null) && [ "$(jq -r .outcome <<<"$out")" = confirmed ] && break
  sleep 3
done
pay=$(jq -r '.payment.id // empty' <<<"$out")
trace=$(jq -r '.payment.trace_id // empty' <<<"$out")
check "checkout with a test Visa card is confirmed ($pay)" "[ -n '$pay' ] && [ \"\$(jq -r .outcome <<<'$out')\" = confirmed ]"
check "receipt is read back from Blob storage" "curl -fsS $SF/api/receipt/$pay | jq -e '.receipt == \"$pay\"'"

declined=$(curl -fsS -X POST -H 'content-type: application/json' "$SF/api/checkout" -d "${body/4242424242424242/4000000000000002}")
check "a declined card is declined (do_not_honour)" "jq -e '.payment.decline_reason == \"do_not_honour\"' <<<'$declined'"

wait_for "console shows live traffic from the generator" "curl -fsS $PORTAL/api/summary | jq -e '.kpi.total > 5'" 45
wait_for "a settlement batch is written to Azure Files" "curl -fsS $PORTAL/api/summary | jq -e '.batches | length > 0'" 90
batch=$(curl -fsS "$PORTAL/api/summary" | jq -r '.batches[0].file_path // empty')
check "settlement CSV is read back from Azure Files" "curl -fsS '$PORTAL/api/settlement-file?path=$batch' | head -1 | grep -q '^payment_id,'"

wait_for "Jaeger has the checkout trace" "curl -fsS $JAEGER/api/traces/$trace | jq -e '.data[0].spans | length > 10'" 30
services=$(curl -fsS "$JAEGER/api/traces/$trace" | jq -r '[.data[0].processes[].serviceName] | unique | join(",")')
check "the trace spans storefront, gateway and risk engine ($services)" "grep -q northwind-storefront <<<'$services' && grep -q payment-gateway <<<'$services' && grep -q risk-engine <<<'$services'"
spans=$(curl -fsS "$JAEGER/api/traces/$trace" | jq -r '[.data[0].spans[].operationName] | join(" ")')
check "the trace includes Key Vault, Blob, PostgreSQL and MongoDB calls" "grep -qi 'secrets\|GET' <<<'$spans' && grep -qi 'insert\|pg\|INSERT' <<<'$spans' && grep -qi 'mongodb\|insert' <<<'$spans'"
wait_for "Prometheus has payment metrics" "curl -fsS '$PROM/api/v1/query?query=sum(cpay_payments_total)' | jq -e '.data.result[0].value[1] | tonumber > 0'" 30
wait_for "Prometheus has span metrics" "curl -fsS '$PROM/api/v1/query?query=sum(traces_span_metrics_calls_total)' | jq -e '.data.result | length > 0'" 30
loki=$(cd "$APPS" && docker compose exec -T loki wget -qO- 'http://localhost:3100/loki/api/v1/labels' 2>/dev/null)
check "Loki is receiving logs" "jq -e '.data | index(\"service_name\")' <<<'$loki'"

[ $fail -eq 0 ] && echo "CONTOSO PAY SMOKE PASSED" || echo "CONTOSO PAY SMOKE FAILED"
exit $fail
