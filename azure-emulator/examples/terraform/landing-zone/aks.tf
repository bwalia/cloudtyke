# ================================================================ corp landing zones run AKS
#
# Corp clusters have no public egress: outbound traffic follows the spoke
# route table to the hub firewall (outboundType userDefinedRouting). AKS
# refuses to create the cluster unless that route table is on the subnet.

locals {
  corp = { for lz, cfg in var.landing_zones : lz => cfg if cfg.archetype == "corp" }
}

resource "azurerm_user_assigned_identity" "aks" {
  for_each            = local.corp
  name                = "id-${var.prefix}-lz-${each.key}-aks"
  location            = azurerm_resource_group.lz[each.key].location
  resource_group_name = azurerm_resource_group.lz[each.key].name
  tags                = local.lz_tags[each.key]
}

# AKS manages the subnet's IP configurations and reads the route table.
resource "azurerm_role_assignment" "aks_network" {
  for_each             = local.corp
  scope                = azurerm_virtual_network.lz[each.key].id
  role_definition_name = "Network Contributor"
  principal_id         = azurerm_user_assigned_identity.aks[each.key].principal_id
}

resource "azurerm_kubernetes_cluster" "lz" {
  for_each            = local.corp
  name                = "aks-${var.prefix}-lz-${each.key}-${local.region_code}"
  location            = azurerm_resource_group.lz[each.key].location
  resource_group_name = azurerm_resource_group.lz[each.key].name
  dns_prefix          = "${var.prefix}-${each.key}"
  sku_tier            = "Standard"
  tags                = local.lz_tags[each.key]

  oidc_issuer_enabled       = true
  workload_identity_enabled = true
  local_account_disabled    = true
  azure_policy_enabled      = true

  default_node_pool {
    name                         = "system"
    vm_size                      = "Standard_D4s_v5"
    vnet_subnet_id               = azurerm_subnet.lz["${each.key}/snet-aks"].id
    node_count                   = 3
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
    identity_ids = [azurerm_user_assigned_identity.aks[each.key].id]
  }

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
    admin_group_object_ids = [azuread_group.this["lz-${each.key}-owners"].object_id]
    azure_rbac_enabled     = true
  }

  oms_agent {
    log_analytics_workspace_id      = azurerm_log_analytics_workspace.platform.id
    msi_auth_for_monitoring_enabled = true
  }

  depends_on = [
    azurerm_subnet_route_table_association.lz,
    azurerm_role_assignment.aks_network,
  ]
}

resource "azurerm_role_assignment" "aks_acr_pull" {
  for_each                         = local.corp
  scope                            = azurerm_container_registry.shared.id
  role_definition_name             = "AcrPull"
  principal_id                     = azurerm_kubernetes_cluster.lz[each.key].kubelet_identity[0].object_id
  skip_service_principal_aad_check = true
}

# Contributors operate workloads with Kubernetes RBAC through Azure RBAC.
resource "azurerm_role_assignment" "aks_contributors" {
  for_each             = local.corp
  scope                = azurerm_kubernetes_cluster.lz[each.key].id
  role_definition_name = "Azure Kubernetes Service RBAC Writer"
  principal_id         = azuread_group.this["lz-${each.key}-contributors"].object_id
}
