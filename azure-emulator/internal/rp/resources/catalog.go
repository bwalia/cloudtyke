package resources

import "strings"

// Location is an emulated Azure region.
type Location struct {
	Name, DisplayName, Geography, Paired string
	Latitude, Longitude                  string
}

// Locations is the curated region catalogue.
var Locations = []Location{
	{"uksouth", "UK South", "United Kingdom", "ukwest", "50.941", "-0.799"},
	{"ukwest", "UK West", "United Kingdom", "uksouth", "53.427", "-3.084"},
	{"westeurope", "West Europe", "Europe", "northeurope", "52.3667", "4.9"},
	{"northeurope", "North Europe", "Europe", "westeurope", "53.3478", "-6.2597"},
	{"francecentral", "France Central", "France", "francesouth", "46.3772", "2.3730"},
	{"germanywestcentral", "Germany West Central", "Germany", "germanynorth", "50.110924", "8.682127"},
	{"swedencentral", "Sweden Central", "Sweden", "swedensouth", "60.67488", "17.14127"},
	{"eastus", "East US", "United States", "westus", "37.3719", "-79.8164"},
	{"eastus2", "East US 2", "United States", "centralus", "36.6681", "-78.3889"},
	{"centralus", "Central US", "United States", "eastus2", "41.5908", "-93.6208"},
	{"westus2", "West US 2", "United States", "westcentralus", "47.233", "-119.852"},
	{"westus3", "West US 3", "United States", "eastus", "33.448376", "-112.074036"},
	{"canadacentral", "Canada Central", "Canada", "canadaeast", "43.653", "-79.383"},
	{"australiaeast", "Australia East", "Australia", "australiasoutheast", "-33.86", "151.2094"},
	{"southeastasia", "Southeast Asia", "Asia Pacific", "eastasia", "1.283", "103.833"},
	{"japaneast", "Japan East", "Japan", "japanwest", "35.68", "139.77"},
	{"centralindia", "Central India", "India", "southindia", "18.5822", "73.9197"},
}

// IsLocation reports whether name is a known region (normalised).
func IsLocation(name string) bool {
	for _, l := range Locations {
		if strings.EqualFold(l.Name, name) {
			return true
		}
	}
	return false
}

// LocationNames returns the region names.
func LocationNames() []string {
	out := make([]string, len(Locations))
	for i, l := range Locations {
		out[i] = l.Name
	}
	return out
}

// KnownNamespaces are resource provider namespaces the emulator reports as
// registered even when it does not emulate them, so provider registration
// in Terraform succeeds. Requests to their resource types return 501.
var KnownNamespaces = []string{
	"Microsoft.AlertsManagement", "Microsoft.ApiManagement", "Microsoft.AppConfiguration", "Microsoft.AppPlatform",
	"Microsoft.Authorization", "Microsoft.Automation", "Microsoft.AVS", "Microsoft.Blueprint", "Microsoft.BotService",
	"Microsoft.Cache", "Microsoft.Cdn", "Microsoft.CognitiveServices", "Microsoft.Compute", "Microsoft.Consumption",
	"Microsoft.ContainerInstance", "Microsoft.ContainerRegistry", "Microsoft.ContainerService", "Microsoft.CostManagement",
	"Microsoft.CustomProviders", "Microsoft.Dashboard", "Microsoft.Databricks", "Microsoft.DataFactory",
	"Microsoft.DataLakeAnalytics", "Microsoft.DataLakeStore", "Microsoft.DataMigration", "Microsoft.DataProtection",
	"Microsoft.DBforMariaDB", "Microsoft.DBforMySQL", "Microsoft.DBforPostgreSQL", "Microsoft.DesktopVirtualization",
	"Microsoft.Devices", "Microsoft.DevTestLab", "Microsoft.DocumentDB", "Microsoft.EventGrid", "Microsoft.EventHub",
	"Microsoft.GuestConfiguration", "Microsoft.HDInsight", "Microsoft.HealthcareApis", "Microsoft.Insights",
	"Microsoft.IoTCentral", "Microsoft.KeyVault", "Microsoft.Kubernetes", "Microsoft.KubernetesConfiguration",
	"Microsoft.Kusto", "Microsoft.Logic", "Microsoft.MachineLearningServices", "Microsoft.Maintenance",
	"Microsoft.ManagedIdentity", "Microsoft.ManagedServices", "Microsoft.Management", "Microsoft.Maps",
	"Microsoft.MarketplaceOrdering", "Microsoft.MixedReality", "Microsoft.Monitor", "Microsoft.Network",
	"Microsoft.NotificationHubs", "Microsoft.OperationalInsights", "Microsoft.OperationsManagement",
	"Microsoft.PolicyInsights", "Microsoft.PowerBIDedicated", "Microsoft.RecoveryServices", "Microsoft.Relay",
	"Microsoft.Resources", "Microsoft.Search", "Microsoft.Security", "Microsoft.SecurityInsights",
	"Microsoft.ServiceBus", "Microsoft.ServiceFabric", "Microsoft.SignalRService", "Microsoft.Sql", "Microsoft.Storage",
	"Microsoft.StreamAnalytics", "Microsoft.Subscription", "Microsoft.Web",
}
