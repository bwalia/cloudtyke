# ================================================================ Payments Core: the cardholder data environment
#
# The payment gateway keeps transactions in PostgreSQL (private, Entra admins),
# writes high-volume events to Cosmos DB for MongoDB, stores signed receipts in
# Blob storage and reads every credential from Key Vault through workload
# identity. No secret ever lives in a Kubernetes manifest.

resource "azurerm_resource_group" "payments" {
  name     = "rg-cpay-payments-${var.environment}-${local.rc}"
  location = var.location
  tags     = local.team_tags.payments
}

# ---------------------------------------------------------------- Key Vault
resource "azurerm_key_vault" "payments" {
  name                          = "kv-cpay-pay-${var.environment}-${local.sfx}"
  location                      = var.location
  resource_group_name           = azurerm_resource_group.payments.name
  tenant_id                     = data.azurerm_client_config.current.tenant_id
  sku_name                      = "premium"
  rbac_authorization_enabled    = true
  purge_protection_enabled      = true
  soft_delete_retention_days    = 90
  tags                          = local.team_tags.payments

  # Reachable only from the AKS subnet (service endpoint) and the platform
  # pipeline's egress IP (a documentation address).
  network_acls {
    bypass                     = "AzureServices"
    default_action             = "Deny"
    ip_rules                   = ["203.0.113.10/32"]
    virtual_network_subnet_ids = [azurerm_subnet.aks.id]
  }
}

# Terraform (the platform pipeline) seeds secrets; people never see them.
resource "azurerm_role_assignment" "deployer_payments_kv" {
  scope                = azurerm_key_vault.payments.id
  role_definition_name = "Key Vault Secrets Officer"
  principal_id         = data.azurerm_client_config.current.object_id
}

resource "azurerm_role_assignment" "payments_leads_kv" {
  scope                = azurerm_key_vault.payments.id
  role_definition_name = "Key Vault Secrets Officer"
  principal_id         = azuread_group.teams["payments-leads"].object_id
}

resource "azurerm_role_assignment" "secops_kv_reader" {
  scope                = azurerm_key_vault.payments.id
  role_definition_name = "Key Vault Reader" # metadata only, never values
  principal_id         = azuread_group.teams["security-readers"].object_id
}

# ---------------------------------------------------------------- PostgreSQL (ledger of record)
resource "random_password" "pg_admin" {
  length           = 32
  min_upper        = 2
  min_lower        = 2
  min_numeric      = 2
  min_special      = 2
  override_special = "!#%+=?"
}

resource "azurerm_postgresql_flexible_server" "payments" {
  name                          = "psql-cpay-payments-${var.environment}-${local.sfx}"
  location                      = var.location
  resource_group_name           = azurerm_resource_group.payments.name
  version                       = "16"
  sku_name                      = "GP_Standard_D4ds_v5"
  storage_mb                    = 131072
  backup_retention_days         = 35
  geo_redundant_backup_enabled  = true
  zone                          = "1"
  delegated_subnet_id           = azurerm_subnet.postgres.id
  private_dns_zone_id           = azurerm_private_dns_zone.zones["cpay.private.postgres.database.azure.com"].id
  public_network_access_enabled = false
  administrator_login           = "cpayadmin"
  administrator_password        = random_password.pg_admin.result
  tags                          = merge(local.team_tags.payments, { protect = tostring(var.protect_data) })

  authentication {
    active_directory_auth_enabled = true
    password_auth_enabled         = true
    tenant_id                     = data.azurerm_client_config.current.tenant_id
  }

  high_availability {
    mode                      = "ZoneRedundant"
    standby_availability_zone = "2"
  }

  maintenance_window {
    day_of_week  = 0
    start_hour   = 2
    start_minute = 0
  }

  depends_on = [azurerm_private_dns_zone_virtual_network_link.spoke]
}

resource "azurerm_postgresql_flexible_server_active_directory_administrator" "dba" {
  server_name         = azurerm_postgresql_flexible_server.payments.name
  resource_group_name = azurerm_resource_group.payments.name
  tenant_id           = data.azurerm_client_config.current.tenant_id
  object_id           = azuread_group.teams["payments-dba"].object_id
  principal_name      = azuread_group.teams["payments-dba"].display_name
  principal_type      = "Group"
}

