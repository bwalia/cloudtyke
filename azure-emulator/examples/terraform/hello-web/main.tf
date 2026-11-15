# hello-web: the smallest end-to-end CloudTyke demo.
#
# One web app on AKS that signs in with workload identity (no secrets), reads
# its greeting from Key Vault and keeps a visit counter in Blob storage, behind
# one Azure Policy guardrail. `make hello-web` deploys it and opens it on
# http://localhost:4740.

terraform {
  required_version = ">= 1.6.0"
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 5.14"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.7"
    }
  }
}

# Credentials and metadata_host come from the ARM_* variables (set in the toolbox).
provider "azurerm" {
  features {
    key_vault {
      # Purge protection is on (company policy), so a destroyed vault stays
      # soft-deleted for 7 days; the random suffix gives each deploy a new name.
      purge_soft_delete_on_destroy = false
    }
  }
}

variable "location" {
  type    = string
  default = "uksouth"
}

variable "greeting" {
  description = "The message the app reads from Key Vault. Change it and re-run make hello-web."
  type        = string
  default     = "Hello from CloudTyke! This greeting came from Azure Key Vault."
}

variable "allow_public_blob" {
  description = "Set true to watch the 'no public blob access' policy deny the storage account."
  type        = bool
  default     = false
}

data "azurerm_client_config" "current" {}

resource "random_string" "suffix" {
  length  = 5
  upper   = false
  special = false
}

locals {
  sfx  = random_string.suffix.result
  # Resource groups must carry these three tags wherever the company's
  # baseline policy is assigned (try removing one and run make hello-web).
  tags = { app = "hello-web", team = "Platform Interns", cost-centre = "CC-9000", data-classification = "internal" }
}

resource "azurerm_resource_group" "hello" {
  name     = "rg-hello-web"
  location = var.location
  tags     = local.tags
}

# ------------------------------------------------------------ guardrail
resource "azurerm_policy_definition" "no_public_blob" {
  name         = "hello-web-no-public-blob"
  display_name = "Storage accounts must not allow public blob access"
  policy_type  = "Custom"
  mode         = "Indexed"
  policy_rule = jsonencode({
    if = { allOf = [
      { field = "type", equals = "Microsoft.Storage/storageAccounts" },
      { field = "Microsoft.Storage/storageAccounts/allowBlobPublicAccess", equals = "true" },
    ] }
    then = { effect = "deny" }
  })
}

resource "azurerm_resource_group_policy_assignment" "no_public_blob" {
  name                 = "no-public-blob"
  resource_group_id    = azurerm_resource_group.hello.id
  policy_definition_id = azurerm_policy_definition.no_public_blob.id
  non_compliance_message {
    content = "hello-web: public blob access is not allowed. Set allow_public_blob = false."
  }
}

# ------------------------------------------------------------ Key Vault
resource "azurerm_key_vault" "hello" {
  name                       = "kv-hello-${local.sfx}"
  location                   = var.location
  resource_group_name        = azurerm_resource_group.hello.name
  tenant_id                  = data.azurerm_client_config.current.tenant_id
  sku_name                   = "standard"
  rbac_authorization_enabled = true
  purge_protection_enabled   = true
  soft_delete_retention_days = 7
  tags                       = local.tags
}

resource "azurerm_role_assignment" "me_kv" {
  scope                = azurerm_key_vault.hello.id
  role_definition_name = "Key Vault Secrets Officer"
  principal_id         = data.azurerm_client_config.current.object_id
}

resource "azurerm_key_vault_secret" "greeting" {
  name         = "greeting"
  value        = var.greeting
  key_vault_id = azurerm_key_vault.hello.id
  depends_on   = [azurerm_role_assignment.me_kv]
}

# ------------------------------------------------------------ Blob storage
resource "azurerm_storage_account" "hello" {
  name                            = "sthello${local.sfx}"
  location                        = var.location
  resource_group_name             = azurerm_resource_group.hello.name
  account_tier                    = "Standard"
  account_replication_type        = "LRS"
  allow_nested_items_to_be_public = var.allow_public_blob
  shared_access_key_enabled       = false
  default_to_oauth_authentication = true
  tags                            = local.tags
  depends_on                      = [azurerm_resource_group_policy_assignment.no_public_blob]
}

resource "azurerm_storage_container" "visits" {
  name               = "visits"
  storage_account_id = azurerm_storage_account.hello.id
}

# ------------------------------------------------------------ AKS + workload identity
resource "azurerm_kubernetes_cluster" "hello" {
  name                      = "aks-hello-web"
  location                  = var.location
  resource_group_name       = azurerm_resource_group.hello.name
  dns_prefix                = "hello"
  oidc_issuer_enabled       = true
  workload_identity_enabled = true
  local_account_disabled    = true # sign in with Entra ID only
  tags                      = local.tags

  # Kubernetes authorization through Azure RBAC role assignments.
  azure_active_directory_role_based_access_control {
    tenant_id          = data.azurerm_client_config.current.tenant_id
    azure_rbac_enabled = true
  }

  default_node_pool {
    name       = "system"
    vm_size    = "Standard_D2s_v5"
    node_count = 1
    upgrade_settings {
      max_surge = "10%"
    }
  }

  node_provisioning_profile {
    mode = "Manual"
  }

  identity {
    type = "SystemAssigned"
  }
}

# You (the deployer) administer the cluster through Azure RBAC.
resource "azurerm_role_assignment" "me_aks" {
  scope                = azurerm_kubernetes_cluster.hello.id
  role_definition_name = "Azure Kubernetes Service RBAC Cluster Admin"
  principal_id         = data.azurerm_client_config.current.object_id
}

# The app's identity: Kubernetes service account default/hello-web.
resource "azurerm_user_assigned_identity" "app" {
  name                = "id-hello-web"
  location            = var.location
  resource_group_name = azurerm_resource_group.hello.name
  tags                = local.tags
}

resource "azurerm_federated_identity_credential" "app" {
  name                      = "aks-default-hello-web"
  user_assigned_identity_id = azurerm_user_assigned_identity.app.id
  audience                  = ["api://AzureADTokenExchange"]
  issuer                    = azurerm_kubernetes_cluster.hello.oidc_issuer_url
  subject                   = "system:serviceaccount:default:hello-web"
}

resource "azurerm_role_assignment" "app_kv" {
  scope                = azurerm_key_vault.hello.id
  role_definition_name = "Key Vault Secrets User"
  principal_id         = azurerm_user_assigned_identity.app.principal_id
}

resource "azurerm_role_assignment" "app_blob" {
  scope                = azurerm_storage_account.hello.id
  role_definition_name = "Storage Blob Data Contributor"
  principal_id         = azurerm_user_assigned_identity.app.principal_id
}

# ------------------------------------------------------------ outputs
output "cluster_id" {
  value = azurerm_kubernetes_cluster.hello.id
}

output "kube_server" {
  description = "API server; sign in with an Entra token (local accounts are disabled)."
  value       = "https://${azurerm_kubernetes_cluster.hello.fqdn}:443"
}

output "kube_ca" {
  value     = azurerm_kubernetes_cluster.hello.kube_config[0].cluster_ca_certificate
  sensitive = true
}

output "app" {
  value = {
    client_id       = azurerm_user_assigned_identity.app.client_id
    tenant_id       = data.azurerm_client_config.current.tenant_id
    vault_uri       = azurerm_key_vault.hello.vault_uri
    storage_account = azurerm_storage_account.hello.name
  }
}
