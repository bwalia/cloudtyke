# ================================================================ Data & Settlement: daily settlement files on Azure Files
#
# Every night the settlement batch nets the day's captured payments per
# merchant and writes settlement files to an Azure Files share; acquirers drop
# their clearing files into another. One legacy acquirer still mounts its
# share with a storage key over SMB, so that account carries a time-boxed,
# CISO-approved policy exemption instead of quietly weakening the baseline.

resource "azurerm_resource_group" "settlement" {
  name     = "rg-cpay-settlement-${var.environment}-${local.rc}"
  location = var.location
  tags     = local.team_tags.settlement
}

resource "azurerm_storage_account" "settlement" {
  name                            = "stcpaysettle${var.environment}${local.sfx}"
  location                        = var.location
  resource_group_name             = azurerm_resource_group.settlement.name
  account_tier                    = "Premium"
  account_kind                    = "FileStorage"
  account_replication_type        = "ZRS"
  min_tls_version                 = "TLS1_2"
  allow_nested_items_to_be_public = false
  shared_access_key_enabled       = false
  default_to_oauth_authentication = true
  tags                            = local.team_tags.settlement
}

resource "azurerm_storage_share" "settlement" {
  for_each           = { settlement-outbound = 1024, acquirer-inbound = 512, reconciliation = 256 }
  name               = each.key
  storage_account_id = azurerm_storage_account.settlement.id
  quota              = each.value
}

resource "azurerm_user_assigned_identity" "settlement" {
  name                = "id-cpay-settlement-batch-${var.environment}"
  location            = var.location
  resource_group_name = azurerm_resource_group.settlement.name
  tags                = local.team_tags.settlement
}

resource "azurerm_federated_identity_credential" "settlement" {
  name                      = "aks-settlement-settlement-batch"
  user_assigned_identity_id = azurerm_user_assigned_identity.settlement.id
  audience                  = ["api://AzureADTokenExchange"]
  issuer                    = azurerm_kubernetes_cluster.payments.oidc_issuer_url
  subject                   = "system:serviceaccount:settlement:settlement-batch"
}

resource "azurerm_role_assignment" "settlement_files" {
  scope                = azurerm_storage_account.settlement.id
  role_definition_name = "Storage File Data Privileged Contributor"
  principal_id         = azurerm_user_assigned_identity.settlement.principal_id
}

resource "azurerm_role_assignment" "settlement_payments_kv" {
  scope                = azurerm_key_vault.payments.id
  role_definition_name = "Key Vault Secrets User"
  principal_id         = azurerm_user_assigned_identity.settlement.principal_id
}

resource "azurerm_role_assignment" "settlement_engineers_smb" {
  scope                = azurerm_storage_account.settlement.id
  role_definition_name = "Storage File Data SMB Share Contributor"
  principal_id         = azuread_group.teams["settlement-engineers"].object_id
}

# ---------------------------------------------------------------- the legacy acquirer share (exempted)
resource "azurerm_resource_group" "legacy" {
  name     = "rg-cpay-legacy-acquirer-${var.environment}-${local.rc}"
  location = var.location
  tags     = merge(local.team_tags.settlement, { decommission-by = "2027-03-31", risk-ticket = "RISK-1182" })
}

resource "azurerm_resource_group_policy_exemption" "legacy_shared_key" {
  name                            = "cpay-legacy-acquirer-shared-key"
  display_name                    = "Legacy acquirer SMB share needs a storage key until migration"
  description                     = "Approved by the CISO under RISK-1182. Compensating controls: private endpoint only, key rotated every 30 days."
  resource_group_id               = azurerm_resource_group.legacy.id
  policy_assignment_id            = azurerm_subscription_policy_assignment.pci.id
  exemption_category              = "Waiver"
  policy_definition_reference_ids = ["storage-no-shared-key"]
  expires_on                      = "2027-03-31T00:00:00Z"
  metadata                        = jsonencode({ approvedBy = "rachel.green", ticket = "RISK-1182" })
}

resource "azurerm_storage_account" "legacy" {
  name                            = "stcpaylegacy${var.environment}${local.sfx}"
  location                        = var.location
  resource_group_name             = azurerm_resource_group.legacy.name
  account_tier                    = "Standard"
  account_replication_type        = "ZRS"
  min_tls_version                 = "TLS1_2"
  allow_nested_items_to_be_public = false
  shared_access_key_enabled       = true # exempted, see above
  tags                            = merge(local.team_tags.settlement, { exemption = "RISK-1182" })

  depends_on = [azurerm_resource_group_policy_exemption.legacy_shared_key]
}

resource "azurerm_storage_share" "legacy" {
  name               = "clearing-files"
  storage_account_id = azurerm_storage_account.legacy.id
  quota              = 100
}

resource "azurerm_role_assignment" "settlement_readers" {
  scope                = azurerm_resource_group.settlement.id
  role_definition_name = "Reader"
  principal_id         = azuread_group.teams["settlement-readers"].object_id
}
