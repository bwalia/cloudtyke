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

# Both providers discover the emulator through metadata_host. Credentials and
# metadata_host come from the ARM_* environment variables printed by
# `tyke azure env` (already set inside `docker compose run --rm toolbox`).
provider "azurerm" {
  features {}
}

provider "azuread" {}
