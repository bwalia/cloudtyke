# ================================================================ seats and stands (the same for every event)
#
# Every event booked in var.events gets exactly this baseline, whatever its kit:
#   a resource group, a spoke VNet with the standard subnets, NSGs, a route
#   table that sends everything to the hub firewall (the bag check), peering
#   both ways, links to the hub DNS zones (the signs), Entra groups with RBAC
#   (stewards and wristbands), a Key Vault (the event's lockers) and a deploy
#   identity for the changeover crew (GitHub OIDC, no stored secrets).
#
# Nothing in this file reads `kit`. That is the point: swapping the kit can
# never touch the seats.

locals {
  # Tags for the seats. The safety code wants event-kit on every resource
  # group; the seats serve any kit, so their value is always "baseline".
  seats_tags = {
    for e, cfg in var.events : e => merge(local.base_tags, {
      event       = e
      event-kit   = "baseline"
      cost-centre = cfg.cost_centre
    })
  }

  # snet-app 10.x.1.0/24, snet-data 10.x.2.0/24 (delegated to PostgreSQL),
  # snet-aks 10.x.4.0/22. Every event gets all three, used or not, so the
  # seat plan never changes when the kit does.
  seats_subnets = merge([
    for e, cfg in var.events : {
      "${e}/snet-app"  = { event = e, name = "snet-app", cidr = cidrsubnet(cfg.address_space, 8, 1), postgres = false }
      "${e}/snet-data" = { event = e, name = "snet-data", cidr = cidrsubnet(cfg.address_space, 8, 2), postgres = true }
      "${e}/snet-aks"  = { event = e, name = "snet-aks", cidr = cidrsubnet(cfg.address_space, 6, 1), postgres = false }
    }
  ]...)

  seats_dns_links = merge([
    for e, cfg in var.events : {
      for zone in local.private_dns_zones : "${e}/${zone}" => { event = e, zone = zone }
    }
  ]...)

  seats_groups = merge([
    for e, cfg in var.events : {
      "${e}-owners"       = { event = e, role = "Owner", description = "${cfg.title}: owns the event's space (cost, access, lifecycle)." }
      "${e}-contributors" = { event = e, role = "Contributor", description = "${cfg.title}: builds and runs the event." }
      "${e}-readers"      = { event = e, role = "Reader", description = "${cfg.title}: read-only access." }
    }
  ]...)
}

resource "azurerm_resource_group" "seats" {
  for_each = var.events
  name     = "rg-ct-${each.key}-seats-${local.region_code}"
  location = var.location
  tags     = local.seats_tags[each.key]

  # Build after the safety code, so every seat is checked by it.
  depends_on = [azurerm_management_group_policy_assignment.safety_code]
}

resource "azurerm_virtual_network" "seats" {
  for_each            = var.events
  name                = "vnet-ct-${each.key}-${local.region_code}"
  location            = azurerm_resource_group.seats[each.key].location
  resource_group_name = azurerm_resource_group.seats[each.key].name
  address_space       = [each.value.address_space]
  tags                = local.seats_tags[each.key]
}

resource "azurerm_subnet" "seats" {
  for_each             = local.seats_subnets
  name                 = each.value.name
  resource_group_name  = azurerm_resource_group.seats[each.value.event].name
  virtual_network_name = azurerm_virtual_network.seats[each.value.event].name
  address_prefixes     = [each.value.cidr]

  # The score-book subnet is reserved for PostgreSQL private access.
  dynamic "delegation" {
    for_each = each.value.postgres ? [1] : []
    content {
      name = "postgres-flexible"
      service_delegation {
        name    = "Microsoft.DBforPostgreSQL/flexibleServers"
        actions = ["Microsoft.Network/virtualNetworks/subnets/join/action"]
      }
    }
  }
}

# ---------------------------------------------------------------- door rules: NSGs

