# ================================================================ application landing zones (spokes)

locals {
  lz_tags = {
    for lz, cfg in var.landing_zones : lz => merge(local.base_tags, {
      landing-zone = lz
      archetype    = cfg.archetype
      team         = cfg.team
      cost-centre  = cfg.cost_centre
    })
  }

  # snet-app 10.x.1.0/24, snet-data 10.x.2.0/24, snet-aks 10.x.4.0/22 (corp only)
  lz_subnets = merge([
    for lz, cfg in var.landing_zones : merge(
      {
        "${lz}/snet-app"  = { lz = lz, name = "snet-app", cidr = cidrsubnet(cfg.address_space, 8, 1) }
        "${lz}/snet-data" = { lz = lz, name = "snet-data", cidr = cidrsubnet(cfg.address_space, 8, 2) }
      },
      cfg.archetype == "corp" ? {
        "${lz}/snet-aks" = { lz = lz, name = "snet-aks", cidr = cidrsubnet(cfg.address_space, 6, 1) }
      } : {}
    )
  ]...)

  lz_dns_links = merge([
    for lz, cfg in var.landing_zones : {
      for zone in local.private_dns_zones : "${lz}/${zone}" => { lz = lz, zone = zone }
    }
  ]...)
}

resource "azurerm_resource_group" "lz" {
  for_each = var.landing_zones
  name     = "rg-${var.prefix}-lz-${each.key}-${local.region_code}"
  location = var.location
  tags     = local.lz_tags[each.key]
}

resource "azurerm_virtual_network" "lz" {
  for_each            = var.landing_zones
  name                = "vnet-${var.prefix}-lz-${each.key}-${local.region_code}"
  location            = azurerm_resource_group.lz[each.key].location
  resource_group_name = azurerm_resource_group.lz[each.key].name
  address_space       = [each.value.address_space]
  dns_servers         = [cidrhost(local.hub_subnets["snet-dns-inbound"], 4)] # hub DNS resolver
  tags                = local.lz_tags[each.key]
}

resource "azurerm_subnet" "lz" {
  for_each             = local.lz_subnets
  name                 = each.value.name
  resource_group_name  = azurerm_resource_group.lz[each.value.lz].name
  virtual_network_name = azurerm_virtual_network.lz[each.value.lz].name
  address_prefixes     = [each.value.cidr]
}

# ---------------------------------------------------------------- traffic: NSGs and forced egress

resource "azurerm_network_security_group" "app" {
  for_each            = var.landing_zones
  name                = "nsg-${var.prefix}-lz-${each.key}-app"
  location            = azurerm_resource_group.lz[each.key].location
  resource_group_name = azurerm_resource_group.lz[each.key].name
  tags                = local.lz_tags[each.key]

  # online landing zones publish HTTPS to the internet; corp ones only to the hub
  security_rule {
    name                       = each.value.archetype == "online" ? "allow-https-from-internet" : "allow-https-from-hub"
    priority                   = 100
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "443"
    source_address_prefix      = each.value.archetype == "online" ? "Internet" : var.hub_address_space
    destination_address_prefix = "*"
  }

  security_rule {
    name                       = "deny-all-other-inbound"
    priority                   = 4096
    direction                  = "Inbound"
    access                     = "Deny"
    protocol                   = "*"
    source_port_range          = "*"
    destination_port_range     = "*"
    source_address_prefix      = "*"
    destination_address_prefix = "*"
  }
}

resource "azurerm_subnet_network_security_group_association" "app" {
  for_each                  = var.landing_zones
  subnet_id                 = azurerm_subnet.lz["${each.key}/snet-app"].id
  network_security_group_id = azurerm_network_security_group.app[each.key].id
}

resource "azurerm_route_table" "lz" {
  for_each                      = var.landing_zones
  name                          = "rt-${var.prefix}-lz-${each.key}-${local.region_code}"
  location                      = azurerm_resource_group.lz[each.key].location
  resource_group_name           = azurerm_resource_group.lz[each.key].name
  bgp_route_propagation_enabled = false
  tags                          = local.lz_tags[each.key]

  route {
    name                   = "default-via-hub-firewall"
    address_prefix         = "0.0.0.0/0"
    next_hop_type          = "VirtualAppliance"
    next_hop_in_ip_address = var.firewall_private_ip
  }
}

