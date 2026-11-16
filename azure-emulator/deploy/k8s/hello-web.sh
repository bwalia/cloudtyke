#!/usr/bin/env bash
# hello-web on the hosted sandbox: https://hello.cloudtyke.dev
#
# Same Terraform as `make hello-web` (examples/terraform/hello-web), applied to
# the hosted emulator (management.cloudtyke.dev) with its own state file,
# and the same app, but running in the real k3s1 cluster (namespace cloudtyke).
# Its workload identity trusts k3s1's service-account issuer, whose keys
# deploy.sh gave the emulator (CLOUDTYKE_TRUSTED_ISSUERS_FILE).
#
#   deploy/k8s/hello-web.sh          apply + build + deploy
#   deploy/k8s/hello-web.sh down     remove the app and destroy its resources
set -euo pipefail
EMU=$(cd "$(dirname "$0")/../.." && pwd)
ROOT=$(cd "$EMU/.." && pwd)
NS=cloudtyke
TOOL=${TOOL:-tofu}
TAG=${IMAGE_TAG:-$(git -C "$ROOT" rev-parse --short=12 HEAD)}
BUILDER=${BUILDER:-cloudtyke}
CRED="$EMU/reports/.cloudtyke-dev-credentials"
[ -f "$CRED" ] || { echo "no $CRED: run deploy/k8s/deploy.sh first" >&2; exit 1; }
v() { sed -n "s/^$1=\([^ ]*\).*/\1/p" "$CRED"; }
ISSUER=$(kubectl get --raw /.well-known/openid-configuration | jq -r .issuer)

# Terraform in the toolbox, pointed at the hosted emulator (real certificates,
# so no local CA). It runs on a copy of main.tf with its own state file in
# reports/hello-web-hosted/ (git-ignored), so it never touches the local
# example's state or selected workspace. --no-deps leaves the local emulator alone.
tf() {
  mkdir -p "$EMU/reports/hello-web-hosted"
  cp "$EMU/examples/terraform/hello-web/main.tf" "$EMU/reports/hello-web-hosted/main.tf"
  [ -f "$EMU/examples/terraform/hello-web/.terraform.lock.hcl" ] && cp "$EMU/examples/terraform/hello-web/.terraform.lock.hcl" "$EMU/reports/hello-web-hosted/"
  docker compose -f "$EMU/docker-compose.yml" run --rm --no-deps -T -w /work/reports/hello-web-hosted \
    -e TOOL="$TOOL" -e ARM_METADATA_HOSTNAME=management.cloudtyke.dev \
    -e ARM_TENANT_ID="$(v tenant_id)" -e ARM_CLIENT_ID="$(v bootstrap_client_id)" -e ARM_CLIENT_SECRET="$(v bootstrap_client_secret)" \
    -e ARM_SUBSCRIPTION_ID=5b5c0000-0000-4000-8000-000000000003 \
    -e TF_VAR_federation_issuer="$ISSUER" -e TF_VAR_app_namespace="$NS" -e TF_VAR_greeting \
    toolbox bash -s
}

if [ "${1:-up}" = down ]; then
  kubectl -n "$NS" delete deploy/hello-web svc/hello-web ingress/hello-web sa/hello-web cm/hello-web-iac --ignore-not-found >/dev/null
  tf <<'EOS'
set -euo pipefail
$TOOL init -input=false -no-color >/dev/null
$TOOL destroy -auto-approve -input=false -no-color | tail -1
EOS
  exit 0
fi

