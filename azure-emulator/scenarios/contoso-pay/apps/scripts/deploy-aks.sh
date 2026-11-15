#!/usr/bin/env bash
# Deploys the Contoso Pay services to the scenario's AKS cluster, as the
# platform pipeline would: Entra sign-in (local accounts are disabled), Azure
# RBAC for Kubernetes, workload identity per service account.
#
# Needs the emulator's k3s backend so the cluster is a real Kubernetes:
#   make up-k3s && make contoso-pay && make contoso-pay-aks
# The emulator starts the cluster's k3s node on the first API request, so a
# cluster created before 'make up-k3s' works too. Telemetry goes to the
# OpenTelemetry collector that 'make contoso-pay' started.
set -euo pipefail
APPS=$(cd "$(dirname "$0")/.." && pwd)
EMU=$(cd "$APPS/../../.." && pwd)
TOOL=${TOOL:-tofu}
TF=/work/scenarios/contoso-pay/terraform
KC=/work/reports/.contoso-pay-kubeconfig # reports/ is git-ignored
cd "$EMU"
[ -f .k3s-mode ] || { echo "The AKS cluster needs the k3s backend: run 'make up-k3s' and 'make contoso-pay' first." >&2; exit 1; }
[ -f "$APPS/.env" ] || { echo "Run 'make contoso-pay' first." >&2; exit 1; }
COMPOSE=(docker compose -f docker-compose.yml -f docker-compose.k3s.yml)
toolbox() { "${COMPOSE[@]}" run --rm -T -w "$TF" -e TOOL="$TOOL" -e KC="$KC" "$@"; }
trap 'rm -f "$EMU/reports/.contoso-pay-kubeconfig" "$EMU/reports/contoso-pay.yaml" "$EMU/reports/workloads.yaml"' EXIT

echo "==> signing in to the cluster with Entra ID (the pipeline's service principal)"
toolbox toolbox bash -s <<'EOS'
set -euo pipefail
D=${CLOUDTYKE_DOMAIN:-cloudtyke.local}
M=https://management.$D
tok() { curl -fsS "https://login.$D/$ARM_TENANT_ID/oauth2/v2.0/token" -d grant_type=client_credentials -d client_id=$ARM_CLIENT_ID \
  --data-urlencode client_secret=$ARM_CLIENT_SECRET -d scope="$1" | jq -r .access_token; }
arm=$(tok "$M/.default")
# what kubelogin does with a service principal: a token for the AKS server app
aks=$(tok 6dae42f8-4368-4678-94ff-3960e28e3630/.default)
cid=$($TOOL output -raw aks_cluster_id)
# az aks get-credentials (user credential: endpoint and CA; Entra replaces the static token)
kc=$(curl -fsS -X POST -H "Authorization: Bearer $arm" "$M$cid/listClusterUserCredential?api-version=2025-02-01" | jq -r '.kubeconfigs[0].value' | base64 -d)
server=$(awk '/server:/ {print $2; exit}' <<<"$kc"); ca=$(awk '/certificate-authority-data:/ {print $2; exit}' <<<"$kc")
cat > "$KC" <<CFG
apiVersion: v1
kind: Config
clusters: [{ name: aks, cluster: { server: "$server", certificate-authority-data: "$ca" } }]
users: [{ name: pipeline, user: { token: "$aks" } }]
contexts: [{ name: aks, context: { cluster: aks, user: pipeline } }]
current-context: aks
CFG
echo "    API server: $server"
for i in $(seq 1 80); do
  if KUBECONFIG=$KC kubectl get --raw /readyz >/dev/null 2>&1; then echo "    ready"; exit 0; fi
  sleep 3
done
KUBECONFIG=$KC kubectl get --raw /readyz
EOS

cluster_id=$(toolbox toolbox sh -c "$TOOL output -raw aks_cluster_id" 2>/dev/null)
node=$(docker ps --filter label=cloudtyke.aks/cluster --format '{{.Names}} {{.Label "cloudtyke.aks/cluster"}}' | awk -v id="$cluster_id" 'tolower($2)==tolower(id) {print $1}')
[ -n "$node" ] || { echo "No k3s node found for $cluster_id." >&2; exit 1; }

echo "==> loading images into the cluster ($node)"
docker build -q -t contoso-pay/apps:dev "$APPS" >/dev/null
# The stand-in databases too: pulling mongo inside k3s takes minutes.
for img in contoso-pay/apps:dev postgres:17-alpine mongo:8.0; do
  docker image inspect "$img" >/dev/null 2>&1 || docker pull -q "$img" >/dev/null
  docker save "$img" | docker exec -i "$node" ctr -n k8s.io images import - >/dev/null
