# ================================================================ people and groups
#
# Access is granted to groups, never to individuals. Platform groups manage
# shared infrastructure; each landing zone gets owners / contributors / readers.

locals {
  platform_groups = {
    "platform-network-admins"   = "Manage the hub network, peerings and DNS for the whole estate."
    "platform-security-readers" = "Read-only access to every subscription and the central logs."
  }
  lz_groups = merge([
    for lz, cfg in var.landing_zones : {
      "lz-${lz}-owners"       = "${cfg.team}: owns the ${lz} landing zone (cost, access, lifecycle)."
      "lz-${lz}-contributors" = "${cfg.team}: deploys and operates workloads in ${lz}."
      "lz-${lz}-readers"      = "${cfg.team}: read-only access to ${lz}."
    }
  ]...)
  groups = merge(local.platform_groups, local.lz_groups)
}

resource "random_password" "people" {
  for_each         = var.people
  length           = 20
  min_upper        = 2
  min_lower        = 2
  min_numeric      = 2
  min_special      = 2
  override_special = "!#%+=?@"
}

resource "azuread_user" "people" {
  for_each              = var.people
  user_principal_name   = "${each.key}@${local.domain}"
  display_name          = each.value.display_name
  given_name            = split(" ", each.value.display_name)[0]
  surname               = split(" ", each.value.display_name)[1]
  job_title             = each.value.job_title
  department            = each.value.department
  usage_location        = "GB"
  password              = random_password.people[each.key].result
  force_password_change = true
}

resource "azuread_group" "this" {
  for_each         = local.groups
  display_name     = "${var.prefix}-${each.key}"
  description      = each.value
  security_enabled = true
  members = [
    for k, p in var.people : azuread_user.people[k].object_id if contains(p.groups, each.key)
  ]
}

# ---------------------------------------------------------------- platform RBAC

resource "azurerm_role_assignment" "network_admins_hub" {
  scope                = azurerm_resource_group.hub.id
  role_definition_name = "Network Contributor"
  principal_id         = azuread_group.this["platform-network-admins"].object_id
}

resource "azurerm_role_assignment" "security_readers_management" {
  scope                = azurerm_resource_group.management.id
  role_definition_name = "Reader"
  principal_id         = azuread_group.this["platform-security-readers"].object_id
}

resource "azurerm_role_assignment" "security_readers_logs" {
  scope                = azurerm_log_analytics_workspace.platform.id
  role_definition_name = "Monitoring Reader"
  principal_id         = azuread_group.this["platform-security-readers"].object_id
}
