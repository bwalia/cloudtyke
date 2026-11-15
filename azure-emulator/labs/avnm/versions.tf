terraform {
  required_version = ">= 1.6.0"
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 5.14"
    }
  }
}

# Credentials and metadata_host come from the ARM_* environment variables
# (already set inside the toolbox: make shell).
provider "azurerm" {
  features {}
}
