package rbac

// Built-in role definitions. GUIDs match Azure's well-known built-in role IDs
// so Terraform code using role_definition_id works unchanged. Permission
// lists follow Microsoft's public built-in roles documentation; they are a
// faithful subset, not a byte-for-byte copy.

// Permission is one permission block of a role definition.
type Permission struct {
	Actions        []string `json:"actions"`
	NotActions     []string `json:"notActions"`
	DataActions    []string `json:"dataActions"`
	NotDataActions []string `json:"notDataActions"`
}

// RoleDefinition is a role definition (built-in or custom).
type RoleDefinition struct {
	ID               string       `json:"id"` // guid
	RoleName         string       `json:"roleName"`
	Description      string       `json:"description"`
	Type             string       `json:"type"` // BuiltInRole | CustomRole
	Permissions      []Permission `json:"permissions"`
	AssignableScopes []string     `json:"assignableScopes"`
	CreatedOn        string       `json:"createdOn,omitempty"`
	UpdatedOn        string       `json:"updatedOn,omitempty"`
	CreatedBy        string       `json:"createdBy,omitempty"`
	UpdatedBy        string       `json:"updatedBy,omitempty"`
}

const builtinTimestamp = "2015-02-02T21:55:09.8806423Z"

var commonMgmt = []string{
	"Microsoft.Authorization/*/read",
	"Microsoft.Insights/alertRules/*",
	"Microsoft.Resources/deployments/*",
	"Microsoft.Resources/subscriptions/resourceGroups/read",
	"Microsoft.Support/*",
}

func a(xs ...string) []string { return xs }

func with(base []string, more ...string) []string {
	return append(append([]string{}, base...), more...)
}

func builtin(id, name, desc string, actions, notActions, dataActions []string) RoleDefinition {
	if notActions == nil {
		notActions = []string{}
	}
	if dataActions == nil {
		dataActions = []string{}
	}
	return RoleDefinition{
		ID: id, RoleName: name, Description: desc, Type: "BuiltInRole",
		Permissions:      []Permission{{Actions: actions, NotActions: notActions, DataActions: dataActions, NotDataActions: []string{}}},
		AssignableScopes: []string{"/"},
		CreatedOn:        builtinTimestamp, UpdatedOn: builtinTimestamp,
	}
}

