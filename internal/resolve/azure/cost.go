package azure

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
)

const (
	hoursPerMonth = cost.HoursPerMonth
	// A message is sent, received and completed, each an operation.
	operationsPerMessage = 3
	// The share of a bucket's requests taken as writes, the rest reads.
	writeShare      = 0.1
	regionAttribute = "armRegionName"
	busLabel        = "bus"
)

// The consumption plan bills the memory a run observes, which the size cannot say, so each
// size is priced at a plausible footprint under the plan's 1.5 GB ceiling.
var functionMemoryGB = map[ir.Size]float64{
	ir.SizeSmall:  0.5,
	ir.SizeMedium: 1,
	ir.SizeLarge:  1.5,
}

type flexibleMeter struct {
	product string
	sku     string
	meter   string
	vCores  float64
}

// The burstable tier is one meter per size; general purpose is one vCore meter per series,
// listed under two sku names that share it.
var flexibleMeters = map[ir.Engine]map[ir.Size]flexibleMeter{
	ir.EnginePostgres: {
		ir.SizeSmall:  {product: "Azure Database for PostgreSQL Flexible Server Burstable BS Series Compute", sku: "B1MS", meter: "B1MS"},
		ir.SizeMedium: {product: "Azure Database for PostgreSQL Flexible Server General Purpose Ddsv4 Series Compute", meter: "vCore", vCores: 2},
		ir.SizeLarge:  {product: "Azure Database for PostgreSQL Flexible Server General Purpose Ddsv4 Series Compute", meter: "vCore", vCores: 4},
	},
	ir.EngineMySQL: {
		ir.SizeSmall:  {product: "Azure Database for MySQL Flexible Server Burstable BS Series Compute", sku: "B1MS", meter: "B1MS"},
		ir.SizeMedium: {product: "Azure Database for MySQL Flexible Server General Purpose Series Compute", meter: "vCore", vCores: 2},
		ir.SizeLarge:  {product: "Azure Database for MySQL Flexible Server General Purpose Series Compute", meter: "vCore", vCores: 4},
	},
}

var flexibleStorage = map[ir.Engine]struct{ service, product string }{
	ir.EnginePostgres: {"Azure Database for PostgreSQL", "Azure Database for PostgreSQL Flex Server Storage"},
	ir.EngineMySQL:    {"Azure Database for MySQL", "Azure Database for MySQL Flexible Server Storage"},
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
		item = gatewayCost(n)
	case ir.NodeQueue:
		item, err = queueCost(n, region, u)
	default:
		return cost.Item{}, false, nil
	}
	return item, true, err
}

