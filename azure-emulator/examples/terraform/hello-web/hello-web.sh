#!/usr/bin/env bash
# hello-web: deploy (up) or remove (down) the smallest end-to-end demo.
#   up   - Terraform (resource group, policy, Key Vault, Blob, AKS, identity),
#          then the app on the AKS cluster, opened on http://localhost:4740
#   down - remove the app and destroy everything
# Terraform runs in the CloudTyke toolbox container (it trusts the emulator's CA
# and resolves *.cloudtyke.local). Set TOOL=terraform to use Terraform instead
# of OpenTofu, TF_VAR_greeting=... to change the greeting.
set -euo pipefail
EX=$(cd "$(dirname "$0")" && pwd)
EMU=$(cd "$EX/../../.." && pwd)
TOOL=${TOOL:-tofu}
PORT=${HELLO_PORT:-4740}
W=/work/examples/terraform/hello-web
KC=/work/reports/.hello-web-kubeconfig # reports/ is git-ignored
PROXY=hello-web-proxy
cd "$EMU"
mkdir -p reports # git-ignored scratch space, shared with the toolbox as /work/reports

if [ ! -f .k3s-mode ]; then
  echo "==> switching the emulator to k3s mode (AKS clusters become real Kubernetes)"
  make up-k3s >/dev/null
fi
COMPOSE=(docker compose -f docker-compose.yml -f docker-compose.k3s.yml)
NET=cloudtyke-azure_cloudtyke
toolbox() { "${COMPOSE[@]}" run --rm -T -w "$W" -e TOOL="$TOOL" -e KC="$KC" -e TF_VAR_greeting -e TF_VAR_allow_public_blob toolbox bash -s; }

case "${1:-up}" in
down)
  docker rm -f "$PROXY" >/dev/null 2>&1 || true
  echo "==> $TOOL destroy"
  toolbox <<'EOS'
set -euo pipefail
$TOOL init -input=false -no-color >/dev/null
$TOOL destroy -auto-approve -input=false -no-color | tail -1
EOS
  exit 0
  ;;
up) ;;
*) echo "usage: $0 up|down" >&2; exit 2 ;;
esac

echo "==> $TOOL apply (resource group, policy, Key Vault, Blob, AKS, managed identity)"
toolbox <<'EOS'
set -euo pipefail
$TOOL init -input=false -no-color >/dev/null
$TOOL apply -auto-approve -input=false -no-color > /tmp/apply.log 2>&1 || { grep -A12 '^Error' /tmp/apply.log; exit 1; }
grep -E '^Apply complete' /tmp/apply.log
# kubeconfig: local accounts are disabled, so sign in with an Entra token for
# the AKS server app (what `kubelogin` does for a service principal).
D=${CLOUDTYKE_DOMAIN:-cloudtyke.local}
aks=$(curl -fsS "https://login.$D/$ARM_TENANT_ID/oauth2/v2.0/token" -d grant_type=client_credentials -d client_id="$ARM_CLIENT_ID" \
  --data-urlencode client_secret="$ARM_CLIENT_SECRET" -d scope=6dae42f8-4368-4678-94ff-3960e28e3630/.default | jq -r .access_token)
cat > "$KC" <<CFG
apiVersion: v1
kind: Config
clusters: [{ name: aks-hello-web, cluster: { server: "$($TOOL output -raw kube_server)", certificate-authority-data: "$($TOOL output -raw kube_ca)" } }]
users: [{ name: me, user: { token: "$aks" } }]
contexts: [{ name: aks-hello-web, context: { cluster: aks-hello-web, user: me } }]
current-context: aks-hello-web
CFG
for i in $(seq 1 80); do KUBECONFIG=$KC kubectl get --raw /readyz >/dev/null 2>&1 && exit 0; sleep 3; done
KUBECONFIG=$KC kubectl get --raw /readyz
EOS

cluster_id=$("${COMPOSE[@]}" run --rm -T -w "$W" toolbox sh -c "$TOOL output -raw cluster_id" 2>/dev/null)
node=$(docker ps --filter label=cloudtyke.aks/cluster --format '{{.Names}} {{.Label "cloudtyke.aks/cluster"}}' | awk -v id="$cluster_id" 'tolower($2)==tolower(id) {print $1}')
[ -n "$node" ] || { echo "no k3s node for $cluster_id" >&2; exit 1; }

