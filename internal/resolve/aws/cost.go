package aws

import (
	"encoding/json"
	"fmt"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
)

const hoursPerMonth = 730

var engineNames = map[ir.Engine]string{
	ir.EnginePostgres: "PostgreSQL",
	ir.EngineMySQL:    "MySQL",
}

type costMatchers struct{}

func Cost() cost.Matchers { return costMatchers{} }

func (costMatchers) Node(n ir.Node, region string) (cost.Item, bool, error) {
	if n.Type != ir.NodeDatabase {
		return cost.Item{}, false, nil
	}
	item, err := databaseCost(n, region)
	return item, true, err
}

func (costMatchers) Implicit(p *ir.Project) []cost.Item {
	if !needsNetwork(p) {
		return nil
	}
	return []cost.Item{networkCost(p.Region)}
}

func (costMatchers) Catalogue(region string) ([]cost.Lookup, error) {
	var out []cost.Lookup
	for _, size := range ir.Sizes {
		for _, engine := range ir.EngineTypes {
			for _, ha := range []bool{false, true} {
				props, err := json.Marshal(ir.DatabaseProps{Engine: engine, Size: size, StorageGB: 20, HighAvailability: ha})
				if err != nil {
					return nil, err
				}
				item, err := databaseCost(ir.Node{ID: "catalogue", Type: ir.NodeDatabase, Name: "catalogue", Properties: props}, region)
				if err != nil {
					return nil, err
				}
				out = append(out, item.Lookups...)
			}
		}
	}
	return append(out, networkCost(region).Lookups...), nil
}

func databaseCost(n ir.Node, region string) (cost.Item, error) {
	p, err := ir.NodeProps[ir.DatabaseProps](n)
	if err != nil {
		return cost.Item{}, fmt.Errorf("node '%s' has properties the aws matcher cannot read: %v", n.ID, err)
	}
	engine, ok := engineNames[p.Engine]
	if !ok {
		return cost.Item{}, fmt.Errorf("database '%s' has an unknown engine '%s'", n.Name, p.Engine)
	}
	instance, ok := dbSizes[p.Size]
	if !ok {
		return cost.Item{}, fmt.Errorf("database '%s' has an unknown size '%s'", n.Name, p.Size)
	}
	version := p.Version
	if version == "" {
		version, _ = ir.DefaultEngineVersion(ir.ProviderAWS, p.Engine)
	}
	deployment, shown := "Single-AZ", "single-AZ"
	if p.HighAvailability {
		deployment, shown = "Multi-AZ", "multi-AZ"
	}
	common := []cost.Filter{
		{Attribute: "databaseEngine", Value: engine},
		{Attribute: "deploymentOption", Value: deployment},
		{Attribute: "termType", Value: "OnDemand"},
		{Attribute: "regionCode", Value: region},
	}
	return cost.Item{
		Name:    n.Name,
		Kind:    string(n.Type),
		Summary: fmt.Sprintf("%s, %s %s, %s, %d GB", instance, p.Engine, version, shown, p.StorageGB),
		Lookups: []cost.Lookup{
			{
				Label:   "instance",
				Service: "AmazonRDS",
				Filters: append([]cost.Filter{
					{Attribute: "productFamily", Value: "Database Instance"},
					{Attribute: "instanceType", Value: instance},
				}, common...),
				Quantity: hoursPerMonth,
				Unit:     "h",
			},
			{
				Label:   "storage gp2",
				Service: "AmazonRDS",
				Filters: append([]cost.Filter{
					{Attribute: "productFamily", Value: "Database Storage"},
					{Attribute: "volumeType", Value: "General Purpose"},
				}, common...),
				Quantity: float64(p.StorageGB),
				Unit:     "GB",
			},
		},
		Unpriced: []string{fmt.Sprintf("backups beyond %d GB", p.StorageGB)},
	}, nil
}

// The hourly usage type carries a region prefix in most files (EUW2-NatGateway-Hours) and none
// in us-east-1, and the regional variant must not match, hence the pattern.
func networkCost(region string) cost.Item {
	return cost.Item{
		Name: networkLabel,
		Kind: "implicit VPC",
		Lookups: []cost.Lookup{{
			Label:   "nat gateway",
			Service: "AmazonEC2",
			Filters: []cost.Filter{
				{Attribute: "productFamily", Value: "NAT Gateway"},
				{Attribute: "usagetype", Value: `(\w+-)?NatGateway-Hours`, Pattern: true},
				{Attribute: "termType", Value: "OnDemand"},
				{Attribute: "regionCode", Value: region},
			},
			Quantity: hoursPerMonth,
			Unit:     "h",
		}},
		Unpriced: []string{"nat gateway data processed"},
	}
}
