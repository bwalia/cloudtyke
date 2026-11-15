#!/usr/bin/env bash
# One-time CI setup for deploying the hosted sandbox from GitHub Actions.
#
# 1. A runner scale set "k3s-linux" for bwalia/cloudtyke in namespace
#    arc-r-cloudtyke, cloned from workstation-website's (same image, DinD
#    sidecar, node placement and GitHub App secret).
# 2. A deployer ServiceAccount that may change only namespace "cloudtyke"
#    (plus read the service-account issuer discovery document), and its
#    kubeconfig in the repo secret KUBE_CONFIG_CLOUDTYKE.
#
# Run with an admin KUBECONFIG for k3s1 and gh logged in. Idempotent.
set -euo pipefail
REPO=bwalia/cloudtyke
NS=cloudtyke
ARC_NS=arc-r-cloudtyke
SRC_NS=arc-r-workstation-website

echo "==> runner scale set ($ARC_NS)"
kubectl create namespace "$ARC_NS" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n "$SRC_NS" get secret arc-github-app -o json |
  jq --arg ns "$ARC_NS" '{apiVersion, kind, type, data, metadata: {name: .metadata.name, namespace: $ns}}' |
  kubectl apply -f - >/dev/null
helm get values -n "$SRC_NS" arc -o yaml |
  sed -e "s#githubConfigUrl: .*#githubConfigUrl: https://github.com/$REPO#" \
      -e 's/^minRunners: .*/minRunners: 0/' -e 's/^maxRunners: .*/maxRunners: 3/' >"${TMPDIR:-/tmp}/arc-cloudtyke.yaml"
chart=$(helm list -n "$SRC_NS" -o json | jq -r '.[0].chart')     # gha-runner-scale-set-<version>
helm upgrade --install arc oci://ghcr.io/actions/actions-runner-controller-charts/gha-runner-scale-set \
  --version "${chart##*-}" -n "$ARC_NS" -f "${TMPDIR:-/tmp}/arc-cloudtyke.yaml" >/dev/null

echo "==> deployer service account (namespace $NS only)"
kubectl create namespace "$NS" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl apply -f - >/dev/null <<YAML
apiVersion: v1
kind: ServiceAccount
metadata: { name: ci-deployer, namespace: $NS }
---
apiVersion: v1
kind: Secret
metadata:
  name: ci-deployer-token
  namespace: $NS
  annotations: { kubernetes.io/service-account.name: ci-deployer }
type: kubernetes.io/service-account-token
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: { name: ci-deployer, namespace: $NS }
rules:
  - apiGroups: ["", apps, networking.k8s.io, traefik.io]
    resources: [configmaps, secrets, services, serviceaccounts, persistentvolumeclaims, pods, pods/log, deployments, replicasets, ingresses, middlewares]
    verbs: [get, list, watch, create, update, patch, delete]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata: { name: ci-deployer, namespace: $NS }
roleRef: { apiGroup: rbac.authorization.k8s.io, kind: Role, name: ci-deployer }
subjects: [{ kind: ServiceAccount, name: ci-deployer, namespace: $NS }]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata: { name: cloudtyke-ci-deployer }
rules:
  - nonResourceURLs: [/.well-known/openid-configuration, /openid/v1/jwks]
    verbs: [get]
  - apiGroups: [""]
    resources: [namespaces]
    resourceNames: [$NS]
    verbs: [get, patch]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata: { name: cloudtyke-ci-deployer }
roleRef: { apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: cloudtyke-ci-deployer }
subjects: [{ kind: ServiceAccount, name: ci-deployer, namespace: $NS }]
YAML
for i in $(seq 1 20); do
  tok=$(kubectl -n "$NS" get secret ci-deployer-token -o jsonpath='{.data.token}' 2>/dev/null | base64 -d) && [ -n "$tok" ] && break
  sleep 1
done
ca=$(kubectl -n "$NS" get secret ci-deployer-token -o jsonpath='{.data.ca\.crt}')
# Runners are pods in this cluster, so they reach the API at its in-cluster name.
kubeconfig=$(cat <<KC
apiVersion: v1
kind: Config
clusters: [{ name: k3s1, cluster: { server: "https://kubernetes.default.svc", certificate-authority-data: "$ca" } }]
users: [{ name: ci-deployer, user: { token: "$tok" } }]
contexts: [{ name: cloudtyke, context: { cluster: k3s1, user: ci-deployer, namespace: $NS } }]
current-context: cloudtyke
KC
)
printf '%s' "$kubeconfig" | base64 | gh secret set KUBE_CONFIG_CLOUDTYKE -R "$REPO" >/dev/null
echo "==> KUBE_CONFIG_CLOUDTYKE set on $REPO"
echo "Still needed once (Docker Hub, for image pushes from CI):"
echo "  gh secret set DOCKER_USERNAME -R $REPO -b bwalia && gh secret set DOCKER_PASSWORD -R $REPO"
