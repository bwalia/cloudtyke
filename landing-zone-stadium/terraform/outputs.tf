output "events" {
  description = "Each event: its kit, resource groups, VNet and AKS cluster."
  value = {
    for e, cfg in var.events : e => {
      title                = cfg.title
      kit                  = cfg.kit
      resource_group       = azurerm_resource_group.pitch[e].name
      seats_resource_group = azurerm_resource_group.seats[e].name
      vnet                 = azurerm_virtual_network.seats[e].name
      vnet_id              = azurerm_virtual_network.seats[e].id
      aks_cluster          = azurerm_kubernetes_cluster.pitch[e].name
      lockers              = azurerm_key_vault.seats[e].name
      media_storage        = azurerm_storage_account.pitch[e].name
      scorebook            = try(azurerm_postgresql_flexible_server.pitch_scorebook[e].name, null)
      crew_client_id       = azurerm_user_assigned_identity.seats_crew[e].client_id
    }
  }
}

output "gates" {
  description = "The shared front of the stadium (sub-connectivity)."
  value = {
    hub_vnet          = azurerm_virtual_network.gates.name
    firewall_ip       = azurerm_public_ip.gates_firewall.ip_address
    private_dns_zones = [for z in azurerm_private_dns_zone.gates : z.name]
    kit_room          = azurerm_container_registry.kit_room.login_server
  }
}

output "control_room" {
  description = "Central logs (sub-management)."
  value       = azurerm_log_analytics_workspace.control_room.name
}

output "safety_code" {
  description = "The policy assignment every event inherits."
  value = {
    assignment_id    = azurerm_management_group_policy_assignment.safety_code.id
    management_group = data.azurerm_management_group.landingzones.name
  }
}
