package aws

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
)

const (
	hoursPerMonth = cost.HoursPerMonth
	// A message is sent, received and deleted, each a request.
	requestsPerMessage = 3
	// The share of a bucket's requests taken as writes (PUT, COPY, POST, LIST), the rest reads.
	writeShare = 0.1
)

var engineNames = map[ir.Engine]string{
	ir.EnginePostgres: "PostgreSQL",
	ir.EngineMySQL:    "MySQL",
}

type costMatchers struct{}

func Cost() cost.Matchers { return costMatchers{} }

func (costMatchers) Node(n ir.Node, region string, u cost.NodeUsage) (cost.Item, bool, error) {
	var item cost.Item
	var err error
	switch n.Type {
	case ir.NodeDatabase:
		item, err = databaseCost(n, region)
	case ir.NodeService:
		item, err = serviceCost(n, region, u)
	case ir.NodeCache:
		item, err = cacheCost(n, region)
	case ir.NodeBucket:
		item, err = bucketCost(n, region, u)
	case ir.NodeFunction:
		item, err = functionCost(n, region, u)
	case ir.NodeGateway:
		item, err = gatewayCost(n, region, u)
	case ir.NodeQueue:
		item, err = queueCost(n, region, u)
	default:
		return cost.Item{}, false, nil
	}
	return item, true, err
}

func (costMatchers) Implicit(p *ir.Project, usage cost.Usage) []cost.Item {
	out := routedLoadBalancers(p, usage)
	if needsNetwork(p) {
		out = append(out, networkCost(p.Region, usage[cost.NetworkUsage].NatGb))
	}
	return out
}

