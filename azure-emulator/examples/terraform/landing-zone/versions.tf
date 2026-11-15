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
#   connectivity - hub network and private DNS (seeded sub-connectivity)
#   management   - central logging (seeded sub-management)
#   default      - application landing zones (ARM_SUBSCRIPTION_ID = sub-sandbox)
# Credentials and metadata_host come from the ARM_* environment variables.
provider "azurerm" {
  features {}
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