resource "azurerm_network_security_group" "seats" {
  for_each            = var.events
  name                = "nsg-ct-${each.key}-${local.region_code}"
  location            = azurerm_resource_group.seats[each.key].location
  resource_group_name = azurerm_resource_group.seats[each.key].name
  tags                = local.seats_tags[each.key]

  # Fans only arrive through the hub (the gates), on HTTPS.
  security_rule {
    name                       = "allow-https-from-gates"
    priority                   = 100
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "443"
    source_address_prefix      = var.hub_address_space
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

resource "azurerm_subnet_network_security_group_association" "seats" {
  for_each                  = { for k, s in local.seats_subnets : k => s if !s.postgres }
  subnet_id                 = azurerm_subnet.seats[each.key].id
  network_security_group_id = azurerm_network_security_group.seats[each.value.event].id
}

# ---------------------------------------------------------------- every road leads to the bag check

resource "azurerm_route_table" "seats" {
  for_each                      = var.events
  name                          = "rt-ct-${each.key}-${local.region_code}"
  location                      = azurerm_resource_group.seats[each.key].location
  resource_group_name           = azurerm_resource_group.seats[each.key].name
  bgp_route_propagation_enabled = false
  tags                          = local.seats_tags[each.key]

  route {
    name                   = "default-via-hub-firewall"
    address_prefix         = "0.0.0.0/0"
    next_hop_type          = "VirtualAppliance"
    next_hop_in_ip_address = var.firewall_private_ip
  }
}

resource "azurerm_subnet_route_table_association" "seats" {
  for_each       = local.seats_subnets
  subnet_id      = azurerm_subnet.seats[each.key].id
  route_table_id = azurerm_route_table.seats[each.value.event].id
}

# ---------------------------------------------------------------- walkways to the hub (both directions)

resource "azurerm_virtual_network_peering" "seats_to_gates" {
  for_each                  = var.events
  name                      = "peer-${each.key}-to-hub"
  resource_group_name       = azurerm_resource_group.seats[each.key].name
  virtual_network_name      = azurerm_virtual_network.seats[each.key].name
  remote_virtual_network_id = azurerm_virtual_network.gates.id
  allow_forwarded_traffic   = true
}

resource "azurerm_virtual_network_peering" "gates_to_seats" {
  provider                  = azurerm.connectivity
  for_each                  = var.events
  name                      = "peer-hub-to-${each.key}"
  resource_group_name       = azurerm_resource_group.gates.name
  virtual_network_name      = azurerm_virtual_network.gates.name
  remote_virtual_network_id = azurerm_virtual_network.seats[each.key].id
  allow_forwarded_traffic   = true
  allow_gateway_transit     = true
}

resource "azurerm_private_dns_zone_virtual_network_link" "seats" {
  provider            = azurerm.connectivity
  for_each            = local.seats_dns_links
  name                = "link-${each.value.event}"
  private_dns_zone_id = azurerm_private_dns_zone.gates[each.value.zone].id
  virtual_network_id  = azurerm_virtual_network.seats[each.value.event].id
}

# ---------------------------------------------------------------- stewards and wristbands

resource "azuread_group" "seats" {
  for_each         = local.seats_groups
  display_name     = "arena-${each.key}"
  description      = each.value.description
  security_enabled = true
}

resource "azurerm_role_assignment" "seats" {
  for_each             = local.seats_groups
  scope                = azurerm_resource_group.seats[each.value.event].id
  role_definition_name = each.value.role
  principal_id         = azuread_group.seats[each.key].object_id
}

# ---------------------------------------------------------------- the event's lockers (Key Vault)

resource "azurerm_key_vault" "seats" {
  for_each                   = var.events
  name                       = "kv-ct-${each.key}-${random_string.suffix.result}" # at most 20 characters
  location                   = azurerm_resource_group.seats[each.key].location
  resource_group_name        = azurerm_resource_group.seats[each.key].name
  tenant_id                  = data.azurerm_client_config.current.tenant_id
  sku_name                   = "standard"
  rbac_authorization_enabled = true
  purge_protection_enabled   = true
  soft_delete_retention_days = 90
  tags                       = local.seats_tags[each.key]
}

resource "azurerm_role_assignment" "seats_lockers" {
  for_each             = var.events
  scope                = azurerm_key_vault.seats[each.key].id
  role_definition_name = "Key Vault Secrets Officer"
  principal_id         = azuread_group.seats["${each.key}-owners"].object_id
}

# ---------------------------------------------------------------- the changeover crew (CI/CD with GitHub OIDC)

resource "azurerm_user_assigned_identity" "seats_crew" {
  for_each            = var.events
  name                = "id-ct-${each.key}-crew"
  location            = azurerm_resource_group.seats[each.key].location
  resource_group_name = azurerm_resource_group.seats[each.key].name
  tags                = local.seats_tags[each.key]
}

# A wristband, not a key on a string: the pipeline proves who it is with a
# GitHub token. No password is stored anywhere.
resource "azurerm_federated_identity_credential" "seats_crew" {
  for_each                  = var.events
  name                      = "github-${each.key}-main"
  user_assigned_identity_id = azurerm_user_assigned_identity.seats_crew[each.key].id
  issuer                    = "https://token.actions.githubusercontent.com"
  subject                   = "repo:${each.value.github_repo}:ref:refs/heads/main"
  audience                  = ["api://AzureADTokenExchange"]
}

resource "azurerm_role_assignment" "seats_crew_kit_room" {
  for_each             = var.events
  scope                = azurerm_container_registry.kit_room.id
  role_definition_name = "AcrPush"
  principal_id         = azurerm_user_assigned_identity.seats_crew[each.key].principal_id
}
