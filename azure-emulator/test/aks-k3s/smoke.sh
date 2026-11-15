#!/bin/sh
# AKS on k3s, end to end (run in the toolbox after `make up-k3s`):
#   ARM creates a cluster -> kubectl against its real API -> a service account token
#   is exchanged for an Entra token (workload identity) -> RBAC allows read, denies write.

set -e
M=https://management.cloudtyke.local
S=/subscriptions/$ARM_SUBSCRIPTION_ID
T=$(curl -fsS https://login.cloudtyke.local/$ARM_TENANT_ID/oauth2/v2.0/token -d grant_type=client_credentials -d client_id=$ARM_CLIENT_ID -d client_secret=$ARM_CLIENT_SECRET -d scope=$M/.default | jq -r .access_token)
H="Authorization: Bearer $T"
curl -fsS -X PUT -H "$H" -H 'content-type: application/json' "$M$S/resourceGroups/rg-k3s-smoke?api-version=2024-11-01" -d '{"location":"uksouth"}' >/dev/null
curl -fsS -X PUT -H "$H" -H 'content-type: application/json' "$M$S/resourceGroups/rg-k3s-smoke/providers/Microsoft.ContainerService/managedClusters/aks-k3s?api-version=2025-02-01" \
  -d '{"location":"uksouth","identity":{"type":"SystemAssigned"},"properties":{"dnsPrefix":"k3s","oidcIssuerProfile":{"enabled":true},"agentPoolProfiles":[{"name":"system","count":1,"vmSize":"Standard_D4s_v5","mode":"System"}]}}' | jq -r '.properties.fqdn'
curl -fsS -X POST -H "$H" "$M$S/resourceGroups/rg-k3s-smoke/providers/Microsoft.ContainerService/managedClusters/aks-k3s/listClusterAdminCredential?api-version=2025-02-01" | jq -r '.kubeconfigs[0].value' | base64 -d > /tmp/kc
export KUBECONFIG=/tmp/kc
for i in $(seq 1 60); do kubectl get --raw /readyz >/dev/null 2>&1 && break; sleep 3; done
kubectl version 2>/dev/null | tail -1
kubectl get nodes -o wide
kubectl create namespace orders --dry-run=client -o yaml | kubectl apply -f -
kubectl -n orders create deployment hello --image=nginx:alpine --replicas=1 --dry-run=client -o yaml | kubectl apply -f -
kubectl -n orders create serviceaccount orders-api --dry-run=client -o yaml | kubectl apply -f -
kubectl -n orders create token orders-api --audience api://AzureADTokenExchange | cut -d. -f2 | tr '_-' '/+' | base64 -d 2>/dev/null | jq -c '{iss,sub,aud}'
kubectl get ns

echo "--- workload identity: service account token -> Entra token -> ARM"
C="$M$S/resourceGroups/rg-k3s-smoke/providers/Microsoft.ContainerService/managedClusters/aks-k3s?api-version=2025-02-01"
ISS=$(curl -fsS -H "$H" "$C" | jq -r .properties.oidcIssuerProfile.issuerURL)
U="$M$S/resourceGroups/rg-k3s-smoke/providers/Microsoft.ManagedIdentity/userAssignedIdentities/id-orders-api"
UAI=$(curl -fsS -X PUT -H "$H" -H 'content-type: application/json' "$U?api-version=2023-01-31" -d '{"location":"uksouth"}')
CID=$(echo "$UAI" | jq -r .properties.clientId); PID=$(echo "$UAI" | jq -r .properties.principalId)
curl -fsS -X PUT -H "$H" -H 'content-type: application/json' "$U/federatedIdentityCredentials/orders?api-version=2023-01-31" \
  -d "{\"properties\":{\"issuer\":\"$ISS\",\"subject\":\"system:serviceaccount:orders:orders-api\",\"audiences\":[\"api://AzureADTokenExchange\"]}}" >/dev/null
curl -fsS -X PUT -H "$H" -H 'content-type: application/json' "$M$S/resourceGroups/rg-k3s-smoke/providers/Microsoft.Authorization/roleAssignments/$(cat /proc/sys/kernel/random/uuid)?api-version=2022-04-01" \
  -d "{\"properties\":{\"roleDefinitionId\":\"$S/providers/Microsoft.Authorization/roleDefinitions/acdd72a7-3385-48ef-bd42-f606fba81ae7\",\"principalId\":\"$PID\",\"principalType\":\"ServicePrincipal\"}}" >/dev/null
SA=$(kubectl -n orders create token orders-api --audience api://AzureADTokenExchange)
WT=$(curl -sS https://login.cloudtyke.local/$ARM_TENANT_ID/oauth2/v2.0/token -d grant_type=client_credentials -d client_id=$CID \
  -d client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer -d client_assertion=$SA -d scope=$M/.default)
echo "$WT" | jq -c 'if .access_token then {token: "issued", expires_in} else . end'
AT=$(echo "$WT" | jq -r .access_token)
curl -sS -o /dev/null -w "pod identity reads its resource group: HTTP %{http_code}\n" -H "Authorization: Bearer $AT" "$M$S/resourceGroups/rg-k3s-smoke?api-version=2024-11-01"
curl -sS -o /dev/null -w "pod identity cannot write: HTTP %{http_code}\n" -X PUT -H "Authorization: Bearer $AT" -H 'content-type: application/json' "$M$S/resourceGroups/rg-k3s-smoke-2?api-version=2024-11-01" -d '{"location":"uksouth"}'

echo "--- checks"
test "$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $AT" "$M$S/resourceGroups/rg-k3s-smoke?api-version=2024-11-01")" = 200
test "$(curl -sS -o /dev/null -w '%{http_code}' -X PUT -H "Authorization: Bearer $AT" -H 'content-type: application/json' "$M$S/resourceGroups/rg-k3s-smoke-2?api-version=2024-11-01" -d '{"location":"uksouth"}')" = 403
curl -fsS -o /dev/null -X DELETE -H "$H" "$M$S/resourceGroups/rg-k3s-smoke?api-version=2024-11-01"
echo "AKS K3S SMOKE PASSED"
