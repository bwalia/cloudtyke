output "people" {
  description = "Invented staff and their sign-in names (passwords are in state; users must change them at first sign-in)."
  value       = { for k, u in azuread_user.people : k => u.user_principal_name }
}

output "aks_cluster" {
  value = { name = azurerm_kubernetes_cluster.payments.name, resource_group = azurerm_resource_group.platform.name, oidc_issuer = azurerm_kubernetes_cluster.payments.oidc_issuer_url }
}

output "key_vaults" {
  value = { payments = azurerm_key_vault.payments.vault_uri, merchant = azurerm_key_vault.merchant.vault_uri }
}

output "data_stores" {
  value = {
    postgres_fqdn    = azurerm_postgresql_flexible_server.payments.fqdn
    cosmos_endpoint  = azurerm_cosmosdb_account.events.endpoint
    receipts_blob    = azurerm_storage_account.receipts.primary_blob_endpoint
    kyc_blob         = azurerm_storage_account.kyc.primary_blob_endpoint
    settlement_files = azurerm_storage_account.settlement.primary_file_endpoint
  }
}

output "workload_identities" {
  description = "Client IDs to annotate Kubernetes service accounts with (azure.workload.identity/client-id)."
  value = merge(
    { for k, v in azurerm_user_assigned_identity.payments : k => v.client_id },
    { for k, v in azurerm_user_assigned_identity.merchant : k => v.client_id },
    { settlement-batch = azurerm_user_assigned_identity.settlement.client_id },
  )
}

output "policy_assignment" {
  value = azurerm_subscription_policy_assignment.pci.id
}

output "local_dev" {
  description = "Service principal the docker compose apps sign in with."
  sensitive   = true
  value = {
    tenant_id     = data.azurerm_client_config.current.tenant_id
    client_id     = azuread_application.local_dev.client_id
    client_secret = azuread_application_password.local_dev.value
  }
}

output "apps" {
  description = "Everything the demo apps need to find their resources (no secrets)."
  value = {
    payments_vault   = azurerm_key_vault.payments.vault_uri
    merchant_vault   = azurerm_key_vault.merchant.vault_uri
    receipts_account = azurerm_storage_account.receipts.name
    kyc_account      = azurerm_storage_account.kyc.name
    settle_account   = azurerm_storage_account.settlement.name
    postgres_fqdn    = azurerm_postgresql_flexible_server.payments.fqdn
    cosmos_account   = azurerm_cosmosdb_account.events.name
  }
}

output "aks_cluster_id" {
  value = azurerm_kubernetes_cluster.payments.id
}