resource "azurerm_postgresql_flexible_server_database" "payments" {
  for_each  = toset(["payments", "ledger", "merchants"])
  name      = each.key
  server_id = azurerm_postgresql_flexible_server.payments.id
  charset   = "UTF8"
  collation = "en_US.utf8"
}

resource "azurerm_postgresql_flexible_server_configuration" "payments" {
  for_each = {
    "log_connections"                     = "on"
    "log_disconnections"                  = "on"
    "pgaudit.log"                         = "WRITE,DDL,ROLE" # PCI-DSS req. 10.2
    "require_secure_transport"            = "on"
    "idle_in_transaction_session_timeout" = "60000"
  }
  name      = each.key
  server_id = azurerm_postgresql_flexible_server.payments.id
  value     = each.value
}

# ---------------------------------------------------------------- Cosmos DB for MongoDB (event store)
resource "azurerm_cosmosdb_account" "events" {
  name                          = "cosmos-cpay-events-${var.environment}-${local.sfx}"
  location                      = var.location
  resource_group_name           = azurerm_resource_group.payments.name
  offer_type                    = "Standard"
  kind                          = "MongoDB"
  mongo_server_version          = "7.0"
  public_network_access_enabled = false
  minimal_tls_version           = "Tls12"
  automatic_failover_enabled    = true
  tags                          = merge(local.team_tags.payments, { protect = tostring(var.protect_data) })

  capabilities {
    name = "EnableMongo"
  }

  consistency_policy {
    consistency_level = "Session"
  }

  geo_location {
    location          = "uksouth"
    failover_priority = 0
    zone_redundant    = true
  }
  geo_location {
    location          = "ukwest"
    failover_priority = 1
  }

  backup {
    type = "Continuous"
    tier = "Continuous30Days"
  }
}

resource "azurerm_cosmosdb_mongo_database" "payments" {
  name                = "payments"
  resource_group_name = azurerm_resource_group.payments.name
  account_name        = azurerm_cosmosdb_account.events.name

  autoscale_settings {
    max_throughput = 4000
  }
}

locals {
  collections = {
    payment_events     = { shard = "merchantId", ttl = -1 }
    risk_signals       = { shard = "paymentId", ttl = 7776000 }  # 90 days
    webhook_deliveries = { shard = "merchantId", ttl = 2592000 } # 30 days
  }
}

resource "azurerm_cosmosdb_mongo_collection" "payments" {
  for_each            = local.collections
  name                = each.key
  resource_group_name = azurerm_resource_group.payments.name
  account_name        = azurerm_cosmosdb_account.events.name
  database_name       = azurerm_cosmosdb_mongo_database.payments.name
  shard_key           = each.value.shard
  default_ttl_seconds = each.value.ttl

  index {
    keys   = ["_id"]
    unique = true
  }
  index {
    keys = [each.value.shard, "createdAt"]
  }
}

resource "azurerm_role_assignment" "risk_analysts_cosmos" {
  scope                = azurerm_cosmosdb_account.events.id
  role_definition_name = "Cosmos DB Account Reader Role"
  principal_id         = azuread_group.teams["risk-analysts"].object_id
}

# ---------------------------------------------------------------- receipts (Blob)
resource "azurerm_storage_account" "receipts" {
  name                            = "stcpayrcpt${var.environment}${local.sfx}"
  location                        = var.location
  resource_group_name             = azurerm_resource_group.payments.name
  account_tier                    = "Standard"
  account_kind                    = "StorageV2"
  account_replication_type        = "GZRS"
  min_tls_version                 = "TLS1_2"
  https_traffic_only_enabled      = true
  allow_nested_items_to_be_public = false
  shared_access_key_enabled       = false
  default_to_oauth_authentication = true
  tags                            = local.team_tags.payments

  network_rules {
    default_action             = "Deny"
    bypass                     = ["AzureServices"]
    ip_rules                   = ["203.0.113.10"]
    virtual_network_subnet_ids = [azurerm_subnet.aks.id]
  }
}

