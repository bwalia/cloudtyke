# AKS platform: a production-shaped cluster with everything around it.
#
#   VNet + subnet (Azure CNI overlay)      Log Analytics (Container insights)
#   ACR + AcrPull for the kubelet identity user-assigned control-plane identity
#   Entra admin group + Azure RBAC for K8s OIDC issuer + workload identity
#   system pool (autoscaling, zones)       user pool for workloads
data "azurerm_client_config" "current" {}

locals {
  tags = {
    environment = "lab"
    workload    = "aks-platform"
    managed-by  = "terraform"
  }
}

resource "azurerm_resource_group" "aks" {
  name     = "rg-${var.prefix}"
  location = var.location
  tags     = local.tags
}

# ---------------------------------------------------------------- network

resource "azurerm_virtual_network" "aks" {
  name                = "vnet-${var.prefix}"
  location            = azurerm_resource_group.aks.location
  resource_group_name = azurerm_resource_group.aks.name
  address_space       = ["10.40.0.0/16"]
  tags                = local.tags
}

resource "azurerm_subnet" "nodes" {
  name                 = "snet-aks-nodes"
  resource_group_name  = azurerm_resource_group.aks.name
  virtual_network_name = azurerm_virtual_network.aks.name
  address_prefixes     = ["10.40.0.0/22"]
}

resource "azurerm_network_security_group" "nodes" {
  name                = "nsg-${var.prefix}-nodes"
  location            = azurerm_resource_group.aks.location
  resource_group_name = azurerm_resource_group.aks.name
  tags                = local.tags

  security_rule {
    name                       = "allow-https-from-vnet"
    priority                   = 100
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "443"
    source_address_prefix      = "VirtualNetwork"
    destination_address_prefix = "*"
  }
}

resource "azurerm_subnet_network_security_group_association" "nodes" {
  subnet_id                 = azurerm_subnet.nodes.id
  network_security_group_id = azurerm_network_security_group.nodes.id
}

# ---------------------------------------------------------------- monitoring + registry

resource "azurerm_log_analytics_workspace" "aks" {
  name                = "log-${var.prefix}"
  location            = azurerm_resource_group.aks.location
  resource_group_name = azurerm_resource_group.aks.name
  sku                 = "PerGB2018"
  retention_in_days   = 30
  tags                = local.tags
}

resource "azurerm_container_registry" "aks" {
  name                = replace("acr${var.prefix}", "-", "")
  location            = azurerm_resource_group.aks.location
  resource_group_name = azurerm_resource_group.aks.name
  sku                 = "Standard"
  admin_enabled       = false
  tags                = local.tags
}

# ---------------------------------------------------------------- identities

resource "azurerm_user_assigned_identity" "control_plane" {
  name                = "id-${var.prefix}-control-plane"
  location            = azurerm_resource_group.aks.location
  resource_group_name = azurerm_resource_group.aks.name
  tags                = local.tags
}

# AKS needs to join the cluster's load balancers and nodes to the subnet.
resource "azurerm_role_assignment" "control_plane_network" {
  scope                = azurerm_virtual_network.aks.id
  role_definition_name = "Network Contributor"
  principal_id         = azurerm_user_assigned_identity.control_plane.principal_id
}

resource "azuread_group" "aks_admins" {
  display_name     = "${var.prefix}-cluster-admins"
  security_enabled = true
}

# ---------------------------------------------------------------- cluster

resource "azurerm_kubernetes_cluster" "aks" {
  name                = "aks-${var.prefix}"
  location            = azurerm_resource_group.aks.location
  resource_group_name = azurerm_resource_group.aks.name
  dns_prefix          = var.prefix
  kubernetes_version  = var.kubernetes_version
  sku_tier            = "Standard"
  tags                = local.tags

  oidc_issuer_enabled       = true
  workload_identity_enabled = true
  local_account_disabled    = false

  default_node_pool {
    name                         = "system"
    vm_size                      = "Standard_D4s_v5"
    vnet_subnet_id               = azurerm_subnet.nodes.id
    auto_scaling_enabled         = true
    min_count                    = 2
    max_count                    = 4
    zones                        = ["1", "2", "3"]
    only_critical_addons_enabled = true
    os_sku                       = "AzureLinux"

    upgrade_settings {
      max_surge = "33%"
    }
  }

  # Manual = you manage node pools; Auto = node auto-provisioning (Karpenter).
  node_provisioning_profile {
    mode = "Manual"
  }

  identity {
    type         = "UserAssigned"
    identity_ids = [azurerm_user_assigned_identity.control_plane.id]
  }

  network_profile {
    network_plugin      = "azure"
    network_plugin_mode = "overlay"
    network_policy      = "cilium"
    network_data_plane  = "cilium"
    pod_cidr            = "192.168.0.0/16"
    service_cidr        = "172.16.0.0/16"
    dns_service_ip      = "172.16.0.10"
    load_balancer_sku   = "standard"
  }

  azure_active_directory_role_based_access_control {
    tenant_id              = data.azurerm_client_config.current.tenant_id
    admin_group_object_ids = [azuread_group.aks_admins.object_id]
    azure_rbac_enabled     = true
  }

  oms_agent {
    log_analytics_workspace_id      = azurerm_log_analytics_workspace.aks.id
    msi_auth_for_monitoring_enabled = true
  }

  key_vault_secrets_provider {
    secret_rotation_enabled = true
  }

  depends_on = [azurerm_role_assignment.control_plane_network, azurerm_subnet_network_security_group_association.nodes]
}

resource "azurerm_kubernetes_cluster_node_pool" "apps" {
  name                  = "apps"
  kubernetes_cluster_id = azurerm_kubernetes_cluster.aks.id
  vm_size               = "Standard_D8s_v5"
  vnet_subnet_id        = azurerm_subnet.nodes.id
  node_count            = 3
  zones                 = ["1", "2", "3"]
  mode                  = "User"
  node_labels = {
    "workload" = "apps"
  }
  tags = local.tags
}

# Pull images from ACR with the kubelet identity, not admin credentials.
resource "azurerm_role_assignment" "kubelet_acr_pull" {
  scope                            = azurerm_container_registry.aks.id
  role_definition_name             = "AcrPull"
  principal_id                     = azurerm_kubernetes_cluster.aks.kubelet_identity[0].object_id
  skip_service_principal_aad_check = true
}

# ---------------------------------------------------------------- workload identity

resource "azurerm_user_assigned_identity" "orders_api" {
  name                = "id-${var.prefix}-orders-api"
  location            = azurerm_resource_group.aks.location
  resource_group_name = azurerm_resource_group.aks.name
  tags                = local.tags
}

# The orders-api service account in namespace "orders" signs in as this identity.
resource "azurerm_federated_identity_credential" "orders_api" {
  name                      = "orders-api"
  user_assigned_identity_id = azurerm_user_assigned_identity.orders_api.id
  issuer                    = azurerm_kubernetes_cluster.aks.oidc_issuer_url
  subject                   = "system:serviceaccount:orders:orders-api"
  audience                  = ["api://AzureADTokenExchange"]
}

resource "azurerm_role_assignment" "orders_api_reader" {
  scope                = azurerm_resource_group.aks.id
  role_definition_name = "Reader"
  principal_id         = azurerm_user_assigned_identity.orders_api.principal_id
}
