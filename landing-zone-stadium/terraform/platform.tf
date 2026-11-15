# ================================================================ the stadium shell (never rebuilt)
#
#   sub-connectivity  the gates: hub VNet with the firewall subnet (bag check)
#                     and its public IP, the Bastion subnet (staff door),
#                     private DNS zones (the signs) and the kit room (shared ACR)
#   sub-management    the control room: Log Analytics keeps the tapes
#
# Both subscriptions sit under the ct-platform management group (from the seed).
# Nothing here knows or cares which event is on tonight.

data "azurerm_client_config" "current" {}

data "azuread_domains" "default" {
  only_default = true
}

locals {
  region_code = "uks"
  domain      = data.azuread_domains.default.domains[0].domain_name
  base_tags = {
    owner      = "stadium-platform"
    managed-by = "terraform"
    venue      = "cloudtyke-arena"
  }
}

# Storage, Key Vault, ACR and PostgreSQL names are global, so one short random
# suffix keeps them unique on a shared emulator. It never changes after apply.
resource "random_string" "suffix" {
  length  = 4
  upper   = false
  special = false
}

# ================================================================ control room (sub-management)

resource "azurerm_resource_group" "control_room" {
  provider = azurerm.management
  name     = "rg-ct-control-room-${local.region_code}"
  location = var.location
  tags     = merge(local.base_tags, { part = "control-room" })
}

resource "azurerm_log_analytics_workspace" "control_room" {
  provider            = azurerm.management
  name                = "log-ct-arena-${local.region_code}"
  location            = azurerm_resource_group.control_room.location
  resource_group_name = azurerm_resource_group.control_room.name
  sku                 = "PerGB2018"
  retention_in_days   = 90 # the tapes are kept after the event leaves
  daily_quota_gb      = 10
  tags                = azurerm_resource_group.control_room.tags
}

# ================================================================ the gates (sub-connectivity)

resource "azurerm_resource_group" "gates" {
  provider = azurerm.connectivity
  name     = "rg-ct-gates-${local.region_code}"
  location = var.location
  tags     = merge(local.base_tags, { part = "gates" })
}

resource "azurerm_virtual_network" "gates" {
  provider            = azurerm.connectivity
  name                = "vnet-ct-hub-${local.region_code}"
  location            = azurerm_resource_group.gates.location
  resource_group_name = azurerm_resource_group.gates.name
  address_space       = [var.hub_address_space]
  tags                = azurerm_resource_group.gates.tags
}

locals {
  gate_subnets = {
    AzureFirewallSubnet  = cidrsubnet(var.hub_address_space, 10, 0) # 10.0.0.0/26  the bag check
    AzureBastionSubnet   = cidrsubnet(var.hub_address_space, 10, 8) # 10.0.2.0/26  the staff door
    snet-shared-services = cidrsubnet(var.hub_address_space, 8, 3)  # 10.0.3.0/24  shared services
  }
}

resource "azurerm_subnet" "gates" {
  provider             = azurerm.connectivity
  for_each             = local.gate_subnets
  name                 = each.key
  resource_group_name  = azurerm_resource_group.gates.name
  virtual_network_name = azurerm_virtual_network.gates.name
  address_prefixes     = [each.value]
}

# The one front door everyone sees. It lives in the platform, never in an
# event: the safety code blocks public IPs in sub-events.
# (The emulator has no Azure Firewall resource yet, so the firewall itself is
# represented by its subnet, this public IP and var.firewall_private_ip.)
resource "azurerm_public_ip" "gates_firewall" {
  provider            = azurerm.connectivity
  name                = "pip-ct-fw-${local.region_code}"
  location            = azurerm_resource_group.gates.location
  resource_group_name = azurerm_resource_group.gates.name
  allocation_method   = "Static"
  sku                 = "Standard"
  zones               = ["1", "2", "3"]
  tags                = azurerm_resource_group.gates.tags
}

# The signs: private DNS zones owned centrally, linked to every event's VNet.
locals {
  private_dns_zones = [
    "privatelink.vaultcore.azure.net",
    "privatelink.blob.core.windows.net",
    "privatelink.azurecr.io",
    "privatelink.${var.location}.azmk8s.io",
    "privatelink.postgres.database.azure.com",
  ]
}

resource "azurerm_private_dns_zone" "gates" {
  provider            = azurerm.connectivity
  for_each            = toset(local.private_dns_zones)
  name                = each.value
  resource_group_name = azurerm_resource_group.gates.name
  tags                = azurerm_resource_group.gates.tags
}

resource "azurerm_private_dns_zone_virtual_network_link" "gates" {
  provider            = azurerm.connectivity
  for_each            = azurerm_private_dns_zone.gates
  name                = "link-hub"
  private_dns_zone_id = each.value.id
  virtual_network_id  = azurerm_virtual_network.gates.id
  tags                = azurerm_resource_group.gates.tags
}

# The kit room: one shared registry for every event's container images.
resource "azurerm_container_registry" "kit_room" {
  provider            = azurerm.connectivity
  name                = "acrctarena${random_string.suffix.result}"
  location            = azurerm_resource_group.gates.location
  resource_group_name = azurerm_resource_group.gates.name
  sku                 = "Premium"
  admin_enabled       = false # wristbands (Entra ID), not a shared password
  tags                = azurerm_resource_group.gates.tags
}
