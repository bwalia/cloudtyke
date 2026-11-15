# Azure Virtual Network Manager lab: hub-and-spoke and mesh at scale, across regions.
#
#   UK South                                   West Europe
#   vnet-hub-uks 10.0.0.0/16                   vnet-hub-weu 10.100.0.0/16
#     ├─ vnet-uks-spoke-1 10.1.0.0/16            ├─ vnet-weu-spoke-1 10.101.0.0/16
#     └─ vnet-uks-spoke-2 10.2.0.0/16            └─ vnet-weu-spoke-2 10.102.0.0/16
#
# There is not a single azurerm_virtual_network_peering in this file. The network
# manager owns connectivity: you describe the intent (groups + configurations),
# commit it to regions, and Azure creates and maintains the peerings.

data "azurerm_subscription" "current" {}

locals {
  tags = { lab = "avnm", managed-by = "terraform" }
  spokes = merge([
    for code, r in var.regions : {
      for i in range(1, var.spokes_per_region + 1) : "${code}-spoke-${i}" => {
        region   = code
        location = r.location
        cidr     = "10.${r.octet + i}.0.0/16"
      }
    }
  ]...)
}

resource "azurerm_resource_group" "lab" {
  name     = "rg-lab-avnm"
  location = "uksouth"
  tags     = local.tags
}

# ---------------------------------------------------------------- hubs, one per region

resource "azurerm_virtual_network" "hub" {
  for_each            = var.regions
  name                = "vnet-hub-${each.key}"
  location            = each.value.location
  resource_group_name = azurerm_resource_group.lab.name
  address_space       = ["10.${each.value.octet}.0.0/16"]
  tags                = local.tags
}

resource "azurerm_subnet" "hub_shared" {
  for_each             = var.regions
  name                 = "snet-shared"
  resource_group_name  = azurerm_resource_group.lab.name
  virtual_network_name = azurerm_virtual_network.hub[each.key].name
  address_prefixes     = ["10.${each.value.octet}.1.0/24"]
}

resource "azurerm_subnet" "bastion" {
  for_each             = var.regions
  name                 = "AzureBastionSubnet"
  resource_group_name  = azurerm_resource_group.lab.name
  virtual_network_name = azurerm_virtual_network.hub[each.key].name
  address_prefixes     = ["10.${each.value.octet}.2.0/26"]
}

# ---------------------------------------------------------------- spokes

resource "azurerm_virtual_network" "spoke" {
  for_each            = local.spokes
  name                = "vnet-${each.key}"
  location            = each.value.location
  resource_group_name = azurerm_resource_group.lab.name
  address_space       = [each.value.cidr]
  tags                = merge(local.tags, { region = each.value.region })
}

resource "azurerm_subnet" "app" {
  for_each             = local.spokes
  name                 = "snet-app"
  resource_group_name  = azurerm_resource_group.lab.name
  virtual_network_name = azurerm_virtual_network.spoke[each.key].name
  address_prefixes     = [cidrsubnet(each.value.cidr, 8, 1)]
}

# The app team on spoke 1 in UK South blocks SSH on its own NSG. Task 5 shows that
# a security admin AlwaysAllow rule still lets the platform's bastion in.
resource "azurerm_network_security_group" "uks_spoke_1" {
  name                = "nsg-uks-spoke-1-app"
  location            = "uksouth"
  resource_group_name = azurerm_resource_group.lab.name
  tags                = local.tags

  security_rule {
    name                       = "team-denies-ssh"
    priority                   = 100
    direction                  = "Inbound"
    access                     = "Deny"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "22"
    source_address_prefix      = "*"
    destination_address_prefix = "*"
  }
}

resource "azurerm_subnet_network_security_group_association" "uks_spoke_1" {
  subnet_id                 = azurerm_subnet.app["uks-spoke-1"].id
  network_security_group_id = azurerm_network_security_group.uks_spoke_1.id
}

# ---------------------------------------------------------------- the network manager

resource "azurerm_network_manager" "contoso" {
  name                = "avnm-contoso"
  location            = azurerm_resource_group.lab.location
  resource_group_name = azurerm_resource_group.lab.name
  description         = "Contoso global network: connectivity and baseline security for every spoke."
  scope_accesses      = ["Connectivity", "SecurityAdmin"]
  tags                = local.tags

  scope {
    subscription_ids = [data.azurerm_subscription.current.id]
  }
}

# Network groups: which VNets a configuration applies to. Membership is static
# here; in production you would usually use Azure Policy for dynamic membership
# (e.g. "every VNet tagged env=prod").
resource "azurerm_network_manager_network_group" "region" {
  for_each           = var.regions
  name               = "ng-spokes-${each.key}"
  network_manager_id = azurerm_network_manager.contoso.id
  description        = "All spokes in ${each.value.location}."
}

resource "azurerm_network_manager_network_group" "all_spokes" {
  name               = "ng-spokes-global"
  network_manager_id = azurerm_network_manager.contoso.id
  description        = "Every spoke in every region."
}

