# ================================================================ Merchant Experience: onboarding, portal and the demo storefront
#
# Merchants (the fictional Northwind Airways among them) sign up through the
# merchant portal. KYC documents land in an immutable container; merchant API
# keys live in the merchant team's own vault.

resource "azurerm_resource_group" "merchant" {
  name     = "rg-cpay-merchant-${var.environment}-${local.rc}"
  location = var.location
  tags     = local.team_tags.merchant
}

resource "azurerm_key_vault" "merchant" {
  name                       = "kv-cpay-mrch-${var.environment}-${local.sfx}"
  location                   = var.location
  resource_group_name        = azurerm_resource_group.merchant.name
  tenant_id                  = data.azurerm_client_config.current.tenant_id
  sku_name                   = "standard"
  rbac_authorization_enabled = true
  purge_protection_enabled   = true
  soft_delete_retention_days = 30
  tags                       = local.team_tags.merchant
}

resource "azurerm_role_assignment" "deployer_merchant_kv" {
  scope                = azurerm_key_vault.merchant.id
  role_definition_name = "Key Vault Secrets Officer"
  principal_id         = data.azurerm_client_config.current.object_id
}

resource "azurerm_role_assignment" "merchant_engineers_kv" {
  scope                = azurerm_key_vault.merchant.id
  role_definition_name = "Key Vault Secrets User"
  principal_id         = azuread_group.teams["merchant-engineers"].object_id
}

# Invented merchant credentials for the demo storefront.
locals {
  merchants = {
    northwind-airways = { mcc = "4511", country = "GB" }
    fabrikam-hotels   = { mcc = "7011", country = "GB" }
    tailspin-toys     = { mcc = "5945", country = "IE" }
  }
}

resource "random_password" "merchant_api_key" {
  for_each = local.merchants
  length   = 40
  special  = false
}

resource "azurerm_key_vault_secret" "merchant_api_key" {
  for_each     = local.merchants
  name         = "${each.key}-api-key"
  value        = "sk_test_${random_password.merchant_api_key[each.key].result}"
  key_vault_id = azurerm_key_vault.merchant.id
  content_type = "text/plain"
  tags         = { merchant = each.key, mcc = each.value.mcc, country = each.value.country }
  depends_on   = [azurerm_role_assignment.deployer_merchant_kv]
}

resource "azurerm_storage_account" "kyc" {
  name                            = "stcpaykyc${var.environment}${local.sfx}"
  location                        = var.location
  resource_group_name             = azurerm_resource_group.merchant.name
  account_tier                    = "Standard"
  account_replication_type        = "ZRS"
  min_tls_version                 = "TLS1_2"
  allow_nested_items_to_be_public = false
  shared_access_key_enabled       = false
  default_to_oauth_authentication = true
  tags                            = local.team_tags.merchant

  blob_properties {
    versioning_enabled = true
    delete_retention_policy {
      days = 30
    }
  }
}

resource "azurerm_storage_container" "kyc" {
  for_each              = toset(["kyc-documents", "storefront-assets"])
  name                  = each.key
  storage_account_id    = azurerm_storage_account.kyc.id
  container_access_type = "private"
}

locals {
  merchant_workloads = {
    merchant-portal = { namespace = "merchant", sa = "merchant-portal" }
    storefront      = { namespace = "storefront", sa = "northwind-storefront" }
  }
}

resource "azurerm_user_assigned_identity" "merchant" {
  for_each            = local.merchant_workloads
  name                = "id-cpay-${each.key}-${var.environment}"
  location            = var.location
  resource_group_name = azurerm_resource_group.merchant.name
  tags                = local.team_tags.merchant
}

resource "azurerm_federated_identity_credential" "merchant" {
  for_each                  = local.merchant_workloads
  name                      = "aks-${each.value.namespace}-${each.value.sa}"
  user_assigned_identity_id = azurerm_user_assigned_identity.merchant[each.key].id
  audience                  = ["api://AzureADTokenExchange"]
  issuer                    = azurerm_kubernetes_cluster.payments.oidc_issuer_url
  subject                   = "system:serviceaccount:${each.value.namespace}:${each.value.sa}"
}

resource "azurerm_role_assignment" "merchant_kv_user" {
  for_each             = local.merchant_workloads
  scope                = azurerm_key_vault.merchant.id
  role_definition_name = "Key Vault Secrets User"
  principal_id         = azurerm_user_assigned_identity.merchant[each.key].principal_id
}

resource "azurerm_role_assignment" "portal_kyc" {
  scope                = azurerm_storage_account.kyc.id
  role_definition_name = "Storage Blob Data Contributor"
  principal_id         = azurerm_user_assigned_identity.merchant["merchant-portal"].principal_id
}

resource "azurerm_role_assignment" "merchant_readers" {
  scope                = azurerm_resource_group.merchant.id
  role_definition_name = "Reader"
  principal_id         = azuread_group.teams["merchant-readers"].object_id
}

# The gateway validates merchant API keys against this vault.
resource "azurerm_role_assignment" "gateway_merchant_kv" {
  scope                = azurerm_key_vault.merchant.id
  role_definition_name = "Key Vault Secrets User"
  principal_id         = azurerm_user_assigned_identity.payments["gateway"].principal_id
}
