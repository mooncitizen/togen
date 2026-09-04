package aws

import "github.com/mooncitizen/togen/internal/ir"

var dbSizes = map[ir.Size]string{
	ir.SizeSmall:  "db.t4g.micro",
	ir.SizeMedium: "db.t4g.medium",
	ir.SizeLarge:  "db.r6g.large",
}

var cacheSizes = map[ir.Size]string{
	ir.SizeSmall:  "cache.t4g.micro",
	ir.SizeMedium: "cache.t4g.medium",
	ir.SizeLarge:  "cache.r7g.large",
}

var lambdaMemory = map[ir.Size]float64{
	ir.SizeSmall:  512,
	ir.SizeMedium: 1024,
	ir.SizeLarge:  2048,
}

// Fargate only accepts fixed cpu and memory pairs, and the provider takes both as strings.
type fargateSize struct{ CPU, Memory string }

var fargateSizes = map[ir.Size]fargateSize{
	ir.SizeSmall:  {CPU: "256", Memory: "512"},
	ir.SizeMedium: {CPU: "1024", Memory: "2048"},
	ir.SizeLarge:  {CPU: "2048", Memory: "4096"},
}
