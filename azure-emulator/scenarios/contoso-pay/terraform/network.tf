# ================================================================ network: hub (connectivity) + payments spoke (CDE)

resource "azurerm_resource_group" "connectivity" {
  provider = azurerm.connectivity
  name     = "rg-cpay-connectivity-${local.rc}"
  location = var.location
  tags     = local.team_tags.platform
}

resource "azurerm_virtual_network" "hub" {
  provider            = azurerm.connectivity
  name                = "vnet-cpay-hub-${local.rc}"
  location            = var.location
  resource_group_name = azurerm_resource_group.connectivity.name
  address_space       = ["10.0.0.0/16"]
  tags                = local.team_tags.platform
}

resource "azurerm_subnet" "hub" {
  provider             = azurerm.connectivity
  for_each             = { AzureFirewallSubnet = "10.0.0.0/26", snet-dns-resolver = "10.0.1.0/28", snet-shared = "10.0.2.0/24" }
  name                 = each.key
  resource_group_name  = azurerm_resource_group.connectivity.name
  virtual_network_name = azurerm_virtual_network.hub.name
  address_prefixes     = [each.value]
}

resource "azurerm_resource_group" "network" {
  name     = "rg-cpay-network-${var.environment}-${local.rc}"
  location = var.location
  tags     = local.team_tags.platform
}

resource "azurerm_virtual_network" "spoke" {
  name                = "vnet-cpay-payments-${var.environment}-${local.rc}"
  location            = var.location
  resource_group_name = azurerm_resource_group.network.name
  address_space       = ["10.50.0.0/16"]
  tags                = local.team_tags.platform
}

resource "azurerm_subnet" "aks" {
  name                 = "snet-aks"
  resource_group_name  = azurerm_resource_group.network.name
  virtual_network_name = azurerm_virtual_network.spoke.name
  address_prefixes     = ["10.50.0.0/22"]

  service_endpoint {
    service = "Microsoft.KeyVault"
  }
  service_endpoint {
    service = "Microsoft.Storage"
  }
}

resource "azurerm_subnet" "postgres" {
  name                 = "snet-postgres"
  resource_group_name  = azurerm_resource_group.network.name
  virtual_network_name = azurerm_virtual_network.spoke.name
  address_prefixes     = ["10.50.8.0/24"]

  delegation {
    name = "postgres-flexible"
    service_delegation {
      name    = "Microsoft.DBforPostgreSQL/flexibleServers"
      actions = ["Microsoft.Network/virtualNetworks/subnets/join/action"]
    }
  }
}

resource "azurerm_subnet" "private_endpoints" {
  name                 = "snet-private-endpoints"
  resource_group_name  = azurerm_resource_group.network.name
  virtual_network_name = azurerm_virtual_network.spoke.name
  address_prefixes     = ["10.50.9.0/24"]
}

resource "azurerm_network_security_group" "aks" {
  name                = "nsg-cpay-aks-${var.environment}"
  location            = var.location
  resource_group_name = azurerm_resource_group.network.name
  tags                = local.team_tags.platform

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

resource "azurerm_subnet_network_security_group_association" "aks" {
  subnet_id                 = azurerm_subnet.aks.id
  network_security_group_id = azurerm_network_security_group.aks.id
}

# Egress from the CDE only through the hub firewall (PCI-DSS req. 1.3).
resource "azurerm_route_table" "aks" {
  name                          = "rt-cpay-aks-${var.environment}"
  location                      = var.location
  resource_group_name           = azurerm_resource_group.network.name
  bgp_route_propagation_enabled = false
  tags                          = local.team_tags.platform

  route {
    name                   = "default-via-hub-firewall"
    address_prefix         = "0.0.0.0/0"
    next_hop_type          = "VirtualAppliance"
    next_hop_in_ip_address = "10.0.0.4"
  }
}

resource "azurerm_subnet_route_table_association" "aks" {
  subnet_id      = azurerm_subnet.aks.id
  route_table_id = azurerm_route_table.aks.id
}

resource "azurerm_virtual_network_peering" "spoke_to_hub" {
  name                      = "peer-payments-to-hub"
  resource_group_name       = azurerm_resource_group.network.name
  virtual_network_name      = azurerm_virtual_network.spoke.name
  remote_virtual_network_id = azurerm_virtual_network.hub.id
  allow_forwarded_traffic   = true
}

resource "azurerm_virtual_network_peering" "hub_to_spoke" {
  provider                  = azurerm.connectivity
  name                      = "peer-hub-to-payments"
  resource_group_name       = azurerm_resource_group.connectivity.name
  virtual_network_name      = azurerm_virtual_network.hub.name
  remote_virtual_network_id = azurerm_virtual_network.spoke.id
  allow_forwarded_traffic   = true
}

# Private DNS, owned centrally in the hub and linked to the spoke.
locals {
  dns_zones = ["privatelink.vaultcore.azure.net", "privatelink.blob.core.windows.net", "privatelink.file.core.windows.net", "privatelink.mongo.cosmos.azure.com", "cpay.private.postgres.database.azure.com"]
}

resource "azurerm_private_dns_zone" "zones" {
  provider            = azurerm.connectivity
  for_each            = toset(local.dns_zones)
  name                = each.value
  resource_group_name = azurerm_resource_group.connectivity.name
  tags                = local.team_tags.platform
}

resource "azurerm_private_dns_zone_virtual_network_link" "spoke" {
  provider            = azurerm.connectivity
  for_each            = azurerm_private_dns_zone.zones
  name                = "link-payments-${var.environment}"
  private_dns_zone_id = each.value.id
  virtual_network_id  = azurerm_virtual_network.spoke.id
}
