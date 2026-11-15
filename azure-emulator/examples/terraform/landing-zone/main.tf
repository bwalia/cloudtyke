# Landing zones: a hub-and-spoke platform in the Cloud Adoption Framework style.
#
#   sub-management    rg-*-management   Log Analytics for the whole estate
#   sub-connectivity  rg-*-hub          hub VNet (firewall, gateway, bastion and
#                                       shared-services subnets), firewall public
#                                       IP, private DNS zones, shared ACR
#   sub-sandbox       rg-*-lz-<name>    one spoke per application team: VNet
#                                       peered to the hub, NSGs, a route table that
#                                       forces egress through the hub firewall,
#                                       Entra groups + RBAC, a CI/CD identity with
#                                       GitHub OIDC, and (corp) an AKS cluster with
#                                       user-defined-routing egress
data "azurerm_client_config" "current" {}

data "azuread_domains" "default" {
  only_default = true
}

locals {
  region_code = "uks"
  domain      = data.azuread_domains.default.domains[0].domain_name
  base_tags = {
    owner       = "cloud-platform"
    managed-by  = "terraform"
    environment = "prod"
  }
}

# ================================================================ management

resource "azurerm_resource_group" "management" {
  provider = azurerm.management
  name     = "rg-${var.prefix}-management-${local.region_code}"
  location = var.location
  tags     = merge(local.base_tags, { purpose = "central-logging" })
}

resource "azurerm_log_analytics_workspace" "platform" {
  provider            = azurerm.management
  name                = "log-${var.prefix}-platform-${local.region_code}"
  location            = azurerm_resource_group.management.location
  resource_group_name = azurerm_resource_group.management.name
  sku                 = "PerGB2018"
  retention_in_days   = 90
  daily_quota_gb      = 10
  tags                = azurerm_resource_group.management.tags
}

# ================================================================ connectivity (hub)

resource "azurerm_resource_group" "hub" {
  provider = azurerm.connectivity
  name     = "rg-${var.prefix}-hub-${local.region_code}"
  location = var.location
  tags     = merge(local.base_tags, { purpose = "hub-network" })
}

resource "azurerm_virtual_network" "hub" {
  provider            = azurerm.connectivity
  name                = "vnet-${var.prefix}-hub-${local.region_code}"
  location            = azurerm_resource_group.hub.location
  resource_group_name = azurerm_resource_group.hub.name
  address_space       = [var.hub_address_space]
  tags                = azurerm_resource_group.hub.tags
}

locals {
  hub_subnets = {
    AzureFirewallSubnet  = cidrsubnet(var.hub_address_space, 10, 0)  # 10.0.0.0/26
    GatewaySubnet        = cidrsubnet(var.hub_address_space, 11, 8)  # 10.0.1.0/27
    AzureBastionSubnet   = cidrsubnet(var.hub_address_space, 10, 8)  # 10.0.2.0/26
    snet-shared-services = cidrsubnet(var.hub_address_space, 8, 3)   # 10.0.3.0/24
    snet-dns-inbound     = cidrsubnet(var.hub_address_space, 12, 64) # 10.0.4.0/28
  }
}

resource "azurerm_subnet" "hub" {
  provider             = azurerm.connectivity
  for_each             = local.hub_subnets
  name                 = each.key
  resource_group_name  = azurerm_resource_group.hub.name
  virtual_network_name = azurerm_virtual_network.hub.name
  address_prefixes     = [each.value]
}

resource "azurerm_public_ip" "firewall" {
  provider            = azurerm.connectivity
  name                = "pip-${var.prefix}-fw-${local.region_code}"
  location            = azurerm_resource_group.hub.location
  resource_group_name = azurerm_resource_group.hub.name
  allocation_method   = "Static"
  sku                 = "Standard"
  zones               = ["1", "2", "3"]
  tags                = azurerm_resource_group.hub.tags
}

resource "azurerm_public_ip" "bastion" {
  provider            = azurerm.connectivity
  name                = "pip-${var.prefix}-bastion-${local.region_code}"
  location            = azurerm_resource_group.hub.location
  resource_group_name = azurerm_resource_group.hub.name
  allocation_method   = "Static"
  sku                 = "Standard"
  tags                = azurerm_resource_group.hub.tags
}

resource "azurerm_network_security_group" "shared_services" {
  provider            = azurerm.connectivity
  name                = "nsg-${var.prefix}-shared-services-${local.region_code}"
  location            = azurerm_resource_group.hub.location
  resource_group_name = azurerm_resource_group.hub.name
  tags                = azurerm_resource_group.hub.tags

  security_rule {
    name                       = "allow-dns-from-spokes"
    priority                   = 100
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Udp"
    source_port_range          = "*"
    destination_port_range     = "53"
    source_address_prefix      = "10.0.0.0/8"
    destination_address_prefix = "*"
  }

  security_rule {
    name                       = "deny-internet-inbound"
    priority                   = 4000
    direction                  = "Inbound"
    access                     = "Deny"
    protocol                   = "*"
    source_port_range          = "*"
    destination_port_range     = "*"
    source_address_prefix      = "Internet"
    destination_address_prefix = "*"
  }
}

resource "azurerm_subnet_network_security_group_association" "shared_services" {
  provider                  = azurerm.connectivity
  subnet_id                 = azurerm_subnet.hub["snet-shared-services"].id
  network_security_group_id = azurerm_network_security_group.shared_services.id
}

# Private DNS zones for Private Link, owned centrally in the hub.
locals {
  private_dns_zones = [
    "privatelink.vaultcore.azure.net",
    "privatelink.blob.core.windows.net",
    "privatelink.azurecr.io",
    "privatelink.${var.location}.azmk8s.io",
  ]
}

resource "azurerm_private_dns_zone" "hub" {
  provider            = azurerm.connectivity
  for_each            = toset(local.private_dns_zones)
  name                = each.value
  resource_group_name = azurerm_resource_group.hub.name
  tags                = azurerm_resource_group.hub.tags
}

resource "azurerm_private_dns_zone_virtual_network_link" "hub" {
  provider            = azurerm.connectivity
  for_each            = azurerm_private_dns_zone.hub
  name                = "link-hub"
  private_dns_zone_id = each.value.id
  virtual_network_id  = azurerm_virtual_network.hub.id
  tags                = azurerm_resource_group.hub.tags
}

resource "azurerm_container_registry" "shared" {
  provider            = azurerm.connectivity
  name                = "acr${var.prefix}shared${local.region_code}"
  location            = azurerm_resource_group.hub.location
  resource_group_name = azurerm_resource_group.hub.name
  sku                 = "Premium"
  admin_enabled       = false
  tags                = azurerm_resource_group.hub.tags
}
