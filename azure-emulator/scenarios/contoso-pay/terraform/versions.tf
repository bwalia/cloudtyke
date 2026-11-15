terraform {
  required_version = ">= 1.6.0"
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 5.14"
    }
    azuread = {
      source  = "hashicorp/azuread"
      version = "~> 3.10"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.7"
    }
  }
}

# Contoso Pay runs three subscriptions, like most regulated platforms:
#   payments-prod  (the cardholder data environment, CDE)  -> default provider (sub-sandbox)
#   connectivity   (hub network)                           -> sub-connectivity
#   management     (logging, shared services)              -> sub-management
provider "azurerm" {
  features {
    key_vault {
      # Purge protection is mandatory by policy, so soft-deleted vaults and
      # secrets cannot (and must not) be purged on destroy.
      purge_soft_delete_on_destroy          = false
      purge_soft_deleted_secrets_on_destroy = false
      recover_soft_deleted_key_vaults       = false
      recover_soft_deleted_secrets          = false
    }
    resource_group {
      prevent_deletion_if_contains_resources = false
    }
  }
}

provider "azurerm" {
  alias           = "connectivity"
  subscription_id = var.connectivity_subscription_id
  features {}
}

provider "azurerm" {
  alias           = "management"
  subscription_id = var.management_subscription_id
  features {}
}

provider "azuread" {}
