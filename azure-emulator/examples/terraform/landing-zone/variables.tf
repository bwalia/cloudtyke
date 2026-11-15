variable "prefix" {
  description = "Organisation prefix used in every name."
  type        = string
  default     = "contoso"
}

variable "location" {
  description = "Primary region."
  type        = string
  default     = "uksouth"
}

variable "connectivity_subscription_id" {
  description = "Subscription for the hub network (seeded sub-connectivity)."
  type        = string
  default     = "5b5c0000-0000-4000-8000-000000000001"
}

variable "management_subscription_id" {
  description = "Subscription for central logging (seeded sub-management)."
  type        = string
  default     = "5b5c0000-0000-4000-8000-000000000002"
}

variable "hub_address_space" {
  type    = string
  default = "10.0.0.0/16"
}

variable "firewall_private_ip" {
  description = "Next hop for spoke egress (the hub firewall / NVA private IP in AzureFirewallSubnet)."
  type        = string
  default     = "10.0.0.4"
}

# Each landing zone is one application team's spoke: its own resource group,
# VNet peered to the hub, egress forced through the firewall, Entra groups,
# RBAC and a deployment identity for its CI/CD pipeline.
variable "landing_zones" {
  type = map(object({
    archetype     = string # corp (private, hub-routed) or online (internet-facing)
    address_space = string
    team          = string
    cost_centre   = string
    github_repo   = string
  }))
  default = {
    payments = {
      archetype     = "corp"
      address_space = "10.10.0.0/16"
      team          = "Payments"
      cost_centre   = "CC-2100"
      github_repo   = "contoso/payments-platform"
    }
    storefront = {
      archetype     = "online"
      address_space = "10.20.0.0/16"
      team          = "Digital Storefront"
      cost_centre   = "CC-3400"
      github_repo   = "contoso/storefront"
    }
  }
}

# People in the directory. Group membership drives every role assignment.
variable "people" {
  type = map(object({
    display_name = string
    job_title    = string
    department   = string
    groups       = list(string)
  }))
  default = {
    "amara.okafor" = {
      display_name = "Amara Okafor"
      job_title    = "Lead Network Engineer"
      department   = "Cloud Platform"
      groups       = ["platform-network-admins"]
    }
    "li.wei" = {
      display_name = "Li Wei"
      job_title    = "Security Analyst"
      department   = "Information Security"
      groups       = ["platform-security-readers"]
    }
    "priya.sharma" = {
      display_name = "Priya Sharma"
      job_title    = "Engineering Manager"
      department   = "Payments"
      groups       = ["lz-payments-owners"]
    }
    "tomas.novak" = {
      display_name = "Tomas Novak"
      job_title    = "Software Engineer"
      department   = "Payments"
      groups       = ["lz-payments-contributors"]
    }
    "zara.haddad" = {
      display_name = "Zara Haddad"
      job_title    = "Senior Frontend Engineer"
      department   = "Digital Storefront"
      groups       = ["lz-storefront-contributors"]
    }
    "felix.moreno" = {
      display_name = "Felix Moreno"
      job_title    = "Product Analyst"
      department   = "Digital Storefront"
      groups       = ["lz-storefront-readers"]
    }
  }
}
