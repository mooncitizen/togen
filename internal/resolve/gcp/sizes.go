package gcp

import "github.com/mooncitizen/togen/internal/ir"

var functionMemory = map[ir.Size]string{
	ir.SizeSmall:  "512Mi",
	ir.SizeMedium: "1Gi",
	ir.SizeLarge:  "2Gi",
}

// Custom tiers are db-custom-<vcpu>-<mb>, memory a multiple of 256 within 0.9 to 6.5 GB per vCPU.
var databaseTiers = map[ir.Size]string{
	ir.SizeSmall:  "db-f1-micro",
	ir.SizeMedium: "db-custom-2-7680",
	ir.SizeLarge:  "db-custom-4-15360",
}
