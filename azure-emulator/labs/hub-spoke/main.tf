# Hub-and-spoke lab
#
#                     ┌──────────── vnet-hub 10.0.0.0/16 ────────────┐
#                     │ AzureFirewallSubnet 10.0.0.0/26  (fw 10.0.0.4)│
#                     │ snet-shared         10.0.1.0/24  (DNS, tools) │
#                     └───────▲──────────────────────────────▲────────┘
#                     peering │                              │ peering
#   ┌── vnet-spoke-a 10.10.0.0/16 ──┐              ┌── vnet-spoke-b 10.20.0.0/16 ──┐
#   │ snet-app  10.10.1.0/24        │   ? ? ? ?    │ snet-app  10.20.1.0/24        │
#   │ snet-data 10.10.2.0/24        │ ◄──────────► │ snet-data 10.20.2.0/24        │
#   └───────────────────────────────┘              └───────────────────────────────┘
#
# Question the lab answers: can spoke A talk to spoke B, and what has to be
# true in Azure for that to work?

locals {
  tags = { lab = "hub-spoke", managed-by = "terraform" }
  spokes = {
    a = { cidr = "10.10.0.0/16", other = "b" }
    b = { cidr = "10.20.0.0/16", other = "a" }
  }
}

resource "azurerm_resource_group" "lab" {
  name     = "rg-lab-hub-spoke"
  location = var.location
  tags     = local.tags
}

# ---------------------------------------------------------------- hub

resource "azurerm_virtual_network" "hub" {
  name                = "vnet-hub"
  location            = azurerm_resource_group.lab.location
  resource_group_name = azurerm_resource_group.lab.name
  address_space       = ["10.0.0.0/16"]
  tags                = local.tags
}

resource "azurerm_subnet" "firewall" {
  name                 = "AzureFirewallSubnet"
  resource_group_name  = azurerm_resource_group.lab.name
  virtual_network_name = azurerm_virtual_network.hub.name
  address_prefixes     = ["10.0.0.0/26"]
}

resource "azurerm_subnet" "shared" {
  name                 = "snet-shared"
  resource_group_name  = azurerm_resource_group.lab.name
  virtual_network_name = azurerm_virtual_network.hub.name
  address_prefixes     = ["10.0.1.0/24"]
}

# ---------------------------------------------------------------- spokes

resource "azurerm_virtual_network" "spoke" {
  for_each            = local.spokes
  name                = "vnet-spoke-${each.key}"
  location            = azurerm_resource_group.lab.location
  resource_group_name = azurerm_resource_group.lab.name
  address_space       = [each.value.cidr]
  tags                = local.tags
}

resource "azurerm_subnet" "app" {
  for_each             = local.spokes
  name                 = "snet-app"
  resource_group_name  = azurerm_resource_group.lab.name
  virtual_network_name = azurerm_virtual_network.spoke[each.key].name
  address_prefixes     = [cidrsubnet(each.value.cidr, 8, 1)]
}

resource "azurerm_subnet" "data" {
  for_each             = local.spokes
  name                 = "snet-data"
  resource_group_name  = azurerm_resource_group.lab.name
  virtual_network_name = azurerm_virtual_network.spoke[each.key].name
  address_prefixes     = [cidrsubnet(each.value.cidr, 8, 2)]
}

# ---------------------------------------------------------------- hub <-> spoke peering
# A peering is two resources: one in each VNet. Until both exist the state is
# "Initiated" and no traffic flows.

resource "azurerm_virtual_network_peering" "spoke_to_hub" {
  for_each                  = var.connectivity == "none" ? {} : local.spokes
  name                      = "peer-spoke-${each.key}-to-hub"
  resource_group_name       = azurerm_resource_group.lab.name
  virtual_network_name      = azurerm_virtual_network.spoke[each.key].name
  remote_virtual_network_id = azurerm_virtual_network.hub.id
  # traffic forwarded by the hub firewall arrives over this peering
  allow_forwarded_traffic = var.allow_forwarded_traffic
}

resource "azurerm_virtual_network_peering" "hub_to_spoke" {
  for_each                  = var.connectivity == "none" ? {} : local.spokes
  name                      = "peer-hub-to-spoke-${each.key}"
  resource_group_name       = azurerm_resource_group.lab.name
  virtual_network_name      = azurerm_virtual_network.hub.name
  remote_virtual_network_id = azurerm_virtual_network.spoke[each.key].id
  allow_forwarded_traffic   = true
}

# ---------------------------------------------------------------- option 1: direct spoke <-> spoke peering

resource "azurerm_virtual_network_peering" "spoke_to_spoke" {
  for_each                  = var.connectivity == "direct-peering" ? local.spokes : {}
  name                      = "peer-spoke-${each.key}-to-spoke-${each.value.other}"
  resource_group_name       = azurerm_resource_group.lab.name
  virtual_network_name      = azurerm_virtual_network.spoke[each.key].name
  remote_virtual_network_id = azurerm_virtual_network.spoke[each.value.other].id
}

# ---------------------------------------------------------------- option 2: route through the hub firewall

resource "azurerm_route_table" "spoke" {
  for_each                      = var.connectivity == "hub-firewall" ? local.spokes : {}
  name                          = "rt-spoke-${each.key}"
  location                      = azurerm_resource_group.lab.location
  resource_group_name           = azurerm_resource_group.lab.name
  bgp_route_propagation_enabled = false
  tags                          = local.tags

  # traffic for the other spoke goes to the firewall, which forwards it
  route {
    name                   = "to-spoke-${each.value.other}-via-firewall"
    address_prefix         = local.spokes[each.value.other].cidr
    next_hop_type          = "VirtualAppliance"
    next_hop_in_ip_address = var.firewall_ip
  }
}

resource "azurerm_subnet_route_table_association" "app" {
  for_each       = azurerm_route_table.spoke
  subnet_id      = azurerm_subnet.app[each.key].id
  route_table_id = each.value.id
}

# ---------------------------------------------------------------- NSG on spoke B's app tier

resource "azurerm_network_security_group" "spoke_b_app" {
  name                = "nsg-spoke-b-app"
  location            = azurerm_resource_group.lab.location
  resource_group_name = azurerm_resource_group.lab.name
  tags                = local.tags

  dynamic "security_rule" {
    for_each = var.block_https_between_spokes ? [1] : []
    content {
      name                       = "deny-https-from-spoke-a"
      priority                   = 200
      direction                  = "Inbound"
      access                     = "Deny"
      protocol                   = "Tcp"
      source_port_range          = "*"
      destination_port_range     = "443"
      source_address_prefix      = local.spokes.a.cidr
      destination_address_prefix = "*"
    }
  }
}

resource "azurerm_subnet_network_security_group_association" "spoke_b_app" {
  subnet_id                 = azurerm_subnet.app["b"].id
  network_security_group_id = azurerm_network_security_group.spoke_b_app.id
}
