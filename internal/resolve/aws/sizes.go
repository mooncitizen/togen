package aws

import "github.com/mooncitizen/togen/internal/ir"

var dbSizes = map[ir.Size]string{
	ir.SizeSmall:  "db.t4g.micro",
	ir.SizeMedium: "db.t4g.medium",
	ir.SizeLarge:  "db.r6g.large",
}

var lambdaMemory = map[ir.Size]float64{
	ir.SizeSmall:  512,
	ir.SizeMedium: 1024,
	ir.SizeLarge:  2048,
}
