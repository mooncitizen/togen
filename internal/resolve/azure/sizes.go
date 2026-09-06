package azure

import "github.com/mooncitizen/togen/internal/ir"

// The same names serve both flexible servers. Small is the burstable tier, which has no
// standby, so zone redundant high availability starts at medium.
var dbSizes = map[ir.Size]string{
	ir.SizeSmall:  "B_Standard_B1ms",
	ir.SizeMedium: "GP_Standard_D2ds_v4",
	ir.SizeLarge:  "GP_Standard_D4ds_v4",
}
