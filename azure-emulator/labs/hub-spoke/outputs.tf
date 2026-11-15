output "test_hosts" {
  description = "Addresses the lab traces between (think of them as VMs)."
  value = {
    spoke_a_app = cidrhost(azurerm_subnet.app["a"].address_prefixes[0], 4)
    spoke_b_app = cidrhost(azurerm_subnet.app["b"].address_prefixes[0], 4)
    hub_shared  = cidrhost(azurerm_subnet.shared.address_prefixes[0], 4)
    firewall    = var.firewall_ip
  }
}

output "connectivity" {
  value = var.connectivity
}

output "peerings" {
  value = concat(
    [for p in azurerm_virtual_network_peering.spoke_to_hub : p.name],
    [for p in azurerm_virtual_network_peering.hub_to_spoke : p.name],
    [for p in azurerm_virtual_network_peering.spoke_to_spoke : p.name],
  )
}