resource "azurerm_storage_container" "receipts" {
  for_each              = toset(["receipts", "chargeback-evidence", "audit-exports"])
  name                  = each.key
  storage_account_id    = azurerm_storage_account.receipts.id
  container_access_type = "private"
}

# Receipts are kept for seven years (card-scheme dispute and tax rules), then deleted.
resource "azurerm_storage_management_policy" "receipts" {
  storage_account_id = azurerm_storage_account.receipts.id

  rule {
    name    = "receipts-tiering"
    enabled = true
    filters {
      blob_types   = ["blockBlob"]
      prefix_match = ["receipts/"]
    }
    actions {
      base_blob {
        tier_to_cool_after_days_since_modification_greater_than    = 30
        tier_to_archive_after_days_since_modification_greater_than = 365
        delete_after_days_since_modification_greater_than          = 2555
      }
    }
  }
}

# ---------------------------------------------------------------- workload identities
locals {
  payments_workloads = {
    gateway     = { namespace = "payments", sa = "payment-gateway" }
    risk-engine = { namespace = "risk", sa = "risk-engine" }
  }
}

resource "azurerm_user_assigned_identity" "payments" {
  for_each            = local.payments_workloads
  name                = "id-cpay-${each.key}-${var.environment}"
  location            = var.location
  resource_group_name = azurerm_resource_group.payments.name
  tags                = local.team_tags.payments
}

resource "azurerm_federated_identity_credential" "payments" {
  for_each                  = local.payments_workloads
  name                      = "aks-${each.value.namespace}-${each.value.sa}"
  user_assigned_identity_id = azurerm_user_assigned_identity.payments[each.key].id
  audience                  = ["api://AzureADTokenExchange"]
  issuer                    = azurerm_kubernetes_cluster.payments.oidc_issuer_url
  subject                   = "system:serviceaccount:${each.value.namespace}:${each.value.sa}"
}

resource "azurerm_role_assignment" "payments_kv_user" {
  for_each             = local.payments_workloads
  scope                = azurerm_key_vault.payments.id
  role_definition_name = "Key Vault Secrets User"
  principal_id         = azurerm_user_assigned_identity.payments[each.key].principal_id
}

resource "azurerm_role_assignment" "gateway_receipts" {
  scope                = azurerm_storage_account.receipts.id
  role_definition_name = "Storage Blob Data Contributor"
  principal_id         = azurerm_user_assigned_identity.payments["gateway"].principal_id
}

resource "azurerm_role_assignment" "secops_audit_exports" {
  scope                = "${azurerm_storage_account.receipts.id}/blobServices/default/containers/audit-exports"
  role_definition_name = "Storage Blob Data Reader"
  principal_id         = azuread_group.teams["security-readers"].object_id
  depends_on           = [azurerm_storage_container.receipts]
}

# ---------------------------------------------------------------- secrets the apps read at runtime
resource "random_password" "app" {
  for_each = toset(["webhook-signing-key", "pan-tokenisation-key", "acquirer-api-key"])
  length   = 48
  special  = false
}

resource "azurerm_key_vault_secret" "payments" {
  for_each = {
    "postgres-admin-password" = random_password.pg_admin.result
    "postgres-host"           = azurerm_postgresql_flexible_server.payments.fqdn
    "cosmos-mongo-connection" = azurerm_cosmosdb_account.events.primary_mongodb_connection_string
    "webhook-signing-key"     = random_password.app["webhook-signing-key"].result
    "pan-tokenisation-key"    = random_password.app["pan-tokenisation-key"].result
    "acquirer-api-key"        = random_password.app["acquirer-api-key"].result
  }
  name            = each.key
  value           = each.value
  key_vault_id    = azurerm_key_vault.payments.id
  content_type    = "text/plain"
  expiration_date = "2027-11-15T00:00:00Z"
  tags            = { owner = "payments-core", rotation = "90d" }
  depends_on      = [azurerm_role_assignment.deployer_payments_kv]
}
