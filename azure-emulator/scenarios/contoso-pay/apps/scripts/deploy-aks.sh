#!/usr/bin/env bash
# Deploys the Contoso Pay services to the scenario's AKS cluster, as the
# platform pipeline would: Entra sign-in (local accounts are disabled), Azure
# RBAC for Kubernetes, workload identity per service account.
#
# Needs the emulator's k3s backend so the cluster is a real Kubernetes:
#   make up-k3s && scenarios/contoso-pay/apps/scripts/up.sh && scenarios/contoso-pay/apps/scripts/deploy-aks.sh
# Telemetry goes to the OpenTelemetry collector that up.sh started.
set -euo pipefail
APPS=$(cd "$(dirname "$0")/.." && pwd)
EMU=$(cd "$APPS/../../.." && pwd)
TOOL=${TOOL:-tofu}
TF=/work/scenarios/contoso-pay/terraform
cd "$EMU"
[ -f .k3s-mode ] || { echo "The AKS cluster needs the k3s backend: run 'make up-k3s', then scripts/up.sh, then this script." >&2; exit 1; }
[ -f "$APPS/.env" ] || { echo "Run scripts/up.sh first." >&2; exit 1; }
COMPOSE=(docker compose -f docker-compose.yml -f docker-compose.k3s.yml)

cluster_id=$("${COMPOSE[@]}" run --rm -T -w "$TF" toolbox sh -c "$TOOL output -raw aks_cluster_id" 2>/dev/null)
node=$(docker ps --filter label=cloudtyke.aks/cluster --format '{{.Names}} {{.Label "cloudtyke.aks/cluster"}}' | awk -v id="$cluster_id" 'tolower($2)==tolower(id) {print $1}')
[ -n "$node" ] || { echo "No k3s node for $cluster_id. Was the cluster created before 'make up-k3s'? Recreate it: $TOOL apply -replace=azurerm_kubernetes_cluster.payments" >&2; exit 1; }

echo "==> loading the app image into the cluster ($node)"
docker build -q -t contoso-pay/apps:dev "$APPS" >/dev/null
docker save contoso-pay/apps:dev | docker exec -i "$node" ctr -n k8s.io images import - >/dev/null

collector_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' contoso-pay-otel-collector-1)
cp "$APPS/k8s/contoso-pay.yaml" "$APPS/k8s/workloads.yaml" "$EMU/reports/"

echo "==> applying manifests as the platform pipeline (Entra token, Azure RBAC)"
"${COMPOSE[@]}" run --rm -T -w "$TF" -e TOOL="$TOOL" -e COLLECTOR_IP="$collector_ip" \
  -v "$APPS/.env:/run/cpay.env:ro" toolbox bash -s <<'EOS'
set -euo pipefail
set -a; . /run/cpay.env; set +a
D=$CLOUDTYKE_DOMAIN
out=$($TOOL output -json)
ids=$(jq -c '.workload_identities.value' <<<"$out")
export AKS_CLUSTER=$(jq -r '.aks_cluster.value.name' <<<"$out")
export GATEWAY_CLIENT_ID=$(jq -r '.gateway' <<<"$ids") RISK_CLIENT_ID=$(jq -r '."risk-engine"' <<<"$ids")
export PORTAL_CLIENT_ID=$(jq -r '."merchant-portal"' <<<"$ids") STOREFRONT_CLIENT_ID=$(jq -r '.storefront' <<<"$ids")
export SETTLEMENT_CLIENT_ID=$(jq -r '."settlement-batch"' <<<"$ids") APP_IMAGE=contoso-pay/apps:dev
export CLOUDTYKE_DNS_IP=${CLOUDTYKE_DNS_IP:-10.253.53.53}

# kubeconfig: cluster endpoint from az aks get-credentials, user = an Entra
# token for the AKS server app (what kubelogin does with a service principal).
M=https://management.$D
arm=$(curl -fsS "https://login.$D/$ARM_TENANT_ID/oauth2/v2.0/token" -d grant_type=client_credentials -d client_id=$ARM_CLIENT_ID \
  --data-urlencode client_secret=$ARM_CLIENT_SECRET -d scope=$M/.default | jq -r .access_token)
aks=$(curl -fsS "https://login.$D/$ARM_TENANT_ID/oauth2/v2.0/token" -d grant_type=client_credentials -d client_id=$ARM_CLIENT_ID \
  --data-urlencode client_secret=$ARM_CLIENT_SECRET -d scope=6dae42f8-4368-4678-94ff-3960e28e3630/.default | jq -r .access_token)
cid=$($TOOL output -raw aks_cluster_id)
curl -fsS -X POST -H "Authorization: Bearer $arm" "$M$cid/listClusterUserCredential?api-version=2025-02-01" | jq -r '.kubeconfigs[0].value' | base64 -d > /tmp/kc.user
server=$(awk '/server:/ {print $2; exit}' /tmp/kc.user); ca=$(awk '/certificate-authority-data:/ {print $2; exit}' /tmp/kc.user)
cat > /tmp/kc <<KC
apiVersion: v1
kind: Config
clusters: [{ name: aks, cluster: { server: "$server", certificate-authority-data: "$ca" } }]
users: [{ name: pipeline, user: { token: "$aks" } }]
contexts: [{ name: aks, context: { cluster: aks, user: pipeline } }]
current-context: aks
KC
export KUBECONFIG=/tmp/kc
for i in $(seq 1 60); do kubectl get --raw /readyz >/dev/null 2>&1 && break; sleep 3; done

python3 -c "import os,sys; sys.stdout.write(os.path.expandvars(sys.stdin.read()))" < /work/reports/contoso-pay.yaml | kubectl apply -f - >/dev/null
for ns in payments risk merchant storefront settlement; do
  kubectl -n $ns create configmap cloudtyke-ca --from-file=ca.pem=/cloudtyke-data/certs/ca.pem --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  kubectl -n $ns create configmap cpay-config --from-literal=CLOUDTYKE_DOMAIN=$D \
    --from-literal=PAYMENTS_VAULT_URI=$PAYMENTS_VAULT_URI --from-literal=MERCHANT_VAULT_URI=$MERCHANT_VAULT_URI \
    --from-literal=RECEIPTS_ACCOUNT=$RECEIPTS_ACCOUNT --from-literal=SETTLEMENT_ACCOUNT=$SETTLEMENT_ACCOUNT \
    --dry-run=client -o yaml | kubectl apply -f - >/dev/null
done
kubectl -n data rollout status deploy/postgres --timeout=180s >/dev/null
pg_ip=$(kubectl -n data get svc postgres -o jsonpath='{.spec.clusterIP}'); mongo_ip=$(kubectl -n data get svc mongo -o jsonpath='{.spec.clusterIP}')
# Cluster DNS: emulator names via the CloudTyke DNS sidecar, except the
# database FQDNs (to the in-cluster stand-ins) and the telemetry collector.
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
python3 -c "import os,sys; sys.stdout.write(os.path.expandvars(sys.stdin.read()))" < /work/reports/workloads.yaml | kubectl apply -f -
for d in payments/payment-gateway risk/risk-engine storefront/northwind-storefront merchant/processor-portal settlement/settlement-batch storefront/traffic-generator; do
  kubectl -n ${d%/*} rollout status deploy/${d#*/} --timeout=240s
done
kubectl get pods -A -l 'azure.workload.identity/use=true' -o wide
EOS
rm -f "$EMU/reports/contoso-pay.yaml" "$EMU/reports/workloads.yaml"
echo
echo "Deployed. Traces from the AKS pods carry k8s.cluster.name=<cluster> and k8s.namespace.name in Jaeger and Grafana."
