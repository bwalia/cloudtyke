variable "prefix" {
  type    = string
  default = "ct-entra"
}

variable "users" {
  description = "Mail nicknames of the users to create."
  type        = list(string)
  default     = ["grace", "linus"]
}

variable "github_repo" {
  description = "owner/repo trusted by the federated credential."
  type        = string
  default     = "bwalia/cloudtyke"
}
