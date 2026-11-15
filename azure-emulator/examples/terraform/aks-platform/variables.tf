variable "prefix" {
  description = "Name prefix for everything this example creates."
  type        = string
  default     = "ct-aks"
}

variable "location" {
  description = "Azure region (any region in the emulator's catalogue)."
  type        = string
  default     = "uksouth"
}

variable "kubernetes_version" {
  description = "AKS Kubernetes version (minor like 1.34 or a patch like 1.34.2)."
  type        = string
  default     = "1.34"
}
