package gcp

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
)

// The services the matchers name, as the Cloud Billing Catalog lists them, with the ids the
// refresh fetches them by. The connector is Compute Engine instances and the NAT is Networking.
var CatalogServices = map[string]string{
	"Cloud SQL":                   "9662-B51E-5089",
	"Cloud Run":                   "152E-C115-5142",
	"Cloud Run Functions":         "29E7-DA93-CA13",
	"Cloud Pub/Sub":               "A1E8-BE35-7EBC",
	"Cloud Storage":               "95FF-2EF5-5EA1",
	"Cloud Memorystore for Redis": "5AF5-2C11-D467",
	"Compute Engine":              "6F81-5844-456A",
	"Networking":                  "E505-1604-58F8",
}

const (
	hoursPerMonth  = cost.HoursPerMonth
	secondsPerHour = 3600
	tebibyte       = 1024
	gibibyte       = 1 << 30
	// Pub/Sub bills a message as at least 1 KB, once when published and once when delivered.
	bytesPerMessage = 2 * 1024
	// The share of a bucket's requests taken as writes (class A), the rest reads (class B).
	writeShare = 0.1
	// The connector's instances are e2-micro, a quarter of a core and a gigabyte each, billed as
	// Compute Engine VMs and counted by the NAT.
	connectorVCPU     = 0.25
	connectorMemoryGB = 1
)

var (
	sqlEngineNames = map[ir.Engine]string{
		ir.EnginePostgres: "Cloud SQL for PostgreSQL",
		ir.EngineMySQL:    "Cloud SQL for MySQL",
	}
	sharedCoreTiers = map[string]string{"db-f1-micro": "Micro instance", "db-g1-small": "Small instance"}
	customTier      = regexp.MustCompile(`^db-custom-(\d+)-(\d+)$`)
	// vCPUs a 2nd gen function gets for its memory.
	functionVCPU = map[ir.Size]float64{ir.SizeSmall: 0.333, ir.SizeMedium: 0.583, ir.SizeLarge: 1}
	cacheTiers   = map[string]string{"BASIC": "Basic", "STANDARD_HA": "Standard"}
)

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

