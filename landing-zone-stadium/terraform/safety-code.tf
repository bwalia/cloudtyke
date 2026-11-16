# ================================================================ the stadium safety code (never rebuilt)
#
# One Azure Policy initiative, defined AND assigned at the ct-landingzones
# management group. Every subscription under it (today sub-events) inherits
# it, so every event follows the same rules without asking. Break a rule and
# Azure answers with RequestDisallowedByPolicy before anything is built.
#
# Management groups come from the seed: the emulator's management group API is
# read-only, so we look the group up and never create it here.

data "azurerm_management_group" "landingzones" {
  name = "ct-landingzones"
}

locals {
  safety_rules = {
    uk-only = {
      display = "Build only in UK regions"
      mode    = "Indexed"
      rule = {
        if   = { field = "location", notIn = "[parameters('allowedLocations')]" }
        then = { effect = "deny" }
      }
      parameters = { allowedLocations = { type = "Array", metadata = { displayName = "Allowed locations" } } }
    }
    # Indexed mode skips resource groups (as in real Azure), so they get their own rule.
    uk-only-rgs = {
      display = "Resource groups only in UK regions"
      mode    = "All"
      rule = {
        if = { allOf = [
          { field = "type", equals = "Microsoft.Resources/subscriptions/resourceGroups" },
          { field = "location", notIn = "[parameters('allowedLocations')]" },
        ] }
        then = { effect = "deny" }
      }
      parameters = { allowedLocations = { type = "Array", metadata = { displayName = "Allowed locations" } } }
    }
    rg-event-tags = {
      display = "Resource groups must carry event, event-kit and cost-centre tags"
      mode    = "All"
      rule = {
        if = { allOf = [
          { field = "type", equals = "Microsoft.Resources/subscriptions/resourceGroups" },
          { anyOf = [
            { field = "tags['event']", exists = "false" },
            { field = "tags['event-kit']", exists = "false" },
            { field = "tags['cost-centre']", exists = "false" },
          ] },
        ] }
        then = { effect = "deny" }
      }
      parameters = {}
    }
    no-public-ips = {
      display = "No public IP addresses in events (no side doors: use the gates)"
      mode    = "Indexed"
      rule = {
        if   = { field = "type", equals = "Microsoft.Network/publicIPAddresses" }
        then = { effect = "deny" }
      }
      parameters = {}
    }
    storage-secure = {
      display = "Storage: no public blobs, HTTPS only, TLS 1.2 or newer"
      mode    = "Indexed"
      rule = {
        if = { allOf = [
          { field = "type", equals = "Microsoft.Storage/storageAccounts" },
          { anyOf = [
            { field = "Microsoft.Storage/storageAccounts/allowBlobPublicAccess", equals = "true" },
            { field = "Microsoft.Storage/storageAccounts/supportsHttpsTrafficOnly", equals = "false" },
            { field = "Microsoft.Storage/storageAccounts/minimumTlsVersion", in = ["TLS1_0", "TLS1_1"] },
          ] },
        ] }
        then = { effect = "deny" }
      }
      parameters = {}
    }
    storage-no-shared-key = {
      display = "Storage: use Entra ID wristbands, not shared keys"
      mode    = "Indexed"
      rule = {
        if = { allOf = [
          { field = "type", equals = "Microsoft.Storage/storageAccounts" },
          { field = "Microsoft.Storage/storageAccounts/allowSharedKeyAccess", notEquals = "false" },
        ] }
        then = { effect = "deny" }
      }
      parameters = {}
    }
    keyvault-hardened = {
      display = "Key vaults (the lockers) must use RBAC and purge protection"
      mode    = "Indexed"
      rule = {
        if = { allOf = [
          { field = "type", equals = "Microsoft.KeyVault/vaults" },
          { anyOf = [
            { field = "Microsoft.KeyVault/vaults/enablePurgeProtection", notEquals = "true" },
            { field = "Microsoft.KeyVault/vaults/enableRbacAuthorization", notEquals = "true" },
          ] },
        ] }
        then = { effect = "deny" }
      }
      parameters = {}
    }
    aks-entra-rbac = {
      display = "AKS clusters must use Entra ID with Azure RBAC and have local accounts off"
      mode    = "Indexed"
      rule = {
        if = { allOf = [
          { field = "type", equals = "Microsoft.ContainerService/managedClusters" },
          { anyOf = [
            { field = "Microsoft.ContainerService/managedClusters/disableLocalAccounts", notEquals = "true" },
            { field = "Microsoft.ContainerService/managedClusters/aadProfile.enableAzureRBAC", notEquals = "true" },
          ] },
        ] }
        then = { effect = "deny" }
      }
      parameters = {}
    }
    postgres-private = {
      display = "PostgreSQL servers (the score books) must not allow public network access"
      mode    = "Indexed"
      rule = {
        if = { allOf = [
          { field = "type", equals = "Microsoft.DBforPostgreSQL/flexibleServers" },
          { field = "Microsoft.DBforPostgreSQL/flexibleServers/network.publicNetworkAccess", notEquals = "Disabled" },
        ] }
        then = { effect = "deny" }
      }
      parameters = {}
    }
  }
}

