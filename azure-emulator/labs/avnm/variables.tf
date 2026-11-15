# Change one switch, apply, then grade with ./verify.sh <stage>.

variable "stage" {
  description = <<-EOT
    Connectivity the network manager deploys:
      hub-spoke      a hub-and-spoke per region (Task 2)
      regional-mesh  + spokes in the same region connected directly (Task 3)
      global-mesh    + every spoke connected to every spoke, across regions (Task 4)
  EOT
  type        = string
  default     = "hub-spoke"
  validation {
    condition     = contains(["hub-spoke", "regional-mesh", "global-mesh"], var.stage)
    error_message = "stage must be hub-spoke, regional-mesh or global-mesh."
  }
}

variable "security_admin" {
  description = "Deploy the organisation-wide security admin rules (Task 5)."
  type        = bool
  default     = false
}

variable "spokes_per_region" {
  description = "Spoke VNets per region. Try 10: the network manager still needs no peering code."
  type        = number
  default     = 2
  validation {
    condition     = var.spokes_per_region >= 2 && var.spokes_per_region <= 50
    error_message = "Use between 2 and 50 spokes per region."
  }
}

variable "regions" {
  description = "Region code => Azure region and the second octet base for its address space."
  type = map(object({
    location = string
    octet    = number
  }))
  default = {
    uks = { location = "uksouth", octet = 0 }
    weu = { location = "westeurope", octet = 100 }
  }
}