func (costMatchers) Implicit(p *ir.Project, usage cost.Usage) []cost.Item {
	nat := callsPrivateService(p)
	if !nat && !needsNetwork(p) {
		return nil
	}
	return []cost.Item{networkCost(p.Region, nat, usage[cost.NetworkUsage].NatGb)}
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

	items := []cost.Item{networkCost(region, true, 0)}
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

// A shared-core tier is one hourly meter; a custom tier is its vCPUs and its RAM, each a meter
// of its own. Regional availability has its own set of meters at about twice the zonal rate.
func databaseCost(n ir.Node, region string) (cost.Item, error) {
	p, err := ir.NodeProps[ir.DatabaseProps](n)
	if err != nil {
		return cost.Item{}, fmt.Errorf("node '%s' has properties the gcp matcher cannot read: %v", n.ID, err)
	}
	engine, ok := sqlEngineNames[p.Engine]
	if !ok {
		return cost.Item{}, fmt.Errorf("database '%s' has an unknown engine '%s'", n.Name, p.Engine)
	}
	tier, ok := databaseTiers[p.Size]
	if !ok {
		return cost.Item{}, fmt.Errorf("database '%s' has an unknown size '%s'", n.Name, p.Size)
	}
	version := p.Version
	if version == "" {
		version, _ = ir.DefaultEngineVersion(ir.ProviderGCP, p.Engine)
	}
	availability := "Zonal"
	if p.HighAvailability {
		availability = "Regional"
	}
	meter := func(label, what string, quantity float64, unit string) cost.Lookup {
		return cost.Lookup{
			Label:    label,
			Service:  "Cloud SQL",
			Filters:  onDemand(region, described(regexp.QuoteMeta(engine+": "+availability+" - "+what)+` in .*`)),
			Quantity: quantity,
			Unit:     unit,
		}
	}
	item := cost.Item{
		Name:     n.Name,
		Kind:     string(n.Type),
		Summary:  fmt.Sprintf("%s, %s %s, %s, %d GB", tier, p.Engine, version, strings.ToLower(availability), p.StorageGB),
		Unpriced: []string{"backups"},
	}
	if shared, ok := sharedCoreTiers[tier]; ok {
		item.Lookups = append(item.Lookups, meter("instance", shared, hoursPerMonth, "h"))
	} else {
		parts := customTier.FindStringSubmatch(tier)
		if parts == nil {
			return cost.Item{}, fmt.Errorf("database '%s': the matcher cannot read the tier '%s'", n.Name, tier)
		}
		vcpu, _ := strconv.ParseFloat(parts[1], 64)
		mb, _ := strconv.ParseFloat(parts[2], 64)
		item.Lookups = append(item.Lookups,
			meter("vcpu", "vCPU", vcpu*hoursPerMonth, "vCPU-h"),
			meter("memory", "RAM", mb/1024*hoursPerMonth, "GB-h"),
		)
	}
	item.Lookups = append(item.Lookups, meter("storage ssd", "Standard storage", float64(p.StorageGB), "GB"))
	return item, nil
}

// The service sits at minReplicas, each instance billed by the second for its cpu and memory
// limits. The meters are priced by the second and shared across regions in two tiers, so the
// description allows for the tier and the region filter picks the one this region is in.
func serviceCost(n ir.Node, region string, u cost.NodeUsage) (cost.Item, error) {
	p, err := ir.NodeProps[ir.ServiceProps](n)
	if err != nil {
		return cost.Item{}, fmt.Errorf("node '%s' has properties the gcp matcher cannot read: %v", n.ID, err)
	}
	size, ok := serviceSizes[p.Size]
	if !ok {
		return cost.Item{}, fmt.Errorf("service '%s' has an unknown size '%s'", n.Name, p.Size)
	}
	requests, err := u.Requests.PerMonth()
	if err != nil {
		return cost.Item{}, fmt.Errorf("service '%s': requests %v", n.Name, err)
	}
	cpu, _ := strconv.ParseFloat(size.CPU, 64)
	memory := gibibytes(size.Memory)
	instances := float64(*p.MinReplicas)
	reach := "private"
	if p.Public {
		reach = "public"
	}
	return cost.Item{
		Name: n.Name,
		Kind: string(n.Type),
		Summary: fmt.Sprintf("%s vCPU, %s GB, %s, %s",
			number(cpu), number(memory), count(*p.MinReplicas, "instance"), reach),
		Lookups: []cost.Lookup{
			{
				Label:    "vcpu",
				Service:  "Cloud Run",
				Filters:  onDemand(region, described(`Services CPU( Tier 2)? \(Request-based billing\)`)),
				Quantity: cpu * instances * hoursPerMonth,
				Unit:     "vCPU-h",
				PerUnit:  secondsPerHour,
			},
			{
				Label:    "memory",
				Service:  "Cloud Run",
				Filters:  onDemand(region, described(`Services Memory( Tier 2)? \(Request-based billing\)`)),
				Quantity: memory * instances * hoursPerMonth,
				Unit:     "GB-h",
				PerUnit:  secondsPerHour,
			},
		},
		Usage: []cost.Lookup{{
			Label:    "requests",
			Service:  "Cloud Run",
			Filters:  onDemand(region, described(`Requests`)),
			Quantity: requests,
			Unit:     "requests",
		}},
		Unpriced: []string{"internet egress"},
	}, nil
}

func cacheCost(n ir.Node, region string) (cost.Item, error) {
	p, err := ir.NodeProps[ir.CacheProps](n)
	if err != nil {
		return cost.Item{}, fmt.Errorf("node '%s' has properties the gcp matcher cannot read: %v", n.ID, err)
	}
	size, ok := cacheSizes[p.Size]
	if !ok {
		return cost.Item{}, fmt.Errorf("cache '%s' has an unknown size '%s'", n.Name, p.Size)
	}
	return cost.Item{
		Name:    n.Name,
		Kind:    string(n.Type),
		Summary: fmt.Sprintf("%s, %d GB, Redis 7.2", size.Tier, size.MemoryGB),
		Lookups: []cost.Lookup{{
			Label:    "capacity",
			Service:  "Cloud Memorystore for Redis",
			Filters:  onDemand(region, described(`Redis Capacity `+cacheTiers[size.Tier]+` `+capacityTier(size.MemoryGB)+` .*`)),
			Quantity: float64(size.MemoryGB) * hoursPerMonth,
			Unit:     "GB-h",
		}},
	}, nil
}

// Memorystore prices a GB-hour by the capacity band the instance falls in.
func capacityTier(gb int) string {
	switch {
	case gb <= 4:
		return "M1"
	case gb <= 10:
		return "M2"
	case gb <= 35:
		return "M3"
	case gb <= 100:
		return "M4"
	}
	return "M5"
}

// One requests figure covers both operation classes, split a tenth writes, and the dual-region
// storage meters carry a slash in their name where the regional one does not.
func bucketCost(n ir.Node, region string, u cost.NodeUsage) (cost.Item, error) {
	p, err := ir.NodeProps[ir.BucketProps](n)
	if err != nil {
		return cost.Item{}, fmt.Errorf("node '%s' has properties the gcp matcher cannot read: %v", n.ID, err)
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
	meter := func(label, pattern string, quantity float64, unit string) cost.Lookup {
		return cost.Lookup{Label: label, Service: "Cloud Storage", Filters: onDemand(region, described(pattern)), Quantity: quantity, Unit: unit}
	}
	return cost.Item{
		Name:    n.Name,
		Kind:    string(n.Type),
		Summary: fmt.Sprintf("standard storage, %s, %s", versioning, access),
		Usage: []cost.Lookup{
			meter("storage", `Standard Storage [^/]*`, u.StorageGb, "GB"),
			meter("class A operations (1 in 10)", `Regional Standard Class A Operations`, math.Round(requests*writeShare), "requests"),
			meter("class B operations (9 in 10)", `Regional Standard Class B Operations`, math.Round(requests*(1-writeShare)), "requests"),
			meter("egress", `Download Worldwide Destinations \(excluding Asia & Australia\)`, u.EgressGb, "GB"),
		},
	}, nil
}

// A 2nd gen function is a Cloud Run service billed per request: invocations, then cpu and
// memory for every run's length at the size's memory and the vCPU that comes with it.
func functionCost(n ir.Node, region string, u cost.NodeUsage) (cost.Item, error) {
	p, err := ir.NodeProps[ir.FunctionProps](n)
	if err != nil {
		return cost.Item{}, fmt.Errorf("node '%s' has properties the gcp matcher cannot read: %v", n.ID, err)
	}
	memory, ok := functionMemory[p.Size]
	if !ok {
		return cost.Item{}, fmt.Errorf("function '%s' has an unknown size '%s'", n.Name, p.Size)
	}
	invocations, err := u.Invocations.PerMonth()
	if err != nil {
		return cost.Item{}, fmt.Errorf("function '%s': invocations %v", n.Name, err)
	}
	vcpu := functionVCPU[p.Size]
	seconds := invocations * u.DurationMs / 1000
	meter := func(label, pattern string, quantity float64, unit string) cost.Lookup {
		return cost.Lookup{Label: label, Service: "Cloud Run Functions", Filters: onDemand(region, described(pattern)), Quantity: quantity, Unit: unit}
	}
	return cost.Item{
		Name:    n.Name,
		Kind:    string(n.Type),
		Summary: fmt.Sprintf("%s, %d MB, %s vCPU", p.Runtime, int(gibibytes(memory)*1024), number(vcpu)),
		Usage: []cost.Lookup{
			meter("invocations", `Cloud Run Functions Invocations`, invocations, "invocations"),
			meter("cpu", `Cloud Run functions CPU \(Request-based billing\) in .*`, round(seconds*vcpu), "vCPU-s"),
			meter("memory", `Cloud Run functions Memory \(Request-based billing\) in .*`, round(seconds*gibibytes(memory)), "GB-s"),
		},
	}, nil
}

// A gateway creates nothing (ADR 0010): each request lands on the routed target's own meters.
func gatewayCost(n ir.Node) cost.Item {
	return cost.Item{
		Name:    n.Name,
		Kind:    string(n.Type),
		Summary: "no resources, requests are billed on the routed targets",
	}
}

// Pub/Sub prices bytes, a message counting as at least a kilobyte each time it is published
// and delivered, and the meter is per TiB, so the line is in GB at a thousandth of the rate.
func queueCost(n ir.Node, region string, u cost.NodeUsage) (cost.Item, error) {
	p, err := ir.NodeProps[ir.QueueProps](n)
	if err != nil {
		return cost.Item{}, fmt.Errorf("node '%s' has properties the gcp matcher cannot read: %v", n.ID, err)
	}
	shown := "standard"
	if p.FIFO {
		shown = "ordered"
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
			Label:    "throughput (1 KB in, 1 KB out a message)",
			Service:  "Cloud Pub/Sub",
			Filters:  onDemand(region, described(`Message Delivery Basic`)),
			Quantity: round(messages * bytesPerMessage / gibibyte),
			Unit:     "GB",
			PerUnit:  1.0 / tebibyte,
		}},
	}, nil
}

