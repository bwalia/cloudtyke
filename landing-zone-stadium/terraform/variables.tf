variable "location" {
  description = "Home region. The safety code only allows UK regions."
  type        = string
  default     = "uksouth"
}

variable "events_subscription_id" {
  description = "The pitch: sub-events, under the ct-landingzones management group."
  type        = string
  default     = "5b5c0000-0000-4000-8000-000000000004"
}

variable "connectivity_subscription_id" {
  description = "The gates: sub-connectivity, under ct-platform."
  type        = string
  default     = "5b5c0000-0000-4000-8000-000000000001"
}

variable "management_subscription_id" {
  description = "The control room: sub-management, under ct-platform."
  type        = string
  default     = "5b5c0000-0000-4000-8000-000000000002"
}

variable "hub_address_space" {
  type    = string
  default = "10.0.0.0/16"
}

variable "firewall_private_ip" {
  description = "The bag check: next hop for all event traffic (first usable IP in AzureFirewallSubnet)."
  type        = string
  default     = "10.0.0.4"
}

# The events booked into the stadium. Each one gets the same seats (seats.tf)
# and its own pitch (pitch.tf), picked from the kit catalogue in pitch.tf.
#
# The key lesson: change one `kit` value (for example derby: football ->
# concert) and only that event's pitch changes. Gates, seats, the control room
# and the safety code stay exactly the same.
variable "events" {
  type = map(object({
    kit           = string # football, cricket or concert
    title         = string
    address_space = string
    cost_centre   = string
    github_repo   = string
  }))
  default = {
    derby = {
      kit           = "football"
      title         = "Saturday Derby"
      address_space = "10.31.0.0/16"
      cost_centre   = "CC-7100"
      github_repo   = "cloudtyke-arena/saturday-derby"
    }
    summer = {
      kit           = "cricket"
      title         = "Summer Test"
      address_space = "10.32.0.0/16"
      cost_centre   = "CC-7200"
      github_repo   = "cloudtyke-arena/summer-test"
    }
  }

  validation {
    # Storage account and Key Vault names have tight length limits.
    condition     = alltrue([for k in keys(var.events) : can(regex("^[a-z0-9]{1,10}$", k))])
    error_message = "Event keys must be 1-10 lowercase letters or digits (they go into storage account and Key Vault names)."
  }

  validation {
    condition     = alltrue([for e in values(var.events) : contains(["football", "cricket", "concert"], e.kit)])
    error_message = "Each event's kit must be one of: football, cricket, concert."
  }
}
