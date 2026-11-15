# ================================================================ people, groups, CI identities

resource "random_password" "people" {
  for_each         = local.people
  length           = 20
  min_upper        = 2
  min_lower        = 2
  min_numeric      = 2
  min_special      = 2
  override_special = "!#%+=?@"
}

resource "azuread_user" "people" {
  for_each              = local.people
  user_principal_name   = "${each.key}@${local.domain}"
  display_name          = each.value.name
  given_name            = split(" ", each.value.name)[0]
  surname               = join(" ", slice(split(" ", each.value.name), 1, length(split(" ", each.value.name))))
  job_title             = each.value.title
  department            = local.teams[each.value.team].name
  company_name          = "Contoso Pay"
  usage_location        = "GB"
  password              = random_password.people[each.key].result
  force_password_change = true
}

resource "azuread_group" "teams" {
  for_each         = local.groups
  display_name     = "grp-cpay-${each.key}"
  description      = each.value
  security_enabled = true
  members          = [for k, p in local.people : azuread_user.people[k].object_id if contains(p.groups, each.key)]
}

# Each team's GitHub repository deploys with OpenID Connect: no client secrets.
resource "azuread_application" "ci" {
  for_each     = local.teams
  display_name = "app-cpay-ci-${each.key}"
  owners       = [data.azurerm_client_config.current.object_id]
}

resource "azuread_service_principal" "ci" {
  for_each  = local.teams
  client_id = azuread_application.ci[each.key].client_id
  owners    = [data.azurerm_client_config.current.object_id]
}

resource "azuread_application_federated_identity_credential" "ci_main" {
  for_each       = local.teams
  application_id = azuread_application.ci[each.key].id
  display_name   = "github-${each.value.repo}-main"
  description    = "Deployments from the main branch of ${var.github_org}/${each.value.repo}"
  audiences      = ["api://AzureADTokenExchange"]
  issuer         = "https://token.actions.githubusercontent.com"
  subject        = "repo:${var.github_org}/${each.value.repo}:environment:${var.environment}"
}
