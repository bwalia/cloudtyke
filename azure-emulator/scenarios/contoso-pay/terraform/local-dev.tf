# ================================================================ local development identity
#
# On AKS every workload signs in with workload identity (see the federated
# credentials in payments.tf, merchant.tf and settlement.tf). When the same apps
# run on a laptop with docker compose there is no Kubernetes token to
# exchange, so the platform team issues one tightly-scoped service principal
# with a short-lived secret, the way most teams run "local against dev".

resource "azuread_application" "local_dev" {
  display_name = "app-cpay-local-dev"
  owners       = [data.azurerm_client_config.current.object_id]
  notes        = "Used by the demo apps under docker compose. Secret expires after 30 days."
}

resource "azuread_service_principal" "local_dev" {
  client_id = azuread_application.local_dev.client_id
  owners    = [data.azurerm_client_config.current.object_id]
}

resource "azuread_application_password" "local_dev" {
  application_id = azuread_application.local_dev.id
  display_name   = "docker-compose"
  end_date       = "2026-12-31T00:00:00Z"
}

locals {
  local_dev_roles = {
    payments-kv   = { scope = azurerm_key_vault.payments.id, role = "Key Vault Secrets User" }
    merchant-kv   = { scope = azurerm_key_vault.merchant.id, role = "Key Vault Secrets User" }
    receipts-blob = { scope = azurerm_storage_account.receipts.id, role = "Storage Blob Data Contributor" }
    kyc-blob      = { scope = azurerm_storage_account.kyc.id, role = "Storage Blob Data Contributor" }
    settle-files  = { scope = azurerm_storage_account.settlement.id, role = "Storage File Data Privileged Contributor" }
  }
}

resource "azurerm_role_assignment" "local_dev" {
  for_each             = local.local_dev_roles
  scope                = each.value.scope
  role_definition_name = each.value.role
  principal_id         = azuread_service_principal.local_dev.object_id
  principal_type       = "ServicePrincipal"
}
