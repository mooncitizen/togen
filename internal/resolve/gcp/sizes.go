package gcp

import "github.com/mooncitizen/togen/internal/ir"

var functionMemory = map[ir.Size]string{
	ir.SizeSmall:  "512Mi",
	ir.SizeMedium: "1Gi",
	ir.SizeLarge:  "2Gi",
}
