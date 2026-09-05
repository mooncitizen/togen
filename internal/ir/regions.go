package ir

type Region struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// The first region of each provider is the one togen init and the studio default to.
var Regions = map[CloudProvider][]Region{
	ProviderAWS: {
		{"eu-west-2", "Europe (London)"},
		{"eu-west-1", "Europe (Ireland)"},
		{"eu-west-3", "Europe (Paris)"},
		{"eu-central-1", "Europe (Frankfurt)"},
		{"eu-central-2", "Europe (Zurich)"},
		{"eu-north-1", "Europe (Stockholm)"},
		{"eu-south-1", "Europe (Milan)"},
		{"eu-south-2", "Europe (Spain)"},
		{"us-east-1", "US East (N. Virginia)"},
		{"us-east-2", "US East (Ohio)"},
		{"us-west-1", "US West (N. California)"},
		{"us-west-2", "US West (Oregon)"},
		{"ca-central-1", "Canada (Central)"},
		{"sa-east-1", "South America (São Paulo)"},
		{"af-south-1", "Africa (Cape Town)"},
		{"me-south-1", "Middle East (Bahrain)"},
		{"me-central-1", "Middle East (UAE)"},
		{"ap-south-1", "Asia Pacific (Mumbai)"},
		{"ap-southeast-1", "Asia Pacific (Singapore)"},
		{"ap-southeast-2", "Asia Pacific (Sydney)"},
		{"ap-northeast-1", "Asia Pacific (Tokyo)"},
		{"ap-northeast-2", "Asia Pacific (Seoul)"},
		{"ap-northeast-3", "Asia Pacific (Osaka)"},
	},
	ProviderGCP: {
		{"europe-west2", "London"},
		{"europe-west1", "Belgium"},
		{"europe-west3", "Frankfurt"},
		{"europe-west4", "Netherlands"},
		{"europe-west6", "Zurich"},
		{"europe-west9", "Paris"},
		{"europe-north1", "Finland"},
		{"europe-central2", "Warsaw"},
		{"europe-southwest1", "Madrid"},
		{"us-central1", "Iowa"},
		{"us-east1", "South Carolina"},
		{"us-east4", "Northern Virginia"},
		{"us-west1", "Oregon"},
		{"us-west2", "Los Angeles"},
		{"northamerica-northeast1", "Montréal"},
		{"northamerica-northeast2", "Toronto"},
		{"southamerica-east1", "São Paulo"},
		{"asia-south1", "Mumbai"},
		{"asia-southeast1", "Singapore"},
		{"asia-east1", "Taiwan"},
		{"asia-northeast1", "Tokyo"},
		{"asia-northeast3", "Seoul"},
		{"australia-southeast1", "Sydney"},
	},
	ProviderAzure: {
		{"uksouth", "UK South"},
		{"ukwest", "UK West"},
		{"northeurope", "North Europe"},
		{"westeurope", "West Europe"},
		{"francecentral", "France Central"},
		{"germanywestcentral", "Germany West Central"},
		{"switzerlandnorth", "Switzerland North"},
		{"swedencentral", "Sweden Central"},
		{"norwayeast", "Norway East"},
		{"polandcentral", "Poland Central"},
		{"eastus", "East US"},
		{"eastus2", "East US 2"},
		{"centralus", "Central US"},
		{"southcentralus", "South Central US"},
		{"westus2", "West US 2"},
		{"westus3", "West US 3"},
		{"canadacentral", "Canada Central"},
		{"brazilsouth", "Brazil South"},
		{"southafricanorth", "South Africa North"},
		{"uaenorth", "UAE North"},
		{"centralindia", "Central India"},
		{"southeastasia", "Southeast Asia"},
		{"japaneast", "Japan East"},
		{"koreacentral", "Korea Central"},
		{"australiaeast", "Australia East"},
	},
}

func DefaultRegion(p CloudProvider) (string, bool) {
	regions := Regions[p]
	if len(regions) == 0 {
		return "", false
	}
	return regions[0].ID, true
}
