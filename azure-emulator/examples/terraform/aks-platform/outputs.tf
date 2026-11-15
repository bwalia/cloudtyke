output "cluster_name" {
  value = azurerm_kubernetes_cluster.aks.name
}

output "cluster_fqdn" {
  value = azurerm_kubernetes_cluster.aks.fqdn
}

output "node_resource_group" {
  value = azurerm_kubernetes_cluster.aks.node_resource_group
}

output "oidc_issuer_url" {
  value = azurerm_kubernetes_cluster.aks.oidc_issuer_url
}

output "kubelet_identity_object_id" {
  value = azurerm_kubernetes_cluster.aks.kubelet_identity[0].object_id
}

output "acr_login_server" {
  value = azurerm_container_registry.aks.login_server
}

output "orders_api_client_id" {
  description = "Put this in the orders-api service account annotation azure.workload.identity/client-id."
  value       = azurerm_user_assigned_identity.orders_api.client_id
}

output "kube_admin_config" {
  value     = azurerm_kubernetes_cluster.aks.kube_admin_config_raw
  sensitive = true
}