echo "==> building the app and loading it into the cluster"
docker build -q -t hello-web:dev "$EX/app" >/dev/null
docker save hello-web:dev | docker exec -i "$node" ctr -n k8s.io images import - >/dev/null

echo "==> deploying to AKS (service account with workload identity, NodePort service)"
toolbox <<'EOS'
set -euo pipefail
export KUBECONFIG=$KC
app=$($TOOL output -json app)
v() { jq -r ".$1" <<<"$app"; }
D=${CLOUDTYKE_DOMAIN:-cloudtyke.local}
# cluster DNS: emulator names through the CloudTyke DNS sidecar
kubectl apply -f - >/dev/null <<YAML
apiVersion: v1
kind: ConfigMap
metadata: { name: coredns-custom, namespace: kube-system }
data:
  cloudtyke.server: |
    $D:53 {
      forward . ${CLOUDTYKE_DNS_IP:-10.253.53.53}
    }
YAML
kubectl -n kube-system rollout restart deploy/coredns >/dev/null
kubectl create configmap cloudtyke-ca --from-file=ca.pem=/cloudtyke-data/certs/ca.pem --dry-run=client -o yaml | kubectl apply -f - >/dev/null
# What the IaC tool built, for the page: names and IDs only (tofu show -json
# also holds secret values, so pick fields instead of passing it through).
providers=$($TOOL version | sed -n 's#^+ provider [^ ]*/\([^/ ]*\) \(v[^ ]*\)$#\1 \2#p' | jq -R . | jq -sc .)
$TOOL show -json | jq --arg tool "$TOOL" --arg v "$($TOOL version | head -1)" --argjson p "$providers" --arg at "$(date -u +%FT%TZ)" '
  {tool: $tool, version: $v, providers: $p, applied_at: $at,
   resources: [.values.root_module.resources[] | {address, type,
     name: (if .type == "azurerm_role_assignment" then null else .values.name end),
     role: .values.role_definition_name, scope: (.values.scope // "" | split("/") | last),
     principal_id: .values.principal_id, client_id: .values.client_id, object_id: .values.object_id,
     issuer: .values.issuer, subject: .values.subject, audience: .values.audience}
     | with_entries(select(.value != null and .value != ""))]}' > /tmp/iac.json
kubectl create configmap hello-web-iac --from-file=iac.json=/tmp/iac.json --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl apply -f - >/dev/null <<YAML
apiVersion: v1
kind: ServiceAccount
metadata:
  name: hello-web
  annotations: { azure.workload.identity/client-id: "$(v client_id)" }
---
apiVersion: apps/v1
kind: Deployment
metadata: { name: hello-web, labels: { app: hello-web } }
spec:
  replicas: 2
  selector: { matchLabels: { app: hello-web } }
  template:
    metadata: { labels: { app: hello-web, azure.workload.identity/use: "true" } }
    spec:
      serviceAccountName: hello-web
      containers:
        - name: web
          image: hello-web:dev
          imagePullPolicy: Never
          ports: [{ containerPort: 8080 }]
          readinessProbe: { httpGet: { path: /healthz, port: 8080 }, periodSeconds: 3 }
          env:
            - { name: AZURE_CLIENT_ID, value: "$(v client_id)" }
            - { name: AZURE_TENANT_ID, value: "$(v tenant_id)" }
            - { name: AZURE_FEDERATED_TOKEN_FILE, value: /var/run/secrets/azure/tokens/azure-identity-token }
            - { name: VAULT_URI, value: "$(v vault_uri)" }
            - { name: STORAGE_ACCOUNT, value: "$(v storage_account)" }
            - { name: CLUSTER_NAME, value: aks-hello-web }
            - { name: CLOUDTYKE_DOMAIN, value: "$D" }
            - { name: NODE_EXTRA_CA_CERTS, value: /etc/cloudtyke/ca.pem }
          volumeMounts:
            - { name: token, mountPath: /var/run/secrets/azure/tokens, readOnly: true }
            - { name: ca, mountPath: /etc/cloudtyke, readOnly: true }
            - { name: iac, mountPath: /etc/hello-web, readOnly: true }
      volumes:
        - name: token
          projected:
            sources: [{ serviceAccountToken: { path: azure-identity-token, audience: api://AzureADTokenExchange, expirationSeconds: 3600 } }]
        - name: ca
          configMap: { name: cloudtyke-ca }
        - name: iac
          configMap: { name: hello-web-iac, optional: true }
---
apiVersion: v1
kind: Service
metadata: { name: hello-web }
spec:
  type: NodePort
  selector: { app: hello-web }
  ports: [{ port: 80, targetPort: 8080, nodePort: 30080 }]
YAML
kubectl rollout restart deploy/hello-web >/dev/null
kubectl rollout status deploy/hello-web --timeout=180s
EOS

BIND=${HELLO_BIND:-127.0.0.1}
docker rm -f "$PROXY" >/dev/null 2>&1 || true
CURL=(curl -fsS)
if [ "$BIND" = "127.0.0.1" ] || [ "$BIND" = "localhost" ]; then
  echo "==> publishing the app on http://localhost:$PORT"
  docker run -d --name "$PROXY" --restart unless-stopped --network "$NET" -p "127.0.0.1:$PORT:8080" \
    alpine/socat -d TCP-LISTEN:8080,fork,reuseaddr "TCP:$node:30080" >/dev/null
  URL="http://localhost:$PORT"
else
  # Reachable from other machines: always behind a password (basic auth via Caddy).
  CRED="$EMU/reports/.hello-web-lan-credentials" # git-ignored
  if [ -n "${HELLO_PASSWORD:-}" ]; then pw=$HELLO_PASSWORD
  elif [ -f "$CRED" ]; then pw=$(sed -n 's/^password=//p' "$CRED")
  else pw=$(openssl rand -base64 18 | tr -d '/+=' | cut -c1-20); fi
  user=${HELLO_USER:-intern}
  (umask 077; printf 'user=%s\npassword=%s\n' "$user" "$pw" > "$CRED")
  hash=$(docker run --rm caddy:2-alpine caddy hash-password --plaintext "$pw")
  cat > "$EMU/reports/.hello-web-Caddyfile" <<CADDY
:8080 {
	basic_auth {
		$user $hash
	}
	reverse_proxy $node:30080
}
CADDY
  echo "==> publishing the app on $BIND:$PORT (password protected)"
  docker run -d --name "$PROXY" --restart unless-stopped --network "$NET" -p "$BIND:$PORT:8080" \
    -v "$EMU/reports/.hello-web-Caddyfile:/etc/caddy/Caddyfile:ro" caddy:2-alpine >/dev/null
  CURL=(curl -fsS -u "$user:$pw")
  ip=$BIND; [ "$ip" = "0.0.0.0" ] && ip=$(ipconfig getifaddr en0 2>/dev/null || hostname -I 2>/dev/null | awk '{print $1}')
  NAME=${HELLO_HOSTNAME:-hello.cloudtyke.local}
  suffix=":$PORT"; [ "$PORT" = 80 ] && suffix=""
  URL="http://$NAME$suffix"
  HOSTS_HINT="  on other machines, add this line to the hosts file (/etc/hosts; on Windows drivers/etc/hosts):  ${ip:-THIS-MACHINE-IP}  $NAME
  or use the IP directly: http://${ip:-localhost}$suffix"
fi
for i in $(seq 1 30); do
  if body=$("${CURL[@]}" "http://localhost:$PORT/" 2>/dev/null); then
    echo
    echo "hello-web is up: $URL"
    [ "${user:-}" ] && echo "  sign in as $user / $pw  (saved in reports/.hello-web-lan-credentials)"
    [ "${HOSTS_HINT:-}" ] && echo "$HOSTS_HINT"
    grep -o 'viewed <b>[0-9]*</b>' <<<"$body" | sed 's/<[^>]*>//g; s/^/  page /'
    exit 0
  fi
  sleep 2
done
echo "the app did not answer on $URL" >&2
exit 1
