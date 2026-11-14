output "deployer_client_id" {
  value = azuread_application.deployer.client_id
}

output "deployer_client_secret" {
  value     = azuread_application_password.deployer.value
  sensitive = true
}

output "platform_group_object_id" {
  value = azuread_group.platform.object_id
}

output "user_principal_names" {
  value = [for u in azuread_user.engineers : u.user_principal_name]
}