resource "azurerm_policy_definition" "safety_code" {
  for_each            = local.safety_rules
  name                = "arena-${each.key}"
  display_name        = "CloudTyke Arena: ${each.value.display}"
  policy_type         = "Custom"
  mode                = each.value.mode
  management_group_id = data.azurerm_management_group.landingzones.id
  policy_rule         = jsonencode(each.value.rule)
  parameters          = length(each.value.parameters) > 0 ? jsonencode(each.value.parameters) : null
  metadata            = jsonencode({ category = "CloudTyke Arena safety code", version = "1.0.0" })
}

resource "azurerm_management_group_policy_set_definition" "safety_code" {
  name                = "arena-safety-code"
  display_name        = "CloudTyke Arena safety code"
  description         = "The same rules for every event. Owned by the stadium platform team."
  policy_type         = "Custom"
  management_group_id = data.azurerm_management_group.landingzones.id
  metadata            = jsonencode({ category = "CloudTyke Arena safety code", version = "1.0.0" })
  parameters          = jsonencode({ allowedLocations = { type = "Array", defaultValue = ["uksouth", "ukwest"] } })

  dynamic "policy_definition_reference" {
    for_each = local.safety_rules
    content {
      policy_definition_id = azurerm_policy_definition.safety_code[policy_definition_reference.key].id
      reference_id         = policy_definition_reference.key
      parameter_values     = startswith(policy_definition_reference.key, "uk-only") ? jsonencode({ allowedLocations = { value = "[parameters('allowedLocations')]" } }) : null
    }
  }
}

# A management group assignment name must be 24 characters or less.
resource "azurerm_management_group_policy_assignment" "safety_code" {
  name                 = "arena-safety-code" # 17 characters
  display_name         = "CloudTyke Arena safety code (every event)"
  management_group_id  = data.azurerm_management_group.landingzones.id
  policy_definition_id = azurerm_management_group_policy_set_definition.safety_code.id
  description          = "Assigned once at ct-landingzones. Every event inherits it."

  non_compliance_message {
    content = "Blocked by the CloudTyke Arena safety code: the same rules apply to every event. Ask the stadium platform team if you need help."
  }
  non_compliance_message {
    content                        = "The stadium is in the UK: build in uksouth or ukwest."
    policy_definition_reference_id = "uk-only"
  }
  non_compliance_message {
    content                        = "The stadium is in the UK: put resource groups in uksouth or ukwest."
    policy_definition_reference_id = "uk-only-rgs"
  }
  non_compliance_message {
    content                        = "Tag every resource group with event, event-kit and cost-centre, so we know who booked it and who pays."
    policy_definition_reference_id = "rg-event-tags"
  }
  non_compliance_message {
    content                        = "No side doors: events get no public IPs. Fans come in through the gates (the hub firewall)."
    policy_definition_reference_id = "no-public-ips"
  }
}
