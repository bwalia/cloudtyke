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

# One provider per subscription, like a real platform team:
#   default       the pitch: every event lands in sub-events
#   connectivity  the gates: hub network, private DNS, kit room (sub-connectivity)
#   management    the control room: central logs (sub-management)
# Credentials and metadata_host come from the ARM_* environment variables.
provider "azurerm" {
  subscription_id = var.events_subscription_id

  features {
    key_vault {
      # Purge protection is required by the safety code, so soft-deleted
      # lockers cannot (and must not) be purged on destroy.
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