echo "==> $TOOL apply against management.cloudtyke.dev (state in reports/hello-web-hosted)"
tf <<'EOS' >"${TMPDIR:-/tmp}/hello-hosted.json"
set -euo pipefail
$TOOL init -input=false -no-color >/dev/null
$TOOL apply -auto-approve -input=false -no-color >/tmp/apply.log 2>&1 || { grep -A12 '^Error' /tmp/apply.log >&2; exit 1; }
grep -E '^Apply complete' /tmp/apply.log >&2
providers=$($TOOL version | sed -n 's#^+ provider [^ ]*/\([^/ ]*\) \(v[^ ]*\)$#\1 \2#p' | jq -R . | jq -sc .)
$TOOL show -json | jq --arg tool "$TOOL" --arg v "$($TOOL version | head -1)" --argjson p "$providers" --arg at "$(date -u +%FT%TZ)" \
  --argjson app "$($TOOL output -json app)" '
  {app: $app, iac: {tool: $tool, version: $v, providers: $p, applied_at: $at,
   resources: [.values.root_module.resources[] | {address, type,
     name: (if .type == "azurerm_role_assignment" then null else .values.name end),
     role: .values.role_definition_name, scope: (.values.scope // "" | split("/") | last),
     principal_id: .values.principal_id, client_id: .values.client_id, object_id: .values.object_id,
     issuer: .values.issuer, subject: .values.subject, audience: .values.audience}
     | with_entries(select(.value != null and .value != ""))]}}'
EOS
out="${TMPDIR:-/tmp}/hello-hosted.json"
a() { jq -r ".app.$1" "$out"; }

echo "==> building bwalia/cloudtyke-hello-web:$TAG"
docker buildx inspect "$BUILDER" >/dev/null 2>&1 || docker buildx create --name "$BUILDER" --driver docker-container >/dev/null
docker buildx build --builder "$BUILDER" --platform linux/amd64 --push -q -t "bwalia/cloudtyke-hello-web:$TAG" \
  "$EMU/examples/terraform/hello-web/app" >/dev/null

echo "==> deploying to k3s1 (namespace $NS)"
jq '.iac' "$out" >"${TMPDIR:-/tmp}/hello-iac.json"
kubectl -n "$NS" create configmap hello-web-iac --from-file=iac.json="${TMPDIR:-/tmp}/hello-iac.json" \
  --dry-run=client -o yaml | kubectl apply -f - >/dev/null
# Basic auth at traefik, with the sandbox login (same as the portal).
pw=$(v portal_password)
kubectl -n "$NS" create secret generic sandbox-basic-auth \
  --from-literal=users="cloudtyke:$(openssl passwd -apr1 "$pw")" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl apply -f - >/dev/null <<YAML
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata: { name: sandbox-auth, namespace: $NS }
spec:
  basicAuth: { secret: sandbox-basic-auth, realm: "CloudTyke sandbox" }
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: hello-web
  namespace: $NS
  annotations: { azure.workload.identity/client-id: "$(a client_id)" }
---
apiVersion: apps/v1
kind: Deployment
metadata: { name: hello-web, namespace: $NS, labels: { app: hello-web } }
spec:
  replicas: 2
  selector: { matchLabels: { app: hello-web } }
  template:
    metadata: { labels: { app: hello-web } }
    spec:
      serviceAccountName: hello-web
      securityContext: { runAsNonRoot: true, runAsUser: 65532, seccompProfile: { type: RuntimeDefault } }
      containers:
        - name: web
          image: bwalia/cloudtyke-hello-web:$TAG
          ports: [{ containerPort: 8080 }]
          readinessProbe: { httpGet: { path: /healthz, port: 8080 }, periodSeconds: 5 }
          resources: { requests: { cpu: 10m, memory: 48Mi }, limits: { memory: 128Mi } }
          securityContext: { allowPrivilegeEscalation: false, readOnlyRootFilesystem: true, capabilities: { drop: [ALL] } }
          env:
            - { name: AZURE_CLIENT_ID, value: "$(a client_id)" }
            - { name: AZURE_TENANT_ID, value: "$(a tenant_id)" }
            - { name: AZURE_FEDERATED_TOKEN_FILE, value: /var/run/secrets/azure/tokens/azure-identity-token }
            - { name: VAULT_URI, value: "$(a vault_uri)" }
            - { name: STORAGE_ACCOUNT, value: "$(a storage_account)" }
            - { name: CLUSTER_NAME, value: "k3s1 (hosted sandbox)" }
            - { name: CLOUDTYKE_DOMAIN, value: cloudtyke.dev }
          volumeMounts:
            - { name: token, mountPath: /var/run/secrets/azure/tokens, readOnly: true }
            - { name: iac, mountPath: /etc/hello-web, readOnly: true }
      volumes:
        - name: token
          projected:
            sources: [{ serviceAccountToken: { path: azure-identity-token, audience: api://AzureADTokenExchange, expirationSeconds: 3600 } }]
        - name: iac
          configMap: { name: hello-web-iac, optional: true }
---
apiVersion: v1
kind: Service
metadata: { name: hello-web, namespace: $NS }
spec:
  selector: { app: hello-web }
  ports: [{ port: 80, targetPort: 8080 }]
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: hello-web
  namespace: $NS
  annotations: { traefik.ingress.kubernetes.io/router.middlewares: $NS-sandbox-auth@kubernetescrd }
spec:
  ingressClassName: traefik
  rules:
    - host: hello.cloudtyke.dev
      http: { paths: [{ path: /, pathType: Prefix, backend: { service: { name: hello-web, port: { number: 80 } } } }] }
YAML
kubectl -n "$NS" rollout restart deploy/hello-web >/dev/null
kubectl -n "$NS" rollout status deploy/hello-web --timeout=180s
echo "hello-web: https://hello.cloudtyke.dev (sign in as cloudtyke / portal_password from reports/.cloudtyke-dev-credentials)"