// The connector's two instances are billed as Compute Engine e2-micro VMs, and once a caller
// sends all its egress through the VPC the Cloud NAT charges an hour for each of them plus
// the data it processes.
func networkCost(region string, nat bool, natGb float64) cost.Item {
	instances := float64(connectorMinInstances)
	item := cost.Item{
		Name:    networkLabel,
		Kind:    "implicit VPC",
		Summary: fmt.Sprintf("connector, %s", count(connectorMinInstances, "e2-micro instance")),
		Lookups: []cost.Lookup{
			{
				Label:    "connector vcpu",
				Service:  "Compute Engine",
				Filters:  onDemand(region, described(`E2 Instance Core running in .*`)),
				Quantity: connectorVCPU * instances * hoursPerMonth,
				Unit:     "vCPU-h",
			},
			{
				Label:    "connector memory",
				Service:  "Compute Engine",
				Filters:  onDemand(region, described(`E2 Instance Ram running in .*`)),
				Quantity: connectorMemoryGB * instances * hoursPerMonth,
				Unit:     "GB-h",
			},
		},
	}
	if !nat {
		return item
	}
	item.Summary += ", Cloud NAT"
	item.Lookups = append(item.Lookups, cost.Lookup{
		Label:    fmt.Sprintf("nat gateway (%s)", count(connectorMinInstances, "instance")),
		Service:  "Networking",
		Filters:  onDemand(region, described(`Networking Cloud Nat Gateway Uptime`)),
		Quantity: instances * hoursPerMonth,
		Unit:     "VM-h",
	})
	item.Usage = []cost.Lookup{{
		Label:    "nat gateway data",
		Service:  "Networking",
		Filters:  onDemand(region, described(`Networking Cloud Nat Data Processing`)),
		Quantity: natGb,
		Unit:     "GB",
	}}
	return item
}