resource "azurerm_subnet_route_table_association" "lz" {
  for_each       = local.lz_subnets
  subnet_id      = azurerm_subnet.lz[each.key].id
  route_table_id = azurerm_route_table.lz[each.value.lz].id
}

# ---------------------------------------------------------------- hub <-> spoke peering (both directions)

resource "azurerm_virtual_network_peering" "spoke_to_hub" {
  for_each                  = var.landing_zones
  name                      = "peer-${each.key}-to-hub"
  resource_group_name       = azurerm_resource_group.lz[each.key].name
  virtual_network_name      = azurerm_virtual_network.lz[each.key].name
  remote_virtual_network_id = azurerm_virtual_network.hub.id
  allow_forwarded_traffic   = true
}

resource "azurerm_virtual_network_peering" "hub_to_spoke" {
  provider                  = azurerm.connectivity
  for_each                  = var.landing_zones
  name                      = "peer-hub-to-${each.key}"
  resource_group_name       = azurerm_resource_group.hub.name
  virtual_network_name      = azurerm_virtual_network.hub.name
  remote_virtual_network_id = azurerm_virtual_network.lz[each.key].id
  allow_forwarded_traffic   = true
  allow_gateway_transit     = true
}

resource "azurerm_private_dns_zone_virtual_network_link" "lz" {
  provider            = azurerm.connectivity
  for_each            = local.lz_dns_links
  name                = "link-lz-${each.value.lz}"
  private_dns_zone_id = azurerm_private_dns_zone.hub[each.value.zone].id
  virtual_network_id  = azurerm_virtual_network.lz[each.value.lz].id
}

# ---------------------------------------------------------------- landing zone access

resource "azurerm_role_assignment" "lz_owners" {
  for_each             = var.landing_zones
  scope                = azurerm_resource_group.lz[each.key].id
  role_definition_name = "Owner"
  principal_id         = azuread_group.this["lz-${each.key}-owners"].object_id
}

resource "azurerm_role_assignment" "lz_contributors" {
  for_each             = var.landing_zones
  scope                = azurerm_resource_group.lz[each.key].id
  role_definition_name = "Contributor"
  principal_id         = azuread_group.this["lz-${each.key}-contributors"].object_id
}

resource "azurerm_role_assignment" "lz_readers" {
  for_each             = var.landing_zones
  scope                = azurerm_resource_group.lz[each.key].id
  role_definition_name = "Reader"
  principal_id         = azuread_group.this["lz-${each.key}-readers"].object_id
}

resource "azurerm_role_assignment" "lz_security_readers" {
  for_each             = var.landing_zones
  scope                = azurerm_resource_group.lz[each.key].id
  role_definition_name = "Reader"
  principal_id         = azuread_group.this["platform-security-readers"].object_id
}

resource "azurerm_role_assignment" "lz_network_admins" {
  for_each             = var.landing_zones
  scope                = azurerm_virtual_network.lz[each.key].id
  role_definition_name = "Network Contributor"
  principal_id         = azuread_group.this["platform-network-admins"].object_id
}

# CI/CD: each team's GitHub repo deploys with workload identity federation (no secrets).
resource "azurerm_user_assigned_identity" "deploy" {
  for_each            = var.landing_zones
  name                = "id-${var.prefix}-lz-${each.key}-deploy"
  location            = azurerm_resource_group.lz[each.key].location
  resource_group_name = azurerm_resource_group.lz[each.key].name
  tags                = local.lz_tags[each.key]
}

resource "azurerm_federated_identity_credential" "github_prod" {
  for_each                  = var.landing_zones
  name                      = "github-${each.key}-prod"
  user_assigned_identity_id = azurerm_user_assigned_identity.deploy[each.key].id
  issuer                    = "https://token.actions.githubusercontent.com"
  subject                   = "repo:${each.value.github_repo}:environment:prod"
  audience                  = ["api://AzureADTokenExchange"]
}

resource "azurerm_role_assignment" "deploy_contributor" {
  for_each             = var.landing_zones
  scope                = azurerm_resource_group.lz[each.key].id
  role_definition_name = "Contributor"
  principal_id         = azurerm_user_assigned_identity.deploy[each.key].principal_id
}

resource "azurerm_role_assignment" "deploy_acr_push" {
  for_each             = var.landing_zones
  scope                = azurerm_container_registry.shared.id
  role_definition_name = "AcrPush"
  principal_id         = azurerm_user_assigned_identity.deploy[each.key].principal_id
}
