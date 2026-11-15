# ================================================================ shared platform: logging, registry, AKS

resource "azurerm_resource_group" "observability" {
  provider = azurerm.management
  name     = "rg-cpay-observability-${local.rc}"
  location = var.location
  tags     = local.team_tags.sre
}

resource "azurerm_log_analytics_workspace" "platform" {
  provider            = azurerm.management
  name                = "log-cpay-platform-${local.rc}"
  location            = var.location
  resource_group_name = azurerm_resource_group.observability.name
  sku                 = "PerGB2018"
  retention_in_days   = 365 # PCI-DSS req. 10.5.1: one year of audit history
  tags                = local.team_tags.sre
}

resource "azurerm_resource_group" "platform" {
  name     = "rg-cpay-platform-${var.environment}-${local.rc}"
  location = var.location
  tags     = local.team_tags.platform
}

resource "azurerm_container_registry" "platform" {
  name                = "acrcpay${var.environment}${local.sfx}"
  location            = var.location
  resource_group_name = azurerm_resource_group.platform.name
  sku                 = "Premium"
  admin_enabled       = false
  tags                = local.team_tags.platform
}

resource "azurerm_user_assigned_identity" "aks" {
  name                = "id-cpay-aks-${var.environment}"
  location            = var.location
  resource_group_name = azurerm_resource_group.platform.name
  tags                = local.team_tags.platform
}

resource "azurerm_role_assignment" "aks_network" {
  scope                = azurerm_virtual_network.spoke.id
  role_definition_name = "Network Contributor"
  principal_id         = azurerm_user_assigned_identity.aks.principal_id
}

resource "azurerm_kubernetes_cluster" "payments" {
  name                = "aks-cpay-${var.environment}-${local.rc}"
  location            = var.location
  resource_group_name = azurerm_resource_group.platform.name
  dns_prefix          = "cpay-${var.environment}"
  kubernetes_version  = "1.34"
  sku_tier            = "Standard"
  tags                = local.team_tags.platform

  oidc_issuer_enabled       = true
  workload_identity_enabled = true
  local_account_disabled    = true
  azure_policy_enabled      = true

  default_node_pool {
    name                         = "system"
    vm_size                      = "Standard_D4s_v5"
    vnet_subnet_id               = azurerm_subnet.aks.id
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
    identity_ids = [azurerm_user_assigned_identity.aks.id]
  }

  network_profile {
    network_plugin      = "azure"
    network_plugin_mode = "overlay"
    network_policy      = "cilium"
    network_data_plane  = "cilium"
    outbound_type       = "userDefinedRouting"
    pod_cidr            = "192.168.0.0/16"
    service_cidr        = "172.16.0.0/16"
    dns_service_ip      = "172.16.0.10"
  }

  azure_active_directory_role_based_access_control {
    tenant_id              = data.azurerm_client_config.current.tenant_id
    admin_group_object_ids = [azuread_group.teams["platform-admins"].object_id]
    azure_rbac_enabled     = true
  }

  oms_agent {
    log_analytics_workspace_id      = azurerm_log_analytics_workspace.platform.id
    msi_auth_for_monitoring_enabled = true
  }

  key_vault_secrets_provider {
    secret_rotation_enabled = true
  }

  depends_on = [azurerm_subnet_route_table_association.aks, azurerm_role_assignment.aks_network, azurerm_subscription_policy_assignment.pci]
}

# One node pool per workload tier, tainted so only that team's pods land there.
resource "azurerm_kubernetes_cluster_node_pool" "workloads" {
  for_each              = { payments = 3, merchant = 2, batch = 1 }
  name                  = each.key
  kubernetes_cluster_id = azurerm_kubernetes_cluster.payments.id
  vm_size               = each.key == "batch" ? "Standard_E4s_v5" : "Standard_D8s_v5"
  vnet_subnet_id        = azurerm_subnet.aks.id
  node_count            = each.value
  zones                 = ["1", "2", "3"]
  mode                  = "User"
  node_labels           = { workload = each.key }
  node_taints           = ["workload=${each.key}:NoSchedule"]
  tags                  = local.team_tags.platform
}

resource "azurerm_role_assignment" "kubelet_acr_pull" {
  scope                            = azurerm_container_registry.platform.id
  role_definition_name             = "AcrPull"
  principal_id                     = azurerm_kubernetes_cluster.payments.kubelet_identity[0].object_id
  skip_service_principal_aad_check = true
}

# Kubernetes access is Azure RBAC on top of Entra ID (local accounts are off):
# the platform pipeline deploys, SRE reads everything, each team writes only to
# its own namespaces.
resource "azurerm_role_assignment" "aks_deployer" {
  scope                = azurerm_kubernetes_cluster.payments.id
  role_definition_name = "Azure Kubernetes Service RBAC Cluster Admin"
  principal_id         = data.azurerm_client_config.current.object_id
}

resource "azurerm_role_assignment" "aks_sre" {
  scope                = azurerm_kubernetes_cluster.payments.id
  role_definition_name = "Azure Kubernetes Service RBAC Reader"
  principal_id         = azuread_group.teams["sre-oncall"].object_id
}

resource "azurerm_role_assignment" "aks_users" {
  for_each             = toset(["platform-engineers", "sre-oncall", "payments-engineers", "risk-engineers", "merchant-engineers", "settlement-engineers"])
  scope                = azurerm_kubernetes_cluster.payments.id
  role_definition_name = "Azure Kubernetes Service Cluster User Role" # az aks get-credentials
  principal_id         = azuread_group.teams[each.key].object_id
}

locals {
  team_namespaces = {
    "payments-engineers/payments"     = "payments-engineers"
    "risk-engineers/risk"             = "risk-engineers"
    "merchant-engineers/merchant"     = "merchant-engineers"
    "merchant-engineers/storefront"   = "merchant-engineers"
    "settlement-engineers/settlement" = "settlement-engineers"
  }
}

resource "azurerm_role_assignment" "aks_team_namespaces" {
  for_each             = local.team_namespaces
  scope                = "${azurerm_kubernetes_cluster.payments.id}/namespaces/${split("/", each.key)[1]}"
  role_definition_name = "Azure Kubernetes Service RBAC Writer"
  principal_id         = azuread_group.teams[each.value].object_id
}
