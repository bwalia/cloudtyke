# Entra ID: users, groups, membership, an app registration with a service
# principal, a client secret, a workload-identity federated credential and
# a Microsoft Graph application permission.
data "azuread_client_config" "current" {}

data "azuread_domains" "default" {
  only_initial = true
}

locals {
  domain = data.azuread_domains.default.domains[0].domain_name
}

resource "random_password" "user" {
  for_each         = toset(var.users)
  length           = 20
  special          = true
  override_special = "!@#%"
  min_upper        = 1
  min_lower        = 1
  min_numeric      = 1
}

resource "azuread_user" "engineers" {
  for_each            = toset(var.users)
  user_principal_name = "${each.key}@${local.domain}"
  display_name        = title(each.key)
  mail_nickname       = each.key
  password            = random_password.user[each.key].result
  department          = "Platform Engineering"
}

resource "azuread_group" "platform" {
  display_name     = "${var.prefix}-platform-engineers"
  description      = "Platform engineering team"
  security_enabled = true
  owners           = [data.azuread_client_config.current.object_id]
}

resource "azuread_group_member" "platform" {
  for_each         = azuread_user.engineers
  group_object_id  = azuread_group.platform.object_id
  member_object_id = each.value.object_id
}

resource "azuread_application" "deployer" {
  display_name = "${var.prefix}-deployer"
  owners       = [data.azuread_client_config.current.object_id]

  required_resource_access {
    resource_app_id = "00000003-0000-0000-c000-000000000000" # Microsoft Graph

    resource_access {
      id   = "df021288-bdef-4463-88db-98f22de89214" # User.Read.All (application)
      type = "Role"
    }
  }
}

resource "azuread_service_principal" "deployer" {
  client_id = azuread_application.deployer.client_id
  owners    = [data.azuread_client_config.current.object_id]
}

resource "azuread_application_password" "deployer" {
  application_id = azuread_application.deployer.id
  display_name   = "ci"
}

# GitHub Actions OIDC -> this app (workload identity federation).
resource "azuread_application_federated_identity_credential" "github_main" {
  application_id = azuread_application.deployer.id
  display_name   = "github-main"
  description    = "Deployments from the main branch"
  audiences      = ["api://AzureADTokenExchange"]
  issuer         = "https://token.actions.githubusercontent.com"
  subject        = "repo:${var.github_repo}:ref:refs/heads/main"
}

data "azuread_service_principal" "msgraph" {
  client_id = "00000003-0000-0000-c000-000000000000"
}

# Admin consent for the application permission.
resource "azuread_app_role_assignment" "deployer_user_read_all" {
  app_role_id         = "df021288-bdef-4463-88db-98f22de89214"
  principal_object_id = azuread_service_principal.deployer.object_id
  resource_object_id  = data.azuread_service_principal.msgraph.object_id
}
