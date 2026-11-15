# ================================================================ the pitch (changes per event)
#
# Everything kit-specific lives here, in its own resource group per event.
# Each event picks one kit from the catalogue below. Change an event's kit in
# var.events and only that event's pitch changes: the node pool, the media
# storage rules and the score-book database. Gates, seats, the control room
# and the safety code are untouched (verify.sh proves it with a plan).

locals {
  kits = {
    # 90 minutes, a huge rush at kick-off and half-time.
    football = {
      pool      = { vm_size = "Standard_D4s_v5", autoscale = true, min = 3, max = 30, count = 3 }
      media     = "highlights"
      lifecycle = { cool_after = null, archive_after = null, delete_after = 7 } # clips kept a short time
      database  = false
    }
    # Up to five days, rain stops play. Small and steady; stats kept for years.
    cricket = {
      pool      = { vm_size = "Standard_D2s_v5", autoscale = false, min = null, max = null, count = 2 }
      media     = "ball-tracking"
      lifecycle = { cool_after = 30, archive_after = 90, delete_after = null } # replays go cool, then archive
      database  = true
    }
    # About three hours, a big rush at doors-open. No database.
    concert = {
      pool      = { vm_size = "Standard_D4s_v5", autoscale = true, min = 2, max = 20, count = 2 }
      media     = "stage-media"
      lifecycle = { cool_after = null, archive_after = null, delete_after = 30 } # media deleted after 30 days
      database  = false
    }
  }

  pitch = { for e, cfg in var.events : e => merge(cfg, { spec = local.kits[cfg.kit] }) }

  pitch_tags = {
    for e, cfg in var.events : e => merge(local.base_tags, {
      event       = e
      event-kit   = cfg.kit
      cost-centre = cfg.cost_centre
    })
  }

  pitch_access = merge([
    for e, cfg in var.events : {
      for g in ["owners", "contributors", "readers"] : "${e}-${g}" => { event = e, group = "${e}-${g}" }
    }
  ]...)
}

resource "azurerm_resource_group" "pitch" {
  for_each = var.events
  name     = "rg-ct-${each.key}-pitch-${local.region_code}"
  location = var.location
  tags     = local.pitch_tags[each.key]

  depends_on = [azurerm_management_group_policy_assignment.safety_code]
}

resource "azurerm_role_assignment" "pitch_access" {
  for_each             = local.pitch_access
  scope                = azurerm_resource_group.pitch[each.value.event].id
  role_definition_name = local.seats_groups[each.value.group].role
  principal_id         = azuread_group.seats[each.value.group].object_id
}

# The changeover crew rebuilds the pitch, so it may change the pitch only.
resource "azurerm_role_assignment" "pitch_crew" {
  for_each             = var.events
  scope                = azurerm_resource_group.pitch[each.key].id
  role_definition_name = "Contributor"
  principal_id         = azurerm_user_assigned_identity.seats_crew[each.key].principal_id
}

# ---------------------------------------------------------------- the team: AKS on the pitch

resource "azurerm_user_assigned_identity" "pitch_aks" {
  for_each            = var.events
  name                = "id-ct-${each.key}-aks"
  location            = azurerm_resource_group.pitch[each.key].location
  resource_group_name = azurerm_resource_group.pitch[each.key].name
  tags                = local.pitch_tags[each.key]
}

# AKS manages IP configurations in the seats subnet and reads its route table.
resource "azurerm_role_assignment" "pitch_aks_network" {
  for_each             = var.events
  scope                = azurerm_virtual_network.seats[each.key].id
  role_definition_name = "Network Contributor"
  principal_id         = azurerm_user_assigned_identity.pitch_aks[each.key].principal_id
}