// The virtual network is free and the resolver makes no NAT gateway, so the one implicit
// charge is the Service Bus namespace's base unit once a fifo queue moves it to Standard.
func (costMatchers) Implicit(p *ir.Project, _ cost.Usage) []cost.Item {
	if slices.ContainsFunc(p.Nodes, func(n ir.Node) bool {
		if n.Type != ir.NodeQueue {
			return false
		}
		props, err := ir.NodeProps[ir.QueueProps](n)
		return err == nil && props.FIFO
	}) {
		return []cost.Item{namespaceCost(p.Region)}
	}
	return nil
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

	items := []cost.Item{namespaceCost(region)}
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

// Zone redundant high availability runs a standby with its own compute and storage, so both
// meters double. Postgres provisions storage in tiers, so the priced size is the tier's.
func databaseCost(n ir.Node, region string) (cost.Item, error) {
	p, err := ir.NodeProps[ir.DatabaseProps](n)
	if err != nil {
		return cost.Item{}, fmt.Errorf("node '%s' has properties the azure matcher cannot read: %v", n.ID, err)
	}
	meters, ok := flexibleMeters[p.Engine]
	if !ok {
		return cost.Item{}, fmt.Errorf("database '%s' has an unknown engine '%s'", n.Name, p.Engine)
	}
	meter, ok := meters[p.Size]
	if !ok {
		return cost.Item{}, fmt.Errorf("database '%s' has an unknown size '%s'", n.Name, p.Size)
	}
	version := p.Version
	if version == "" {
		version, _ = ir.DefaultEngineVersion(ir.ProviderAzure, p.Engine)
	}
	storageGB := p.StorageGB
	if p.Engine == ir.EnginePostgres {
		storageGB = postgresStorageGB(p.StorageGB)
	}
	servers, zones := 1.0, "single zone"
	if p.HighAvailability {
		servers, zones = 2, "zone redundant"
	}
	compute := cost.Lookup{
		Label:   "compute",
		Service: flexibleStorage[p.Engine].service,
		Filters: []cost.Filter{
			{Attribute: "productName", Value: meter.product},
			{Attribute: "meterName", Value: meter.meter},
			{Attribute: regionAttribute, Value: region},
		},
		Quantity: hoursPerMonth * servers,
		Unit:     "h",
	}
	if meter.sku != "" {
		compute.Filters = slices.Insert(compute.Filters, 1, cost.Filter{Attribute: "skuName", Value: meter.sku})
	}
	if meter.vCores > 0 {
		compute.Quantity *= meter.vCores
		compute.Unit = "vCore-h"
	}
	return cost.Item{
		Name:    n.Name,
		Kind:    string(n.Type),
		Summary: fmt.Sprintf("%s, %s %s, %s, %d GB", dbSizes[p.Size], p.Engine, version, zones, storageGB),
		Lookups: []cost.Lookup{
			compute,
			{
				Label:   "storage",
				Service: flexibleStorage[p.Engine].service,
				Filters: []cost.Filter{
					{Attribute: "productName", Value: flexibleStorage[p.Engine].product},
					{Attribute: "skuName", Value: "Storage"},
					{Attribute: "meterName", Value: "Storage Data Stored"},
					{Attribute: regionAttribute, Value: region},
				},
				Quantity: float64(storageGB) * servers,
				Unit:     "GB",
			},
		},
		Unpriced: []string{fmt.Sprintf("backups beyond %d GB", storageGB)},
	}, nil
}

func postgresStorageGB(gb int) int {
	need := float64(gb) * 1024
	i := slices.IndexFunc(postgresStorageTiers, func(tier float64) bool { return tier >= need })
	if i < 0 {
		i = len(postgresStorageTiers) - 1
	}
	return int(postgresStorageTiers[i] / 1024)
}

// The Consumption profile bills vCPU-seconds and GiB-seconds; the snapshot holds them per
// hour. Replicas at minReplicas are priced active for the month, the rate a replica serving
// requests pays, so an idle one costs less than the line says.
func serviceCost(n ir.Node, region string, u cost.NodeUsage) (cost.Item, error) {
	p, err := ir.NodeProps[ir.ServiceProps](n)
	if err != nil {
		return cost.Item{}, fmt.Errorf("node '%s' has properties the azure matcher cannot read: %v", n.ID, err)
	}
	size, ok := appSizes[p.Size]
	if !ok {
		return cost.Item{}, fmt.Errorf("service '%s' has an unknown size '%s'", n.Name, p.Size)
	}
	memory, err := strconv.ParseFloat(strings.TrimSuffix(size.memory, "Gi"), 64)
	if err != nil {
		return cost.Item{}, fmt.Errorf("service '%s': memory %q: %v", n.Name, size.memory, err)
	}
	requests, err := u.Requests.PerMonth()
	if err != nil {
		return cost.Item{}, fmt.Errorf("service '%s': requests %v", n.Name, err)
	}
	replicas := float64(*p.MinReplicas)
	reach := "private"
	if p.Public {
		reach = "public"
	}
	return cost.Item{
		Name: n.Name,
		Kind: string(n.Type),
		Summary: fmt.Sprintf("%s vCPU, %s GiB, %s, %s",
			number(size.cpu), number(memory), count(*p.MinReplicas, "replica"), reach),
		Lookups: []cost.Lookup{
			{
				Label:    "vcpu",
				Service:  "Azure Container Apps",
				Filters:  containerApps(region, "Standard vCPU Active Usage"),
				Quantity: size.cpu * replicas * hoursPerMonth,
				Unit:     "vCPU-h",
			},
			{
				Label:    "memory",
				Service:  "Azure Container Apps",
				Filters:  containerApps(region, "Standard Memory Active Usage"),
				Quantity: memory * replicas * hoursPerMonth,
				Unit:     "GiB-h",
			},
		},
		Usage: []cost.Lookup{
			{
				Label:    "requests",
				Service:  "Azure Container Apps",
				Filters:  containerApps(region, "Standard Requests"),
				Quantity: requests,
				Unit:     "requests",
			},
			egressLookup(region, u.EgressGb),
		},
	}, nil
}

func containerApps(region, meter string) []cost.Filter {
	return []cost.Filter{
		{Attribute: "productName", Value: "Azure Container Apps"},
		{Attribute: "skuName", Value: "Standard"},
		{Attribute: "meterName", Value: meter},
		{Attribute: regionAttribute, Value: region},
	}
}

// Internet egress is one Bandwidth meter for everything in the region, priced on the default
// routing preference (Microsoft's network), which the API names two ways and does not list in
// Poland Central, where the Internet routing meter stands in. The first 100 GB a month are a
// free tier row the refresh passes over.
func egressLookup(region string, egressGb float64) cost.Lookup {
	product := cost.Filter{Attribute: "productName", Value: `.*(MGN|Microsoft Global Network).*`, Pattern: true}
	if region == "polandcentral" {
		product = cost.Filter{Attribute: "productName", Value: "Bandwidth - Routing Preference: Internet"}
	}
	return cost.Lookup{
		Label:   "egress",
		Service: "Bandwidth",
		Filters: []cost.Filter{
			product,
			{Attribute: "skuName", Value: "Standard"},
			{Attribute: "meterName", Value: "Standard Data Transfer Out"},
			{Attribute: regionAttribute, Value: region},
		},
		Quantity: egressGb,
		Unit:     "GB",
	}
}

// Basic is one node on its "Cache" meter. Standard is a primary and a replica; its whole-cache
// meter is missing from some regions, so the per-node "Cache Instance" meter is priced twice.
func cacheCost(n ir.Node, region string) (cost.Item, error) {
	p, err := ir.NodeProps[ir.CacheProps](n)
	if err != nil {
		return cost.Item{}, fmt.Errorf("node '%s' has properties the azure matcher cannot read: %v", n.ID, err)
	}
	sku, ok := cacheSizes[p.Size]
	if !ok {
		return cost.Item{}, fmt.Errorf("cache '%s' has an unknown size '%s'", n.Name, p.Size)
	}
	name := fmt.Sprintf("%s%d", sku.family, int(sku.capacity))
	nodes, meter := 1, name+" Cache"
	if sku.name == "Standard" {
		nodes, meter = 2, name+" Cache Instance"
	}
	return cost.Item{
		Name:    n.Name,
		Kind:    string(n.Type),
		Summary: fmt.Sprintf("%s %s, redis %s, %s", sku.name, name, redisVersion, count(nodes, "node")),
		Lookups: []cost.Lookup{{
			Label:   "node",
			Service: "Redis Cache",
			Filters: []cost.Filter{
				{Attribute: "productName", Value: "Azure Redis Cache " + sku.name},
				{Attribute: "skuName", Value: name},
				{Attribute: "meterName", Value: meter},
				{Attribute: regionAttribute, Value: region},
			},
			Quantity: float64(nodes) * hoursPerMonth,
			Unit:     "h",
		}},
	}, nil
}

// One requests figure covers both operation meters, split a tenth writes.
func bucketCost(n ir.Node, region string, u cost.NodeUsage) (cost.Item, error) {
	p, err := ir.NodeProps[ir.BucketProps](n)
	if err != nil {
		return cost.Item{}, fmt.Errorf("node '%s' has properties the azure matcher cannot read: %v", n.ID, err)
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
	blob := func(meter string) []cost.Filter {
		return []cost.Filter{
			{Attribute: "productName", Value: "General Block Blob v2"},
			{Attribute: "skuName", Value: "Hot LRS"},
			{Attribute: "meterName", Value: meter},
			{Attribute: regionAttribute, Value: region},
		}
	}
	return cost.Item{
		Name:    n.Name,
		Kind:    string(n.Type),
		Summary: fmt.Sprintf("hot block blob, LRS, %s, %s", versioning, access),
		Usage: []cost.Lookup{
			{Label: "storage", Service: "Storage", Filters: blob("Hot LRS Data Stored"), Quantity: u.StorageGb, Unit: "GB"},
			{Label: "put requests (1 in 10)", Service: "Storage", Filters: blob("Hot LRS Write Operations"), Quantity: math.Round(requests * writeShare), Unit: "requests"},
			{Label: "get requests (9 in 10)", Service: "Storage", Filters: blob("Hot Read Operations"), Quantity: math.Round(requests * (1 - writeShare)), Unit: "requests"},
			egressLookup(region, u.EgressGb),
		},
	}, nil
}

// Duration is GB-seconds: every run for its length at the size's memory.
func functionCost(n ir.Node, region string, u cost.NodeUsage) (cost.Item, error) {
	p, err := ir.NodeProps[ir.FunctionProps](n)
	if err != nil {
		return cost.Item{}, fmt.Errorf("node '%s' has properties the azure matcher cannot read: %v", n.ID, err)
	}
	memory, ok := functionMemoryGB[p.Size]
	if !ok {
		return cost.Item{}, fmt.Errorf("function '%s' has an unknown size '%s'", n.Name, p.Size)
	}
	invocations, err := u.Invocations.PerMonth()
	if err != nil {
		return cost.Item{}, fmt.Errorf("function '%s': invocations %v", n.Name, err)
	}
	consumption := func(meter string) []cost.Filter {
		return []cost.Filter{
			{Attribute: "productName", Value: "Functions"},
			{Attribute: "skuName", Value: "Standard"},
			{Attribute: "meterName", Value: meter},
			{Attribute: regionAttribute, Value: region},
		}
	}
	return cost.Item{
		Name:    n.Name,
		Kind:    string(n.Type),
		Summary: fmt.Sprintf("%s, consumption plan, %d MB", p.Runtime, int(memory*1024)),
		Usage: []cost.Lookup{
			{Label: "executions", Service: "Functions", Filters: consumption("Standard Total Executions"), Quantity: invocations, Unit: "executions"},
			{Label: "duration", Service: "Functions", Filters: consumption("Standard Execution Time"), Quantity: invocations * u.DurationMs / 1000 * memory, Unit: "GB-s"},
		},
	}, nil
}

// There is no gateway resource on Azure (ADR 0010): the routed targets answer on their own
// URLs, so there is nothing to meter.
func gatewayCost(n ir.Node) cost.Item {
	return cost.Item{
		Name:    n.Name,
		Kind:    string(n.Type),
		Summary: "no resource, routes go to the targets' own URLs",
	}
}

// A queue is priced on the tier its own fifo property implies. The namespace is shared, so a
// plain queue beside a fifo one really pays the Standard operation rate; the Standard base
// unit comes once, as an implicit item.
func queueCost(n ir.Node, region string, u cost.NodeUsage) (cost.Item, error) {
	p, err := ir.NodeProps[ir.QueueProps](n)
	if err != nil {
		return cost.Item{}, fmt.Errorf("node '%s' has properties the azure matcher cannot read: %v", n.ID, err)
	}
	tier, shown := "Basic", "standard"
	if p.FIFO {
		tier, shown = "Standard", "FIFO (sessions)"
	}
	messages, err := u.Messages.PerMonth()
	if err != nil {
		return cost.Item{}, fmt.Errorf("queue '%s': messages %v", n.Name, err)
	}
	return cost.Item{
		Name:    n.Name,
		Kind:    string(n.Type),
		Summary: fmt.Sprintf("%s, %s namespace", shown, tier),
		Usage: []cost.Lookup{{
			Label:    fmt.Sprintf("operations (%d per message)", operationsPerMessage),
			Service:  "Service Bus",
			Filters:  serviceBus(region, tier, tier+" Messaging Operations"),
			Quantity: messages * operationsPerMessage,
			Unit:     "operations",
		}},
	}, nil
}

// The base unit is listed per hour everywhere and per month in most regions, so the hour is
// the one priced.
func namespaceCost(region string) cost.Item {
	return cost.Item{
		Name:    busLabel,
		Kind:    "implicit namespace",
		Summary: "Service Bus Standard, shared by the queues",
		Lookups: []cost.Lookup{{
			Label:    "base charge",
			Service:  "Service Bus",
			Filters:  append(serviceBus(region, "Standard", "Standard Base Unit"), cost.Filter{Attribute: "unitOfMeasure", Value: "1/Hour"}),
			Quantity: hoursPerMonth,
			Unit:     "h",
		}},
	}
}

func serviceBus(region, tier, meter string) []cost.Filter {
	return []cost.Filter{
		{Attribute: "productName", Value: "Service Bus"},
		{Attribute: "skuName", Value: tier},
		{Attribute: "meterName", Value: meter},
		{Attribute: regionAttribute, Value: region},
	}
}

func number(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