// Every shape of node the matchers tell apart, so the refresh checks each meter they can name.
func (m costMatchers) Catalogue(region string) ([]cost.Lookup, error) {
	var nodes []ir.Node
	add := func(t ir.NodeType, props any) error {
		raw, err := json.Marshal(props)
		if err != nil {
			return err
		}
		nodes = append(nodes, ir.Node{ID: "catalogue", Type: t, Name: "catalogue", Properties: raw})
		return nil
	}
	for _, size := range ir.Sizes {
		for _, engine := range ir.EngineTypes {
			for _, ha := range []bool{false, true} {
				if err := add(ir.NodeDatabase, ir.DatabaseProps{Engine: engine, Size: size, StorageGB: 20, HighAvailability: ha}); err != nil {
					return nil, err
				}
			}
		}
		if err := add(ir.NodeService, ir.ServiceProps{Size: size, MinReplicas: ir.Ptr(1), Public: true}); err != nil {
			return nil, err
		}
		if err := add(ir.NodeCache, ir.CacheProps{Size: size}); err != nil {
			return nil, err
		}
		if err := add(ir.NodeFunction, ir.FunctionProps{Runtime: ir.RuntimeNode, Size: size}); err != nil {
			return nil, err
		}
	}
	if err := add(ir.NodeBucket, ir.BucketProps{}); err != nil {
		return nil, err
	}
	if err := add(ir.NodeGateway, ir.GatewayProps{}); err != nil {
		return nil, err
	}
	for _, fifo := range []bool{false, true} {
		if err := add(ir.NodeQueue, ir.QueueProps{FIFO: fifo}); err != nil {
			return nil, err
		}
	}

	items := []cost.Item{networkCost(region, 0)}
	for _, n := range nodes {
		item, _, err := m.Node(n, region, cost.NodeUsage{})
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	var out []cost.Lookup
	for _, item := range items {
		out = append(out, item.Lookups...)
		out = append(out, item.Usage...)
	}
	return out, nil
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

// The task definition sets no runtime platform, so the tasks run on x86 and the ARM meters
// must not match. A public service also pays for its load balancer's hours, and its capacity
// units on what it sends out.
func serviceCost(n ir.Node, region string, u cost.NodeUsage) (cost.Item, error) {
	p, err := ir.NodeProps[ir.ServiceProps](n)
	if err != nil {
		return cost.Item{}, fmt.Errorf("node '%s' has properties the aws matcher cannot read: %v", n.ID, err)
	}
	size, ok := fargateSizes[p.Size]
	if !ok {
		return cost.Item{}, fmt.Errorf("service '%s' has an unknown size '%s'", n.Name, p.Size)
	}
	tasks := float64(*p.MinReplicas)
	reach := "private"
	if p.Public {
		reach = "public"
	}
	item := cost.Item{
		Name: n.Name,
		Kind: string(n.Type),
		Summary: fmt.Sprintf("%s vCPU, %s GB, %s, %s",
			number(size.vCPU()), number(size.memoryGB()), count(*p.MinReplicas, "task"), reach),
		Lookups: []cost.Lookup{
			{
				Label:   "vcpu",
				Service: "AmazonECS",
				Filters: onDemand(region,
					cost.Filter{Attribute: "productFamily", Value: "Compute"},
					regional(`Fargate-vCPU-Hours:perCPU`),
				),
				Quantity: size.vCPU() * tasks * hoursPerMonth,
				Unit:     "vCPU-h",
			},
			{
				Label:   "memory",
				Service: "AmazonECS",
				Filters: onDemand(region,
					cost.Filter{Attribute: "productFamily", Value: "Compute"},
					regional(`Fargate-GB-Hours`),
				),
				Quantity: size.memoryGB() * tasks * hoursPerMonth,
				Unit:     "GB-h",
			},
		},
	}
	item.Usage = []cost.Lookup{egressLookup(region, u.EgressGb)}
	if p.Public {
		item.Lookups = append(item.Lookups, loadBalancerLookup(region))
		item.Usage = append(item.Usage, capacityUnitsLookup(region, u.EgressGb))
	}
	return item, nil
}

// A private service gets an internal load balancer only when a gateway routes to it, which
// the node alone cannot tell, so those are priced here: one per service however many routes.
func routedLoadBalancers(p *ir.Project, usage cost.Usage) []cost.Item {
	nodes := make(map[string]ir.Node, len(p.Nodes))
	for _, n := range p.Nodes {
		nodes[n.ID] = n
	}
	var out []cost.Item
	seen := map[string]bool{}
	for _, e := range p.Edges {
		from, to := nodes[e.From], nodes[e.To]
		if e.Relation != ir.RelRoutes || from.Type != ir.NodeGateway || to.Type != ir.NodeService || seen[to.ID] {
			continue
		}
		props, err := ir.NodeProps[ir.ServiceProps](to)
		if err != nil || props.Public {
			continue
		}
		seen[to.ID] = true
		out = append(out, cost.Item{
			Name:    to.Name,
			Kind:    "implicit ALB",
			Summary: "internal load balancer, routed from " + from.Name,
			Lookups: []cost.Lookup{loadBalancerLookup(p.Region)},
			Usage:   []cost.Lookup{capacityUnitsLookup(p.Region, usage[to.Name].EgressGb)},
		})
	}
	return out
}

func loadBalancerLookup(region string) cost.Lookup {
	return cost.Lookup{
		Label:   "load balancer",
		Service: "AWSELB",
		Filters: onDemand(region,
			cost.Filter{Attribute: "productFamily", Value: "Load Balancer-Application"},
			regional(`LoadBalancerUsage`),
		),
		Quantity: hoursPerMonth,
		Unit:     "h",
	}
}

// A capacity unit is the largest of four dimensions an hour, and the only one a design can say
// anything about is bytes, at 1 GB an hour per unit, so the egress stands in for all four.
func capacityUnitsLookup(region string, egressGb float64) cost.Lookup {
	return cost.Lookup{
		Label:   "capacity units",
		Service: "AWSELB",
		Filters: onDemand(region,
			cost.Filter{Attribute: "productFamily", Value: "Load Balancer-Application"},
			regional(`LCUUsage`),
		),
		Quantity: egressGb,
		Unit:     "LCU-h",
	}
}

// Data out to the internet is one meter for every service, priced from the region it leaves.
// The global free allowance is a row of its own with no region, so the filter passes it by.
func egressLookup(region string, egressGb float64) cost.Lookup {
	return cost.Lookup{
		Label:   "egress",
		Service: "AWSDataTransfer",
		Filters: onDemand(region,
			cost.Filter{Attribute: "productFamily", Value: "Data Transfer"},
			cost.Filter{Attribute: "transferType", Value: "AWS Outbound"},
			cost.Filter{Attribute: "toLocation", Value: "External"},
			regional(`DataTransfer-Out-Bytes`),
		),
		Quantity: egressGb,
		Unit:     "GB",
	}
}

// The extended support rows share the instance type and engine, so the usage type has to say
// plain node hours.
func cacheCost(n ir.Node, region string) (cost.Item, error) {
	p, err := ir.NodeProps[ir.CacheProps](n)
	if err != nil {
		return cost.Item{}, fmt.Errorf("node '%s' has properties the aws matcher cannot read: %v", n.ID, err)
	}
	instance, ok := cacheNodeType(region, p.Size)
	if !ok {
		return cost.Item{}, fmt.Errorf("cache '%s' has an unknown size '%s'", n.Name, p.Size)
	}
	return cost.Item{
		Name:    n.Name,
		Kind:    string(n.Type),
		Summary: fmt.Sprintf("%s, %s %s, %s", instance, cacheEngine, cacheEngineVersion, count(cacheNodes, "node")),
		Lookups: []cost.Lookup{{
			Label:   "node",
			Service: "AmazonElastiCache",
			Filters: onDemand(region,
				cost.Filter{Attribute: "productFamily", Value: "Cache Instance"},
				cost.Filter{Attribute: "instanceType", Value: instance},
				cost.Filter{Attribute: "cacheEngine", Value: "Redis"},
				regional(`NodeUsage:`+regexp.QuoteMeta(instance)),
			),
			Quantity: cacheNodes * hoursPerMonth,
			Unit:     "h",
		}},
	}, nil
}

// The annotation rows share S3's storage class and request groups, so the usage types say
// which is meant. One requests figure covers both request meters, split a tenth writes.
func bucketCost(n ir.Node, region string, u cost.NodeUsage) (cost.Item, error) {
	p, err := ir.NodeProps[ir.BucketProps](n)
	if err != nil {
		return cost.Item{}, fmt.Errorf("node '%s' has properties the aws matcher cannot read: %v", n.ID, err)
	}
	versioning, access := "unversioned", "private"
	if p.Versioning {
		versioning = "versioned"
	}
	if p.Public {
		access = "public"
	}
	requests, err := u.Requests.PerMonth()
	if err != nil {
		return cost.Item{}, fmt.Errorf("bucket '%s': requests %v", n.Name, err)
	}
	return cost.Item{
		Name:    n.Name,
		Kind:    string(n.Type),
		Summary: fmt.Sprintf("standard storage, %s, %s", versioning, access),
		Usage: []cost.Lookup{
			{
				Label:   "storage",
				Service: "AmazonS3",
				Filters: onDemand(region,
					cost.Filter{Attribute: "productFamily", Value: "Storage"},
					cost.Filter{Attribute: "storageClass", Value: "General Purpose"},
					cost.Filter{Attribute: "volumeType", Value: "Standard"},
					regional(`TimedStorage-ByteHrs`),
				),
				Quantity: u.StorageGb,
				Unit:     "GB",
			},
			{
				Label:   "put requests (1 in 10)",
				Service: "AmazonS3",
				Filters: onDemand(region,
					cost.Filter{Attribute: "productFamily", Value: "API Request"},
					cost.Filter{Attribute: "group", Value: "S3-API-Tier1"},
					regional(`Requests-Tier1`),
				),
				Quantity: math.Round(requests * writeShare),
				Unit:     "requests",
			},
			{
				Label:   "get requests (9 in 10)",
				Service: "AmazonS3",
				Filters: onDemand(region,
					cost.Filter{Attribute: "productFamily", Value: "API Request"},
					cost.Filter{Attribute: "group", Value: "S3-API-Tier2"},
					regional(`Requests-Tier2`),
				),
				Quantity: math.Round(requests * (1 - writeShare)),
				Unit:     "requests",
			},
			egressLookup(region, u.EgressGb),
		},
	}, nil
}

// The hourly usage type carries a region prefix in most files (EUW2-NatGateway-Hours) and none
// in us-east-1, and the regional variant must not match, hence the pattern. GCP's implicit
// network carries a Cloud NAT too once a caller needs all-traffic egress; it is not priced yet.
func networkCost(region string, natGb float64) cost.Item {
	natGateway := func(usage string) cost.Filter {
		return cost.Filter{Attribute: "usagetype", Value: `(\w+-)?NatGateway-` + usage, Pattern: true}
	}
	return cost.Item{
		Name: networkLabel,
		Kind: "implicit VPC",
		Lookups: []cost.Lookup{{
			Label:   "nat gateway",
			Service: "AmazonEC2",
			Filters: []cost.Filter{
				{Attribute: "productFamily", Value: "NAT Gateway"},
				natGateway("Hours"),
				{Attribute: "termType", Value: "OnDemand"},
				{Attribute: "regionCode", Value: region},
			},
			Quantity: hoursPerMonth,
			Unit:     "h",
		}},
		Usage: []cost.Lookup{{
			Label:   "nat gateway data",
			Service: "AmazonEC2",
			Filters: []cost.Filter{
				{Attribute: "productFamily", Value: "NAT Gateway"},
				natGateway("Bytes"),
				{Attribute: "termType", Value: "OnDemand"},
				{Attribute: "regionCode", Value: region},
			},
			Quantity: natGb,
			Unit:     "GB",
		}},
	}
}

// The function runs on x86_64, Lambda's default when architectures is not set, so the arm64
// meters (a fifth cheaper) are not the ones to price. Duration is GB-seconds: every run for
// its length at the size's memory.
func functionCost(n ir.Node, region string, u cost.NodeUsage) (cost.Item, error) {
	p, err := ir.NodeProps[ir.FunctionProps](n)
	if err != nil {
		return cost.Item{}, fmt.Errorf("node '%s' has properties the aws matcher cannot read: %v", n.ID, err)
	}
	memory, ok := lambdaMemory[p.Size]
	if !ok {
		return cost.Item{}, fmt.Errorf("function '%s' has an unknown size '%s'", n.Name, p.Size)
	}
	invocations, err := u.Invocations.PerMonth()
	if err != nil {
		return cost.Item{}, fmt.Errorf("function '%s': invocations %v", n.Name, err)
	}
	common := []cost.Filter{
		{Attribute: "productFamily", Value: "Serverless"},
		{Attribute: "termType", Value: "OnDemand"},
		{Attribute: "regionCode", Value: region},
	}
	return cost.Item{
		Name:    n.Name,
		Kind:    string(n.Type),
		Summary: fmt.Sprintf("%s, %d MB, x86_64", p.Runtime, int(memory)),
		Usage: []cost.Lookup{
			{
				Label:   "requests",
				Service: "AWSLambda",
				Filters: append([]cost.Filter{
					{Attribute: "group", Value: "AWS-Lambda-Requests"},
					{Attribute: "usagetype", Value: `(\w+-)?Request`, Pattern: true},
				}, common...),
				Quantity: invocations,
				Unit:     "requests",
			},
			{
				Label:   "duration",
				Service: "AWSLambda",
				Filters: append([]cost.Filter{
					{Attribute: "group", Value: "AWS-Lambda-Duration"},
					{Attribute: "usagetype", Value: `(\w+-)?Lambda-GB-Second`, Pattern: true},
				}, common...),
				Quantity: invocations * u.DurationMs / 1000 * memory / 1024,
				Unit:     "GB-s",
			},
		},
	}, nil
}

// The resolver makes an HTTP API, whose request meter is a third of the REST API's.
func gatewayCost(n ir.Node, region string, u cost.NodeUsage) (cost.Item, error) {
	requests, err := u.Requests.PerMonth()
	if err != nil {
		return cost.Item{}, fmt.Errorf("gateway '%s': requests %v", n.Name, err)
	}
	return cost.Item{
		Name:    n.Name,
		Kind:    string(n.Type),
		Summary: "HTTP API",
		Usage: []cost.Lookup{{
			Label:   "requests",
			Service: "AmazonApiGateway",
			Filters: []cost.Filter{
				{Attribute: "productFamily", Value: "API Calls"},
				{Attribute: "usagetype", Value: `(\w+-)?ApiGatewayHttpRequest`, Pattern: true},
				{Attribute: "termType", Value: "OnDemand"},
				{Attribute: "regionCode", Value: region},
			},
			Quantity: requests,
			Unit:     "requests",
		}},
		Unpriced: []string{"data transfer"},
	}, nil
}

func queueCost(n ir.Node, region string, u cost.NodeUsage) (cost.Item, error) {
	p, err := ir.NodeProps[ir.QueueProps](n)
	if err != nil {
		return cost.Item{}, fmt.Errorf("node '%s' has properties the aws matcher cannot read: %v", n.ID, err)
	}
	queueType, shown := "Standard", "standard"
	if p.FIFO {
		queueType, shown = "FIFO (first-in, first-out)", "FIFO"
	}
	messages, err := u.Messages.PerMonth()
	if err != nil {
		return cost.Item{}, fmt.Errorf("queue '%s': messages %v", n.Name, err)
	}
	return cost.Item{
		Name:    n.Name,
		Kind:    string(n.Type),
		Summary: shown,
		Usage: []cost.Lookup{{
			Label:   fmt.Sprintf("requests (%d per message)", requestsPerMessage),
			Service: "AWSQueueService",
			Filters: []cost.Filter{
				{Attribute: "productFamily", Value: "API Request"},
				{Attribute: "queueType", Value: queueType},
				{Attribute: "termType", Value: "OnDemand"},
				{Attribute: "regionCode", Value: region},
			},
			Quantity: messages * requestsPerMessage,
			Unit:     "requests",
		}},
	}, nil
}

func onDemand(region string, filters ...cost.Filter) []cost.Filter {
	return append(filters,
		cost.Filter{Attribute: "termType", Value: "OnDemand"},
		cost.Filter{Attribute: "regionCode", Value: region},
	)
}

// Usage types carry a region prefix such as EUW2- in every file but us-east-1's, with
// eu-west-1 keeping the old EU-. The prefix has to look like one: TS-LoadBalancerUsage is the
// trust store hour, not a region.
func regional(usage string) cost.Filter {
	return cost.Filter{Attribute: "usagetype", Value: `(EU-|[A-Z]+\d-)?` + usage, Pattern: true}
}

func number(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