resource "azurerm_network_manager_static_member" "region" {
  for_each                  = local.spokes
  name                      = each.key
  network_group_id          = azurerm_network_manager_network_group.region[each.value.region].id
  target_virtual_network_id = azurerm_virtual_network.spoke[each.key].id
}

resource "azurerm_network_manager_static_member" "global" {
  for_each                  = local.spokes
  name                      = each.key
  network_group_id          = azurerm_network_manager_network_group.all_spokes.id
  target_virtual_network_id = azurerm_virtual_network.spoke[each.key].id
}

# ---------------------------------------------------------------- connectivity configurations

resource "azurerm_network_manager_connectivity_configuration" "hub_spoke" {
  for_each              = var.regions
  name                  = "cc-hub-spoke-${each.key}"
  network_manager_id    = azurerm_network_manager.contoso.id
  connectivity_topology = "HubAndSpoke"
  description           = "Hub-and-spoke in ${each.value.location}."

  hub {
    resource_id   = azurerm_virtual_network.hub[each.key].id
    resource_type = "Microsoft.Network/virtualNetworks"
  }

  applies_to_group {
    network_group_id = azurerm_network_manager_network_group.region[each.key].id
    # DirectlyConnected = spokes in this group also reach each other, without peerings
    group_connectivity = var.stage == "hub-spoke" ? "None" : "DirectlyConnected"
  }
}

resource "azurerm_network_manager_connectivity_configuration" "global_mesh" {
  name                  = "cc-global-mesh"
  network_manager_id    = azurerm_network_manager.contoso.id
  connectivity_topology = "Mesh"
  global_mesh_enabled   = true
  description           = "Every spoke reaches every spoke, across regions."

  applies_to_group {
    network_group_id   = azurerm_network_manager_network_group.all_spokes.id
    group_connectivity = "DirectlyConnected"
    global_mesh_enabled = true
  }
}

# ---------------------------------------------------------------- security admin rules

resource "azurerm_network_manager_security_admin_configuration" "baseline" {
  name               = "sac-baseline"
  network_manager_id = azurerm_network_manager.contoso.id
  description        = "Organisation-wide guardrails that application teams cannot override."
}

resource "azurerm_network_manager_admin_rule_collection" "management_ports" {
  name                            = "rc-management-ports"
  security_admin_configuration_id = azurerm_network_manager_security_admin_configuration.baseline.id
  network_group_ids               = [azurerm_network_manager_network_group.all_spokes.id]
}

# Platform bastions can always reach SSH/RDP, even if a team's NSG says no.
resource "azurerm_network_manager_admin_rule" "always_allow_bastion" {
  name                     = "always-allow-bastion"
  admin_rule_collection_id = azurerm_network_manager_admin_rule_collection.management_ports.id
  action                   = "AlwaysAllow"
  direction                = "Inbound"
  priority                 = 10
  protocol                 = "Tcp"
  destination_port_ranges  = ["22", "3389"]

  dynamic "source" {
    for_each = var.regions
    content {
      address_prefix      = "10.${source.value.octet}.2.0/26"
      address_prefix_type = "IPPrefix"
    }
  }
}

# Nobody else gets SSH/RDP into a spoke, and no NSG can allow it.
resource "azurerm_network_manager_admin_rule" "deny_management_ports" {
  name                     = "deny-ssh-rdp"
  admin_rule_collection_id = azurerm_network_manager_admin_rule_collection.management_ports.id
  action                   = "Deny"
  direction                = "Inbound"
  priority                 = 100
  protocol                 = "Tcp"
  destination_port_ranges  = ["22", "3389"]

  source {
    address_prefix      = "*"
    address_prefix_type = "IPPrefix"
  }
}

# ---------------------------------------------------------------- deployments (commits), per region

resource "azurerm_network_manager_deployment" "connectivity" {
  for_each           = var.regions
  network_manager_id = azurerm_network_manager.contoso.id
  location           = each.value.location
  scope_access       = "Connectivity"
  configuration_ids = concat(
    [azurerm_network_manager_connectivity_configuration.hub_spoke[each.key].id],
    var.stage == "global-mesh" ? [azurerm_network_manager_connectivity_configuration.global_mesh.id] : [],
  )
  # A deployment is a snapshot. Editing a configuration (e.g. group_connectivity)
  # changes nothing in Azure until it is committed again, so redeploy on change.
  triggers = {
    connectivity = var.stage
  }
  depends_on = [azurerm_network_manager_static_member.region, azurerm_network_manager_static_member.global]
}

resource "azurerm_network_manager_deployment" "security" {
  for_each           = var.security_admin ? var.regions : {}
  network_manager_id = azurerm_network_manager.contoso.id
  location           = each.value.location
  scope_access       = "SecurityAdmin"
  configuration_ids  = [azurerm_network_manager_security_admin_configuration.baseline.id]
  depends_on         = [azurerm_network_manager_admin_rule.always_allow_bastion, azurerm_network_manager_admin_rule.deny_management_ports]
}
