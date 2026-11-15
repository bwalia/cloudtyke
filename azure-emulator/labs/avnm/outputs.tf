output "network_manager_id" {
  value = azurerm_network_manager.contoso.id
}

output "spokes" {
  description = "Spoke => test host (think of it as a VM in snet-app)."
  value       = { for k, v in local.spokes : k => cidrhost(cidrsubnet(v.cidr, 8, 1), 4) }
}

output "managed_vnets" {
  value = length(local.spokes) + length(var.regions)
}
