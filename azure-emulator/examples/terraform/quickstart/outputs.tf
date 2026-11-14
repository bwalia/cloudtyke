output "tenant_id" {
  value = data.azurerm_client_config.current.tenant_id
}

output "caller_object_id" {
  value = data.azurerm_client_config.current.object_id
}

output "app_resource_group_id" {
  value = azurerm_resource_group.app.id
}

output "readers_group_object_id" {
  value = azuread_group.app_readers.object_id
}