// The resolver creates the network for the first database or cache, and for the first caller
// of a private service, which is also what brings the NAT.
func needsNetwork(p *ir.Project) bool {
	for _, n := range p.Nodes {
		if n.Type == ir.NodeDatabase || n.Type == ir.NodeCache {
			return true
		}
	}
	return false
}

func callsPrivateService(p *ir.Project) bool {
	nodes := make(map[string]ir.Node, len(p.Nodes))
	for _, n := range p.Nodes {
		nodes[n.ID] = n
	}
	for _, e := range p.Edges {
		to := nodes[e.To]
		if e.Relation != ir.RelCalls || to.Type != ir.NodeService {
			continue
		}
		if props, err := ir.NodeProps[ir.ServiceProps](to); err == nil && !props.Public {
			return true
		}
	}
	return false
}

// A snapshot entry lists the regions its sku serves, space separated, or global for a meter
// with one price everywhere.
func inRegion(region string) cost.Filter {
	return cost.Filter{Attribute: "region", Value: `(.* )?(` + regexp.QuoteMeta(region) + `|global)( .*)?`, Pattern: true}
}

func described(pattern string) cost.Filter {
	return cost.Filter{Attribute: "description", Value: pattern, Pattern: true}
}

func onDemand(region string, filters ...cost.Filter) []cost.Filter {
	return append(filters, cost.Filter{Attribute: "usageType", Value: "OnDemand"}, inRegion(region))
}

func gibibytes(limit string) float64 {
	switch {
	case strings.HasSuffix(limit, "Gi"):
		n, _ := strconv.ParseFloat(strings.TrimSuffix(limit, "Gi"), 64)
		return n
	case strings.HasSuffix(limit, "Mi"):
		n, _ := strconv.ParseFloat(strings.TrimSuffix(limit, "Mi"), 64)
		return n / 1024
	}
	return 0
}

func round(v float64) float64 { return math.Round(v*1000) / 1000 }

func number(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
