# Quickstart: resource groups, an Entra group and Azure RBAC on CloudTyke.
data "azurerm_client_config" "current" {}

data "azurerm_subscription" "current" {}

resource "azurerm_resource_group" "app" {
  name     = "rg-${var.prefix}-app"
  location = var.location
  tags = {
    environment = "dev"
    owner       = "platform"
    costCentre  = "cc-1234"
  }
}

resource "azurerm_resource_group" "shared" {
  name     = "rg-${var.prefix}-shared"
  location = var.location
  tags = {
    environment = "dev"
  }
}

resource "azuread_group" "app_readers" {
  display_name     = "${var.prefix}-app-readers"
  security_enabled = true
}

# Built-in role by name (looked up through the roleDefinitions API).
resource "azurerm_role_assignment" "readers" {
  scope                = azurerm_resource_group.app.id
  role_definition_name = "Reader"
  principal_id         = azuread_group.app_readers.object_id
}

# A least-privilege custom role, assignable only in this subscription.
resource "azurerm_role_definition" "tag_editor" {
  name        = "${var.prefix}-tag-editor"
  scope       = data.azurerm_subscription.current.id
  description = "Can read resources and edit tags only."

  permissions {
    actions     = ["*/read", "Microsoft.Resources/tags/*"]
    not_actions = []
  }

  assignable_scopes = [data.azurerm_subscription.current.id]
}

resource "azurerm_role_assignment" "tag_editor" {
  scope              = azurerm_resource_group.shared.id
  role_definition_id = azurerm_role_definition.tag_editor.role_definition_resource_id
  principal_id       = azuread_group.app_readers.object_id
}