resource "azurerm_kubernetes_cluster" "pitch" {
  for_each            = var.events
  name                = "aks-ct-${each.key}-${local.region_code}"
  location            = azurerm_resource_group.pitch[each.key].location
  resource_group_name = azurerm_resource_group.pitch[each.key].name
  dns_prefix          = "ct-${each.key}"
  sku_tier            = "Standard"
  tags                = local.pitch_tags[each.key]

  oidc_issuer_enabled       = true
  workload_identity_enabled = true
  local_account_disabled    = true # the safety code insists
  azure_policy_enabled      = true # the referee on the pitch

  # The coaching staff: a small system pool. The players live in the
  # kit-specific "event" pool below.
  default_node_pool {
    name                         = "system"
    vm_size                      = "Standard_D2s_v5"
    vnet_subnet_id               = azurerm_subnet.seats["${each.key}/snet-aks"].id
    node_count                   = 2
    zones                        = ["1", "2", "3"]
    only_critical_addons_enabled = true

    upgrade_settings {
      max_surge = "33%"
    }
  }

  node_provisioning_profile {
    mode = "Manual"
  }

  identity {
    type         = "UserAssigned"
    identity_ids = [azurerm_user_assigned_identity.pitch_aks[each.key].id]
  }

  # No public way out: egress follows the seats' route table to the bag check.
  network_profile {
    network_plugin      = "azure"
    network_plugin_mode = "overlay"
    network_policy      = "azure"
    outbound_type       = "userDefinedRouting"
    pod_cidr            = "192.168.0.0/16"
    service_cidr        = "172.16.0.0/16"
    dns_service_ip      = "172.16.0.10"
  }

  azure_active_directory_role_based_access_control {
    tenant_id              = data.azurerm_client_config.current.tenant_id
    admin_group_object_ids = [azuread_group.seats["${each.key}-owners"].object_id]
    azure_rbac_enabled     = true
  }

  # Every event plugs its cameras into the same control room.
  oms_agent {
    log_analytics_workspace_id      = azurerm_log_analytics_workspace.control_room.id
    msi_auth_for_monitoring_enabled = true
  }

  # userDefinedRouting fails unless the subnet already has its route table.
  depends_on = [
    azurerm_subnet_route_table_association.seats,
    azurerm_role_assignment.pitch_aks_network,
  ]
}

# The squad: sized by the kit. Autoscaling needs min and max, and the count
# must sit between them.
resource "azurerm_kubernetes_cluster_node_pool" "pitch" {
  for_each              = local.pitch
  name                  = "event"
  kubernetes_cluster_id = azurerm_kubernetes_cluster.pitch[each.key].id
  mode                  = "User"
  vm_size               = each.value.spec.pool.vm_size
  vnet_subnet_id        = azurerm_subnet.seats["${each.key}/snet-aks"].id
  zones                 = ["1", "2", "3"]
  auto_scaling_enabled  = each.value.spec.pool.autoscale
  node_count            = each.value.spec.pool.count
  min_count             = each.value.spec.pool.min
  max_count             = each.value.spec.pool.max
  node_labels           = { "arena/event-kit" = each.value.kit }
  tags                  = local.pitch_tags[each.key]
}

resource "azurerm_role_assignment" "pitch_aks_kit_room" {
  for_each                         = var.events
  scope                            = azurerm_container_registry.kit_room.id
  role_definition_name             = "AcrPull"
  principal_id                     = azurerm_kubernetes_cluster.pitch[each.key].kubelet_identity[0].object_id
  skip_service_principal_aad_check = true
}

resource "azurerm_role_assignment" "pitch_aks_writers" {
  for_each             = var.events
  scope                = azurerm_kubernetes_cluster.pitch[each.key].id
  role_definition_name = "Azure Kubernetes Service RBAC Writer"
  principal_id         = azuread_group.seats["${each.key}-contributors"].object_id
}

# ---------------------------------------------------------------- the vault: media storage with kit-specific rules

