output "hub" {
  value = {
    vnet_id           = azurerm_virtual_network.hub.id
    firewall_ip       = azurerm_public_ip.firewall.ip_address
    private_dns_zones = [for z in azurerm_private_dns_zone.hub : z.name]
    shared_acr        = azurerm_container_registry.shared.login_server
  }
}

output "landing_zones" {
  value = {
    for lz, cfg in var.landing_zones : lz => {
      archetype        = cfg.archetype
      resource_group   = azurerm_resource_group.lz[lz].name
      vnet_id          = azurerm_virtual_network.lz[lz].id
      address_space    = cfg.address_space
      deploy_client_id = azurerm_user_assigned_identity.deploy[lz].client_id
      aks_cluster      = try(azurerm_kubernetes_cluster.lz[lz].name, null)
      aks_oidc_issuer  = try(azurerm_kubernetes_cluster.lz[lz].oidc_issuer_url, null)
    }
  }
}

output "people" {
  description = "Sign-in names and their groups."
  value       = { for k, p in var.people : azuread_user.people[k].user_principal_name => p.groups }
}

output "log_analytics_workspace_id" {
  value = azurerm_log_analytics_workspace.platform.workspace_id
}
