# The lab is driven by these three switches. Change one, apply, then run
# ./verify.sh <stage> to see what changed and why.

variable "connectivity" {
  description = <<-EOT
    How the two spokes reach each other:
      none            no peering at all: three isolated VNets (Task 1)
      hub-only        spokes are peered to the hub only (Task 2)
      direct-peering  spokes are also peered to each other (Task 3)
      hub-firewall    spoke traffic is routed through the hub firewall with UDRs (Task 4)
  EOT
  type        = string
  default     = "hub-only"
  validation {
    condition     = contains(["none", "hub-only", "direct-peering", "hub-firewall"], var.connectivity)
    error_message = "connectivity must be none, hub-only, direct-peering or hub-firewall."
  }
}

variable "allow_forwarded_traffic" {
  description = "Whether the spokes accept traffic forwarded by the hub firewall (Task 5 turns this off)."
  type        = bool
  default     = true
}

variable "block_https_between_spokes" {
  description = "Adds an NSG rule on spoke B's app subnet that denies HTTPS from spoke A (Task 6)."
  type        = bool
  default     = false
}

variable "location" {
  type    = string
  default = "uksouth"
}

variable "firewall_ip" {
  description = "Private IP of the hub firewall / NVA in AzureFirewallSubnet."
  type        = string
  default     = "10.0.0.4"
}
