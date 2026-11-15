# ================================================================ Azure Policy: the PCI-DSS guardrails
#
# Custom definitions bundled into one initiative, assigned to the payments
# subscription (the cardholder data environment). Engineers meet these as
# RequestDisallowedByPolicy errors the moment they try something non-compliant.

locals {
  policies = {
    allowed-locations = {
      display = "Resources must be deployed in the UK"
      mode    = "Indexed"
      rule = {
        if   = { field = "location", notIn = "[parameters('allowedLocations')]" }
        then = { effect = "deny" }
      }
      parameters = { allowedLocations = { type = "Array", metadata = { displayName = "Allowed locations" } } }
    }
    rg-required-tags = {
      display = "Resource groups must carry cost-centre, team and data-classification tags"
      mode    = "All"
      rule = {
        if = { allOf = [
          { field = "type", equals = "Microsoft.Resources/subscriptions/resourceGroups" },
          { anyOf = [
            { field = "tags['cost-centre']", exists = "false" },
            { field = "tags['team']", exists = "false" },
            { field = "tags['data-classification']", exists = "false" },
          ] },
        ] }
        then = { effect = "deny" }
      }
      parameters = {}
    }
    inherit-cost-centre = {
      display = "Inherit the cost-centre tag from the resource group"
      mode    = "Indexed"
      rule = {
        if = { allOf = [
          { field = "tags['cost-centre']", exists = "false" },
          { value = "[resourceGroup().tags['cost-centre']]", notEquals = "" },
        ] }
        then = { effect = "modify", details = {
          roleDefinitionIds = ["/providers/Microsoft.Authorization/roleDefinitions/b24988ac-6180-42a0-ab88-20f7382dd24c"]
          operations        = [{ operation = "add", field = "tags['cost-centre']", value = "[resourceGroup().tags['cost-centre']]" }]
        } }
      }
      parameters = {}
    }
    storage-secure = {
      display = "Storage accounts: no public blob access, HTTPS only, TLS 1.2+"
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
      display = "Storage accounts in the CDE must use Entra ID, not shared keys"
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
      display = "Key vaults must use RBAC and purge protection"
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
    postgres-private = {
      display = "PostgreSQL flexible servers must not allow public network access"
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
    cosmos-entra-only = {
      display = "Cosmos DB accounts should disable key-based (local) authentication"
      mode    = "Indexed"
      rule = {
        if = { allOf = [
          { field = "type", equals = "Microsoft.DocumentDB/databaseAccounts" },
          { field = "Microsoft.DocumentDB/databaseAccounts/disableLocalAuth", notEquals = "true" },
        ] }
        then = { effect = "audit" }
      }
      parameters = {}
    }
    aks-entra-rbac = {
      display = "AKS clusters must use Azure RBAC and have local accounts disabled"
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
    no-public-ips = {
      display = "No public IP addresses in the cardholder data environment"
      mode    = "Indexed"
      rule = {
        if   = { field = "type", equals = "Microsoft.Network/publicIPAddresses" }
        then = { effect = "deny" }
      }
      parameters = {}
    }
    protect-prod-data = {
      display = "Production data stores cannot be deleted"
      mode    = "Indexed"
      rule = {
        if = { allOf = [
          { field = "type", in = ["Microsoft.DBforPostgreSQL/flexibleServers", "Microsoft.DocumentDB/databaseAccounts"] },
          { field = "tags['protect']", equals = "true" },
        ] }
        then = { effect = "denyAction", details = { actionNames = ["delete"], cascadeBehaviors = { resourceGroup = "deny" } } }
      }
      parameters = {}
    }
  }
}

resource "azurerm_policy_definition" "cpay" {
  for_each     = local.policies
  name         = "cpay-${each.key}"
  display_name = "Contoso Pay: ${each.value.display}"
  policy_type  = "Custom"
  mode         = each.value.mode
  policy_rule  = jsonencode(each.value.rule)
  parameters   = length(each.value.parameters) > 0 ? jsonencode(each.value.parameters) : null
  metadata     = jsonencode({ category = "Contoso Pay PCI-DSS", version = "1.0.0" })
}

resource "azurerm_policy_set_definition" "pci" {
  name         = "cpay-pci-dss-baseline"
  display_name = "Contoso Pay PCI-DSS v4.0 baseline"
  description  = "Guardrails for the cardholder data environment. Owned by Security Operations."
  policy_type  = "Custom"
  metadata     = jsonencode({ category = "Contoso Pay PCI-DSS", version = "1.0.0" })
  parameters   = jsonencode({ allowedLocations = { type = "Array", defaultValue = ["uksouth", "ukwest"] } })

  dynamic "policy_definition_reference" {
    for_each = { for k, v in local.policies : k => v if k != "inherit-cost-centre" }
    content {
      policy_definition_id = azurerm_policy_definition.cpay[policy_definition_reference.key].id
      reference_id         = policy_definition_reference.key
      parameter_values     = policy_definition_reference.key == "allowed-locations" ? jsonencode({ allowedLocations = { value = "[parameters('allowedLocations')]" } }) : null
    }
  }
}

resource "azurerm_subscription_policy_assignment" "pci" {
  name                 = "cpay-pci-dss-baseline"
  display_name         = "Contoso Pay PCI-DSS v4.0 baseline (payments-prod)"
  subscription_id      = data.azurerm_subscription.prod.id
  policy_definition_id = azurerm_policy_set_definition.pci.id
  description          = "Assigned by Security Operations. Exemptions need CISO approval."

  non_compliance_message {
    content = "Blocked by the Contoso Pay PCI-DSS baseline. See https://wiki.contoso-pay.example/platform/policies or ask #platform-help."
  }
  non_compliance_message {
    content                        = "Only UK regions are approved for cardholder data (PCI-DSS req. 12.8 / data residency)."
    policy_definition_reference_id = "allowed-locations"
  }
  non_compliance_message {
    content                        = "Tag every resource group with cost-centre, team and data-classification (FinOps standard FIN-04)."
    policy_definition_reference_id = "rg-required-tags"
  }
}

resource "azurerm_subscription_policy_assignment" "inherit_cost_centre" {
  name                 = "cpay-inherit-cost-centre"
  display_name         = "Inherit cost-centre tags (payments-prod)"
  subscription_id      = data.azurerm_subscription.prod.id
  policy_definition_id = azurerm_policy_definition.cpay["inherit-cost-centre"].id
  location             = var.location

  identity {
    type = "SystemAssigned"
  }
}

resource "azurerm_role_assignment" "inherit_cost_centre" {
  scope                = data.azurerm_subscription.prod.id
  role_definition_name = "Contributor"
  principal_id         = azurerm_subscription_policy_assignment.inherit_cost_centre.identity[0].principal_id
}

# Security Operations and the policy admins manage guardrails.
resource "azurerm_role_assignment" "policy_admins" {
  scope                = data.azurerm_subscription.prod.id
  role_definition_name = "Resource Policy Contributor"
  principal_id         = azuread_group.teams["policy-admins"].object_id
}
