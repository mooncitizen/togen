package aws

import (
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
)

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

// me-south-1 and me-central-1 offer no t4g cache nodes, so the burstable sizes are t3 there.
func cacheNodeType(region string, size ir.Size) (string, bool) {
	instance, ok := cacheSizes[size]
	if ok && (region == "me-south-1" || region == "me-central-1") {
		instance = strings.Replace(instance, ".t4g.", ".t3.", 1)
	}
	return instance, ok
}

var lambdaMemory = map[ir.Size]float64{
	ir.SizeSmall:  512,
	ir.SizeMedium: 1024,
	ir.SizeLarge:  2048,
}

// Fargate only accepts fixed cpu and memory pairs, cpu in 1024ths of a vCPU and memory in MiB.
type fargateSize struct{ CPU, Memory int }

var fargateSizes = map[ir.Size]fargateSize{
	ir.SizeSmall:  {CPU: 256, Memory: 512},
	ir.SizeMedium: {CPU: 1024, Memory: 2048},
	ir.SizeLarge:  {CPU: 2048, Memory: 4096},
}

func (s fargateSize) vCPU() float64 { return float64(s.CPU) / 1024 }

func (s fargateSize) memoryGB() float64 { return float64(s.Memory) / 1024 }