// BuiltinRoles is the catalogue of emulated built-in roles.
var BuiltinRoles = []RoleDefinition{
	builtin("8e3af657-a8ff-443c-a75c-2fe8c4bcb635", "Owner", "Grants full access to manage all resources, including the ability to assign roles in Azure RBAC.", a("*"), nil, nil),
	builtin("b24988ac-6180-42a0-ab88-20f7382dd24c", "Contributor", "Grants full access to manage all resources, but does not allow you to assign roles in Azure RBAC, manage assignments in Azure Blueprints, or share image galleries.", a("*"),
		a("Microsoft.Authorization/*/Delete", "Microsoft.Authorization/*/Write", "Microsoft.Authorization/elevateAccess/Action", "Microsoft.Blueprint/blueprintAssignments/write", "Microsoft.Blueprint/blueprintAssignments/delete", "Microsoft.Compute/galleries/share/action", "Microsoft.Purview/consents/write", "Microsoft.Purview/consents/delete", "Microsoft.Resources/deploymentStacks/manageDenySetting/action", "Microsoft.Subscription/cancel/action", "Microsoft.Subscription/enable/action"), nil),
	builtin("acdd72a7-3385-48ef-bd42-f606fba81ae7", "Reader", "View all resources, but does not allow you to make any changes.", a("*/read"), nil, nil),
	builtin("18d7d88d-d35e-4fb5-a5c3-7773c20a72d9", "User Access Administrator", "Lets you manage user access to Azure resources.", a("*/read", "Microsoft.Authorization/*", "Microsoft.Support/*"), nil, nil),
	builtin("f58310d9-a9f6-439a-9e8d-f62e7b41a168", "Role Based Access Control Administrator", "Manage access to Azure resources by assigning roles using Azure RBAC. This role does not allow you to manage access using other ways, such as Azure Policy.", a("Microsoft.Authorization/roleAssignments/write", "Microsoft.Authorization/roleAssignments/delete", "*/read", "Microsoft.Support/*"), nil, nil),
	builtin("4d97b98b-1d4f-4787-a291-c67834d212e7", "Network Contributor", "Lets you manage networks, but not access to them.", with(commonMgmt, "Microsoft.Network/*", "Microsoft.ResourceHealth/availabilityStatuses/read"), nil, nil),
	builtin("b12aa53e-6015-4669-85d0-8515ebb3ae7f", "Private DNS Zone Contributor", "Lets you manage private DNS zone resources, but not the virtual networks they are linked to.", with(commonMgmt, "Microsoft.Network/privateDnsZones/*", "Microsoft.Network/privateDnsOperationResults/*", "Microsoft.Network/privateDnsOperationStatuses/*", "Microsoft.Network/virtualNetworks/read", "Microsoft.Network/virtualNetworks/join/action"), nil, nil),
	builtin("36243c78-bf99-498c-9df9-86d9f8d28608", "Resource Policy Contributor", "Users with rights to create/modify resource policy, create support ticket and read resources/hierarchy.", a("*/read", "Microsoft.Authorization/policyassignments/*", "Microsoft.Authorization/policydefinitions/*", "Microsoft.Authorization/policyexemptions/*", "Microsoft.Authorization/policysetdefinitions/*", "Microsoft.PolicyInsights/*", "Microsoft.Support/*"), nil, nil),
	builtin("00482a5a-887f-4fb3-b363-3b7fe8e74483", "Key Vault Administrator", "Perform all data plane operations on a key vault and all objects in it, including certificates, keys, and secrets.", with(commonMgmt, "Microsoft.KeyVault/checkNameAvailability/read", "Microsoft.KeyVault/deletedVaults/read", "Microsoft.KeyVault/locations/*/read", "Microsoft.KeyVault/vaults/*/read", "Microsoft.KeyVault/operations/read"), nil, a("Microsoft.KeyVault/vaults/*")),
	builtin("b86a8fe4-44ce-4948-aee5-eccb2c155cd7", "Key Vault Secrets Officer", "Perform any action on the secrets of a key vault, except manage permissions.", with(commonMgmt, "Microsoft.KeyVault/checkNameAvailability/read", "Microsoft.KeyVault/deletedVaults/read", "Microsoft.KeyVault/locations/*/read", "Microsoft.KeyVault/vaults/*/read", "Microsoft.KeyVault/operations/read"), nil, a("Microsoft.KeyVault/vaults/secrets/*")),
	builtin("4633458b-17de-408a-b874-0445c86b69e6", "Key Vault Secrets User", "Read secret contents.", a(), nil, a("Microsoft.KeyVault/vaults/secrets/getSecret/action", "Microsoft.KeyVault/vaults/secrets/readMetadata/action")),
	builtin("14b46e9e-c2b7-41b4-b07b-48a6ebf60603", "Key Vault Crypto Officer", "Perform any action on the keys of a key vault, except manage permissions.", with(commonMgmt, "Microsoft.KeyVault/checkNameAvailability/read", "Microsoft.KeyVault/deletedVaults/read", "Microsoft.KeyVault/locations/*/read", "Microsoft.KeyVault/vaults/*/read", "Microsoft.KeyVault/operations/read"), nil, a("Microsoft.KeyVault/vaults/keys/*", "Microsoft.KeyVault/vaults/keyrotationpolicies/*")),
	builtin("21090545-7ca7-4776-b22c-e363652d74d2", "Key Vault Reader", "Read metadata of key vaults and its certificates, keys, and secrets.", with(commonMgmt, "Microsoft.KeyVault/checkNameAvailability/read", "Microsoft.KeyVault/deletedVaults/read", "Microsoft.KeyVault/locations/*/read", "Microsoft.KeyVault/vaults/*/read", "Microsoft.KeyVault/operations/read"), nil, a("Microsoft.KeyVault/vaults/*/read", "Microsoft.KeyVault/vaults/secrets/readMetadata/action")),
	builtin("7f951dda-4ed3-4680-a7ca-43fe172d538d", "AcrPull", "acr pull", a("Microsoft.ContainerRegistry/registries/pull/read"), nil, nil),
	builtin("8311e382-0749-4cb8-b61a-304f252e45ec", "AcrPush", "acr push", a("Microsoft.ContainerRegistry/registries/pull/read", "Microsoft.ContainerRegistry/registries/push/write"), nil, nil),
	builtin("4abbcc35-e782-43d8-92c5-2d3f1bd2253f", "Azure Kubernetes Service Cluster User Role", "List cluster user credential action.", a("Microsoft.ContainerService/managedClusters/listClusterUserCredential/action", "Microsoft.ContainerService/managedClusters/read"), nil, nil),
	builtin("0ab0b1a8-8aac-4efd-b8c2-3ee1fb270be8", "Azure Kubernetes Service Cluster Admin Role", "List cluster admin credential action.", a("Microsoft.ContainerService/managedClusters/listClusterAdminCredential/action", "Microsoft.ContainerService/managedClusters/accessProfiles/listCredential/action", "Microsoft.ContainerService/managedClusters/read", "Microsoft.ContainerService/managedClusters/runcommand/action"), nil, nil),
	builtin("b1ff04bb-8a4e-4dc4-8eb5-8693973ce19b", "Azure Kubernetes Service RBAC Cluster Admin", "Lets you manage all resources in the cluster.", a("Microsoft.Authorization/*/read", "Microsoft.Insights/alertRules/*", "Microsoft.Resources/deployments/write", "Microsoft.Resources/subscriptions/operationresults/read", "Microsoft.Resources/subscriptions/read", "Microsoft.Resources/subscriptions/resourceGroups/read", "Microsoft.ContainerService/managedClusters/listClusterUserCredential/action"), nil, a("Microsoft.ContainerService/managedClusters/*")),
	builtin("3498e952-d568-435e-9b2c-8d77e338d7f7", "Azure Kubernetes Service RBAC Admin", "Lets you manage all resources under cluster/namespace, except update or delete resource quotas and namespaces.", a("Microsoft.Authorization/*/read", "Microsoft.Insights/alertRules/*", "Microsoft.Resources/deployments/write", "Microsoft.Resources/subscriptions/operationresults/read", "Microsoft.Resources/subscriptions/read", "Microsoft.Resources/subscriptions/resourceGroups/read", "Microsoft.ContainerService/managedClusters/listClusterUserCredential/action"), nil, a("Microsoft.ContainerService/managedClusters/*")),
	builtin("a7ffa36f-339b-4b5c-8bdf-e2c188b2c0eb", "Azure Kubernetes Service RBAC Writer", "Allows read/write access to most objects in a namespace.", a(), nil, a("Microsoft.ContainerService/managedClusters/apps/*", "Microsoft.ContainerService/managedClusters/batch/*", "Microsoft.ContainerService/managedClusters/configmaps/*", "Microsoft.ContainerService/managedClusters/pods/*", "Microsoft.ContainerService/managedClusters/secrets/*", "Microsoft.ContainerService/managedClusters/services/*", "Microsoft.ContainerService/managedClusters/serviceaccounts/*")),
	builtin("7f6c6a51-bcf8-42ba-9220-52d62157d7db", "Azure Kubernetes Service RBAC Reader", "Allows read-only access to see most objects in a namespace.", a(), nil, a("Microsoft.ContainerService/managedClusters/apps/*/read", "Microsoft.ContainerService/managedClusters/configmaps/read", "Microsoft.ContainerService/managedClusters/pods/read", "Microsoft.ContainerService/managedClusters/services/read")),
	builtin("f1a07417-d97a-45cb-824c-7a7467783830", "Managed Identity Operator", "Read and Assign User Assigned Identity", with(commonMgmt, "Microsoft.ManagedIdentity/userAssignedIdentities/*/read", "Microsoft.ManagedIdentity/userAssignedIdentities/*/assign/action"), nil, nil),
	builtin("e40ec5ca-96e0-45a2-b4ff-59039f2c2b59", "Managed Identity Contributor", "Create, Read, Update, and Delete User Assigned Identity", with(commonMgmt, "Microsoft.ManagedIdentity/userAssignedIdentities/read", "Microsoft.ManagedIdentity/userAssignedIdentities/write", "Microsoft.ManagedIdentity/userAssignedIdentities/delete", "Microsoft.ManagedIdentity/userAssignedIdentities/federatedIdentityCredentials/read", "Microsoft.ManagedIdentity/userAssignedIdentities/federatedIdentityCredentials/write", "Microsoft.ManagedIdentity/userAssignedIdentities/federatedIdentityCredentials/delete"), nil, nil),
	builtin("ba92f5b4-2d11-453d-a403-e96b0029c9fe", "Storage Blob Data Contributor", "Allows for read, write and delete access to Azure Storage blob containers and data", a("Microsoft.Storage/storageAccounts/blobServices/containers/delete", "Microsoft.Storage/storageAccounts/blobServices/containers/read", "Microsoft.Storage/storageAccounts/blobServices/containers/write", "Microsoft.Storage/storageAccounts/blobServices/generateUserDelegationKey/action"), nil, a("Microsoft.Storage/storageAccounts/blobServices/containers/blobs/delete", "Microsoft.Storage/storageAccounts/blobServices/containers/blobs/read", "Microsoft.Storage/storageAccounts/blobServices/containers/blobs/write", "Microsoft.Storage/storageAccounts/blobServices/containers/blobs/move/action", "Microsoft.Storage/storageAccounts/blobServices/containers/blobs/add/action")),
	builtin("2a2b9908-6ea1-4ae2-8e65-a410df84e7d1", "Storage Blob Data Reader", "Allows for read access to Azure Storage blob containers and data", a("Microsoft.Storage/storageAccounts/blobServices/containers/read", "Microsoft.Storage/storageAccounts/blobServices/generateUserDelegationKey/action"), nil, a("Microsoft.Storage/storageAccounts/blobServices/containers/blobs/read")),
	builtin("92aaf0da-9dab-42b6-94a3-d43ce8d16293", "Log Analytics Contributor", "Log Analytics Contributor can read all monitoring data and edit monitoring settings.", a("*/read", "Microsoft.ClassicCompute/virtualMachines/extensions/*", "Microsoft.Compute/virtualMachines/extensions/*", "Microsoft.Insights/alertRules/*", "Microsoft.Insights/diagnosticSettings/*", "Microsoft.OperationalInsights/*", "Microsoft.OperationsManagement/*", "Microsoft.Resources/deployments/*", "Microsoft.Resources/subscriptions/resourcegroups/deployments/*", "Microsoft.Storage/storageAccounts/listKeys/action", "Microsoft.Support/*"), nil, nil),
	builtin("43d0d8ad-25c7-4714-9337-8ba259a9fe05", "Monitoring Reader", "Can read all monitoring data (metrics, logs, etc.).", a("*/read", "Microsoft.OperationalInsights/workspaces/search/action", "Microsoft.Support/*"), nil, nil),
	builtin("749f88d5-cbae-40b8-bcfc-e573ddc772fa", "Monitoring Contributor", "Can read all monitoring data and update monitoring settings.", a("*/read", "Microsoft.AlertsManagement/*", "Microsoft.Insights/*", "Microsoft.OperationalInsights/workspaces/*", "Microsoft.Support/*"), nil, nil),
}

// BuiltinByID indexes built-in roles by lower-case GUID.
var BuiltinByID = func() map[string]RoleDefinition {
	m := map[string]RoleDefinition{}
	for _, r := range BuiltinRoles {
		m[r.ID] = r
	}
	return m
}()
