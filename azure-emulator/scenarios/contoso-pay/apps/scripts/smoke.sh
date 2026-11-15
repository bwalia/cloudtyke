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

# Spans arrive from each service's exporter in batches: wait for the whole trace.
wait_for "Jaeger has the whole checkout trace (storefront, gateway, risk engine)" "curl -fsS $JAEGER/api/traces/$trace | jq -e '[.data[0].processes[].serviceName] as \$s | (\$s | index(\"northwind-storefront\")) and (\$s | index(\"payment-gateway\")) and (\$s | index(\"risk-engine\"))'" 30
# What the trace touched: Azure data planes (by host) and databases (by db.system).
touched=$(curl -fsS "$JAEGER/api/traces/$trace" | jq -r '[.data[0].spans[].tags[] | select(.key=="server.address" or .key=="net.peer.name" or .key=="db.system" or .key=="db.system.name") | .value | tostring] | unique | join(" ")')
has() { grep -q -- "$1" <<<"$touched"; }
if has '.blob.storage.' && has 'postgresql' && has 'mongodb'; then ok "the trace includes Blob, PostgreSQL and MongoDB calls"
else bad "the trace includes Blob, PostgreSQL and MongoDB calls (saw: $touched)"; fi
# Secrets are cached for five minutes, so look for Key Vault calls across recent gateway traces.
vault=$(grep -s '^PAYMENTS_VAULT_URI=' "$APPS/.env" | cut -d/ -f3)
kv=$(curl -fsS -G "$JAEGER/api/v3/traces" --data-urlencode query.service_name=payment-gateway \
  --data-urlencode "query.attributes={\"server.address\":\"$vault\"}" --data-urlencode query.search_depth=5 \
  --data-urlencode "query.start_time_min=$(date -u -d '-1 hour' +%FT%TZ 2>/dev/null || date -u -v-1H +%FT%TZ)" \
  --data-urlencode "query.start_time_max=$(date -u +%FT%TZ)" 2>/dev/null | jq '[.result.resourceSpans[]?.scopeSpans[]?.spans[]?] | length' 2>/dev/null)
if [ "${kv:-0}" -gt 0 ]; then ok "Jaeger has Key Vault calls from the gateway ($vault)"; else bad "Jaeger has Key Vault calls from the gateway ($vault)"; fi
wait_for "Prometheus has payment metrics" "curl -fsS '$PROM/api/v1/query?query=sum(cpay_payments_total)' | jq -e '.data.result[0].value[1] | tonumber > 0'" 30
wait_for "Prometheus has span metrics" "curl -fsS '$PROM/api/v1/query?query=sum(traces_span_metrics_duration_milliseconds_count)' | jq -e '.data.result[0].value[1] | tonumber > 0'" 30
GRAFANA=http://localhost:${GRAFANA_PORT:-4730}
wait_for "Loki has logs from the payment gateway" "curl -fsS -G '$GRAFANA/api/datasources/proxy/uid/loki/loki/api/v1/query_range' --data-urlencode 'query={service_name=\"payment-gateway\"}' --data-urlencode limit=5 | jq -e '.data.result | length > 0'" 30

[ $fail -eq 0 ] && echo "CONTOSO PAY SMOKE PASSED" || echo "CONTOSO PAY SMOKE FAILED"
exit $fail
