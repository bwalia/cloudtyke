variable "location" {
  type    = string
  default = "uksouth"
}

variable "environment" {
  type    = string
  default = "prod"
}

variable "connectivity_subscription_id" {
  type    = string
  default = "5b5c0000-0000-4000-8000-000000000001"
}

variable "management_subscription_id" {
  type    = string
  default = "5b5c0000-0000-4000-8000-000000000002"
}

variable "github_org" {
  description = "Where each team's repositories live (CI signs in with GitHub OIDC)."
  type        = string
  default     = "contoso-pay"
}

variable "protect_data" {
  description = "Tag production data stores protect=true. The 'Production data stores cannot be deleted' policy then blocks their deletion; set false (via change request) before decommissioning."
  type        = bool
  default     = true
}
