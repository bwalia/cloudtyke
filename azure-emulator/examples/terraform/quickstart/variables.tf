variable "prefix" {
  description = "Name prefix for everything this example creates."
  type        = string
  default     = "ct-quickstart"
}

variable "location" {
  description = "Azure region (any region in the emulator's catalogue)."
  type        = string
  default     = "uksouth"
}
