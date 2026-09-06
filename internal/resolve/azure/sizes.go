package azure

import "github.com/mooncitizen/togen/internal/ir"

// The same names serve both flexible servers. Small is the burstable tier, which has no
// standby, so zone redundant high availability starts at medium.
var dbSizes = map[ir.Size]string{
	ir.SizeSmall:  "B_Standard_B1ms",
	ir.SizeMedium: "GP_Standard_D2ds_v4",
	ir.SizeLarge:  "GP_Standard_D4ds_v4",
}

type appSize struct {
	cpu    float64
	memory string
}

// The Consumption profile accepts fixed cpu and memory pairs, so these are not free choices.
var appSizes = map[ir.Size]appSize{
	ir.SizeSmall:  {0.25, "0.5Gi"},
	ir.SizeMedium: {0.5, "1Gi"},
	ir.SizeLarge:  {1, "2Gi"},
}