done

collector_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' contoso-pay-otel-collector-1)
cp "$APPS/k8s/contoso-pay.yaml" "$APPS/k8s/workloads.yaml" "$EMU/reports/"

echo "==> applying manifests (Azure RBAC for Kubernetes authorizes every call)"
toolbox -e COLLECTOR_IP="$collector_ip" -v "$APPS/.env:/run/cpay.env:ro" toolbox bash -s <<'EOS'
set -euo pipefail
set -a; . /run/cpay.env; set +a
export KUBECONFIG=$KC
D=$CLOUDTYKE_DOMAIN
render() { python3 -c "import os,sys; sys.stdout.write(os.path.expandvars(sys.stdin.read()))" < "$1"; }
out=$($TOOL output -json)
ids=$(jq -c '.workload_identities.value' <<<"$out")
export AKS_CLUSTER=$(jq -r '.aks_cluster.value.name' <<<"$out") APP_IMAGE=contoso-pay/apps:dev
export GATEWAY_CLIENT_ID=$(jq -r '.gateway' <<<"$ids") RISK_CLIENT_ID=$(jq -r '."risk-engine"' <<<"$ids")
export PORTAL_CLIENT_ID=$(jq -r '."merchant-portal"' <<<"$ids") STOREFRONT_CLIENT_ID=$(jq -r '.storefront' <<<"$ids")
export SETTLEMENT_CLIENT_ID=$(jq -r '."settlement-batch"' <<<"$ids")
export CLOUDTYKE_DNS_IP=${CLOUDTYKE_DNS_IP:-10.253.53.53}

render /work/reports/contoso-pay.yaml | kubectl apply -f - >/dev/null
for ns in payments risk merchant storefront settlement; do
  kubectl -n $ns create configmap cloudtyke-ca --from-file=ca.pem=/cloudtyke-data/certs/ca.pem --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  kubectl -n $ns create configmap cpay-config --from-literal=CLOUDTYKE_DOMAIN=$D \
    --from-literal=PAYMENTS_VAULT_URI=$PAYMENTS_VAULT_URI --from-literal=MERCHANT_VAULT_URI=$MERCHANT_VAULT_URI \
    --from-literal=RECEIPTS_ACCOUNT=$RECEIPTS_ACCOUNT --from-literal=SETTLEMENT_ACCOUNT=$SETTLEMENT_ACCOUNT \
    --dry-run=client -o yaml | kubectl apply -f - >/dev/null
done
kubectl -n data rollout status deploy/postgres --timeout=240s >/dev/null
kubectl -n data rollout status deploy/mongo --timeout=240s >/dev/null
pg_ip=$(kubectl -n data get svc postgres -o jsonpath='{.spec.clusterIP}'); mongo_ip=$(kubectl -n data get svc mongo -o jsonpath='{.spec.clusterIP}')
# Cluster DNS: emulator names through the CloudTyke DNS sidecar, except the
# database FQDNs (in-cluster stand-ins) and the telemetry collector.
kubectl apply -f - >/dev/null <<CM
apiVersion: v1
kind: ConfigMap
metadata: { name: coredns-custom, namespace: kube-system }
data:
  contoso-pay.server: |
    $D:53 {
      hosts {
        $pg_ip $POSTGRES_FQDN
        $mongo_ip $COSMOS_ACCOUNT.mongo.cosmos.$D
        fallthrough
      }
      forward . $CLOUDTYKE_DNS_IP
    }
    cpay.internal:53 {
      hosts {
        $COLLECTOR_IP otel-collector.cpay.internal
      }
    }
CM
kubectl -n kube-system rollout restart deploy/coredns >/dev/null && kubectl -n kube-system rollout status deploy/coredns --timeout=120s >/dev/null
render /work/reports/workloads.yaml | kubectl apply -f - >/dev/null
for d in payments/payment-gateway risk/risk-engine storefront/northwind-storefront merchant/processor-portal settlement/settlement-batch storefront/traffic-generator; do
  kubectl -n ${d%/*} rollout status deploy/${d#*/} --timeout=300s
done
kubectl get pods -A -l 'azure.workload.identity/use=true' -o wide
EOS
echo
echo "Deployed. In Jaeger and Grafana, spans from the AKS pods carry k8s.cluster.name and k8s.namespace.name."