resource "azurerm_storage_account" "pitch" {
  for_each                        = var.events
  name                            = "stct${each.key}${random_string.suffix.result}" # at most 18 characters
  location                        = azurerm_resource_group.pitch[each.key].location
  resource_group_name             = azurerm_resource_group.pitch[each.key].name
  account_tier                    = "Standard"
  account_kind                    = "StorageV2"
  account_replication_type        = "ZRS"
  min_tls_version                 = "TLS1_2"
  https_traffic_only_enabled      = true
  allow_nested_items_to_be_public = false
  shared_access_key_enabled       = false # wristbands only
  default_to_oauth_authentication = true
  tags                            = local.pitch_tags[each.key]
}

resource "azurerm_storage_container" "pitch" {
  for_each              = local.pitch
  name                  = each.value.spec.media
  storage_account_id    = azurerm_storage_account.pitch[each.key].id
  container_access_type = "private"
}

resource "azurerm_storage_management_policy" "pitch" {
  for_each           = local.pitch
  storage_account_id = azurerm_storage_account.pitch[each.key].id

  rule {
    name    = "${each.value.kit}-media"
    enabled = true
    filters {
      blob_types   = ["blockBlob"]
      prefix_match = ["${each.value.spec.media}/"]
    }
    actions {
      base_blob {
        tier_to_cool_after_days_since_modification_greater_than    = each.value.spec.lifecycle.cool_after
        tier_to_archive_after_days_since_modification_greater_than = each.value.spec.lifecycle.archive_after
        delete_after_days_since_modification_greater_than          = each.value.spec.lifecycle.delete_after
      }
    }
  }
}

# ---------------------------------------------------------------- the score book: PostgreSQL (cricket only)

locals {
  pitch_scorebooks = { for e, p in local.pitch : e => p if p.spec.database }
}

resource "random_password" "pitch_scorebook" {
  for_each         = local.pitch_scorebooks
  length           = 24
  min_upper        = 2
  min_lower        = 2
  min_numeric      = 2
  min_special      = 2
  override_special = "!#%+=?@"
}

resource "azurerm_postgresql_flexible_server" "pitch_scorebook" {
  for_each                      = local.pitch_scorebooks
  name                          = "psql-ct-${each.key}-${random_string.suffix.result}"
  location                      = azurerm_resource_group.pitch[each.key].location
  resource_group_name           = azurerm_resource_group.pitch[each.key].name
  version                       = "16"
  sku_name                      = "B_Standard_B1ms"
  storage_mb                    = 32768
  backup_retention_days         = 35
  zone                          = "1"
  delegated_subnet_id           = azurerm_subnet.seats["${each.key}/snet-data"].id
  private_dns_zone_id           = azurerm_private_dns_zone.gates["privatelink.postgres.database.azure.com"].id
  public_network_access_enabled = false # the safety code insists
  administrator_login           = "scorer"
  administrator_password        = random_password.pitch_scorebook[each.key].result
  tags                          = local.pitch_tags[each.key]

  authentication {
    active_directory_auth_enabled = true
    password_auth_enabled         = true
    tenant_id                     = data.azurerm_client_config.current.tenant_id
  }

  depends_on = [azurerm_private_dns_zone_virtual_network_link.seats]
}

resource "azurerm_postgresql_flexible_server_database" "pitch_scorebook" {
  for_each  = local.pitch_scorebooks
  name      = "scorecards"
  server_id = azurerm_postgresql_flexible_server.pitch_scorebook[each.key].id
  charset   = "UTF8"
  collation = "en_GB.utf8"
}

# The admin password goes straight into the event's lockers. Nobody sees it.
resource "azurerm_role_assignment" "pitch_deployer_lockers" {
  for_each             = local.pitch_scorebooks
  scope                = azurerm_key_vault.seats[each.key].id
  role_definition_name = "Key Vault Secrets Officer"
  principal_id         = data.azurerm_client_config.current.object_id
}

resource "azurerm_key_vault_secret" "pitch_scorebook" {
  for_each     = local.pitch_scorebooks
  name         = "scorebook-admin-password"
  value        = random_password.pitch_scorebook[each.key].result
  key_vault_id = azurerm_key_vault.seats[each.key].id

  depends_on = [azurerm_role_assignment.pitch_deployer_lockers]
}
