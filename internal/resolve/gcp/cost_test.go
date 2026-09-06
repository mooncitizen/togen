package gcp

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
)

func sku(id, service, description, region, unit string, price float64) cost.SKU {
	return cost.SKU{ID: id, Service: service, Unit: unit, Price: price, Attributes: map[string]string{
		"description": description, "usageType": "OnDemand", "region": region,
	}}
}

// The skus a refresh would keep, taken from the catalog slices under internal/cost/cmd/testdata
// with their real ids and rates, plus the neighbours a loose filter would catch: the other
// engine and availability of Cloud SQL, the tier 1 Cloud Run meters that serve europe-west1,
// min-instance and instance-based Cloud Run, spot and committed cores, autoclass and nearline
// storage, the cluster and standard node Memorystore rows, and private NAT.
func priceFixture() *cost.Snapshot {
	sql := func(id, engine, availability, meter, unit string, price float64) cost.SKU {
		return sku(id, "Cloud SQL", "Cloud SQL for "+engine+": "+availability+" - "+meter+" in London", "europe-west2", unit, price)
	}
	return &cost.Snapshot{
		Provider: ir.ProviderGCP,
		Date:     "2026-09-05",
		Currency: "USD",
		Versions: map[string]map[string]string{
			"Cloud SQL":                   {"europe-west2": "2026-08-01", "europe-west1": "2026-08-01"},
			"Cloud Run":                   {"europe-west2": "2026-08-01", "europe-west1": "2026-08-01"},
			"Cloud Run Functions":         {"europe-west2": "2026-08-01"},
			"Cloud Storage":               {"europe-west2": "2026-08-01"},
			"Cloud Memorystore for Redis": {"europe-west2": "2026-08-01"},
			"Cloud Pub/Sub":               {"europe-west2": "2026-08-01"},
			"Compute Engine":              {"europe-west2": "2026-08-01"},
			"Networking":                  {"europe-west2": "2026-08-01"},
		},
		SKUs: []cost.SKU{
			sql("3E1A-85BB-7956", "PostgreSQL", "Zonal", "vCPU", "h", 0.0535),
			sql("A466-F7CB-DD13", "PostgreSQL", "Zonal", "RAM", "GiBy.h", 0.00905),
			sql("D3C0-6698-D7D2", "PostgreSQL", "Regional", "vCPU", "h", 0.107),
			sql("6F5E-33D5-3944", "PostgreSQL", "Regional", "RAM", "GiBy.h", 0.0181),
			sql("7A3E-C2B1-9F10", "PostgreSQL", "Zonal", "Micro instance", "h", 0.0126),
			sql("B2D4-8E7F-1C33", "PostgreSQL", "Regional", "Micro instance", "h", 0.0252),
			sql("A025-9006-CE7C", "PostgreSQL", "Zonal", "Standard storage", "GiBy.mo", 0.204),
			sql("3782-B08A-12E4", "PostgreSQL", "Regional", "Standard storage", "GiBy.mo", 0.408),
			sql("5F0C-2B9A-77E1", "MySQL", "Zonal", "vCPU", "h", 0.0535),
			sql("9D31-6C0E-4AB2", "MySQL", "Zonal", "RAM", "GiBy.h", 0.00905),
			sql("E1B7-40D2-3C6F", "MySQL", "Zonal", "Micro instance", "h", 0.0126),
			sql("8AA6-0F3F-991C", "MySQL", "Zonal", "Standard storage", "GiBy.mo", 0.204),
			sql("84E2-CA20-196B", "MySQL", "Regional", "vCPU", "h", 0.107),
			sql("CF85-986A-75A9", "MySQL", "Regional", "RAM", "GiBy.h", 0.0181),
			sql("0A9E-7D45-B21C", "MySQL", "Regional", "Micro instance", "h", 0.0252),
			sql("B3E0-7C8F-34BF", "MySQL", "Regional", "Standard storage", "GiBy.mo", 0.408),
			sql("6E2A-C40B-5D18", "PostgreSQL", "Zonal", "Enterprise Plus vCPU", "h", 0.0722),
			sql("7EE2-1B01-9DF5", "PostgreSQL", "Zonal", "Low cost storage", "GiBy.mo", 0.09),

			sku("085C-A237-027A", "Cloud Run", "Services CPU Tier 2 (Request-based billing)", "europe-west2 europe-west3", "s", 0.0000336),
			sku("600C-3782-6708", "Cloud Run", "Services Memory Tier 2 (Request-based billing)", "europe-west2 europe-west3", "GiBy.s", 0.0000035),
			sku("4856-B847-F1EB", "Cloud Run", "Services CPU (Request-based billing)", "europe-west1 us-central1", "s", 0.000024),
			sku("02A2-9231-36A6", "Cloud Run", "Services Memory (Request-based billing)", "europe-west1 us-central1", "GiBy.s", 0.0000025),
			sku("2DA5-55D3-E679", "Cloud Run", "Requests", "global", "count", 0.0000004),
			sku("4350-71E4-3051", "Cloud Run", "Services Min Instance CPU Tier 2 (Request-based billing)", "europe-west2", "s", 0.0000035),
			sku("3DFE-FD5E-A6D0", "Cloud Run", "Services CPU (Instance-based billing) in europe-west2", "europe-west2", "s", 0.0000252),

			sku("92DF-0F0E-630F", "Cloud Run Functions", "Cloud Run Functions Invocations", "global", "count", 0.0000004),
			sku("CCB6-0B74-2074", "Cloud Run Functions", "Cloud Run functions CPU (Request-based billing) in europe-west2", "europe-west2", "s", 0.0000336),
			sku("89AF-3B9D-D104", "Cloud Run Functions", "Cloud Run functions Memory (Request-based billing) in europe-west2", "europe-west2", "GiBy.s", 0.0000035),
			sku("2A97-8DD1-3C31", "Cloud Run Functions", "Cloud Run functions Min-Instance CPU (Request-based billing) in europe-west2", "europe-west2", "s", 0.0000035),

			sku("BB55-3E5A-405C", "Cloud Storage", "Standard Storage London", "europe-west2", "GiBy.mo", 0.023),
			sku("CBAE-5A46-5152", "Cloud Storage", "Standard Storage Belgium/London Dual-region", "europe-west1 europe-west2", "GiBy.mo", 0.0276),
			sku("01A8-57BF-A535", "Cloud Storage", "Autoclass Standard Storage London", "europe-west2", "GiBy.mo", 0.023),
			sku("3C81-A91C-A94F", "Cloud Storage", "Nearline Storage London", "europe-west2", "GiBy.mo", 0.013),
			sku("4DBF-185F-A415", "Cloud Storage", "Regional Standard Class A Operations", "global", "count", 0.000005),
			sku("7870-010B-2763", "Cloud Storage", "Regional Standard Class B Operations", "global", "count", 0.0000004),
			egressSKU(),

			sku("8FF9-D19D-3185", "Cloud Memorystore for Redis", "Redis Capacity Basic M1 London", "europe-west2", "GiBy.h", 0.0592),
			sku("583A-B0BE-7BD2", "Cloud Memorystore for Redis", "Redis Capacity Basic M2 London", "europe-west2", "GiBy.h", 0.0396),
			sku("2599-6A81-3CB1", "Cloud Memorystore for Redis", "Redis Capacity Standard M1 London", "europe-west2", "GiBy.h", 0.0985),
			sku("D706-6EB9-B22E", "Cloud Memorystore for Redis", "Redis Capacity Standard M2 London", "europe-west2", "GiBy.h", 0.0888),
			sku("6B4E-CF8F-A645", "Cloud Memorystore for Redis", "Redis Standard Node Capacity M2 London", "europe-west2", "GiBy.h", 0.0444),
			sku("04EF-FE6B-7ADC", "Cloud Memorystore for Redis", "Redis Cluster Node Default London", "europe-west2", "h", 0.0688),

			sku("027D-B6C7-CCA2", "Cloud Pub/Sub", "Message Delivery Basic", "global", "TiBy", 40),
			sku("3EAB-48F3-A0D5", "Cloud Pub/Sub", "Subscriptions message backlog", "global", "GiBy.mo", 0.27),

			sku("C6A7-8E3D-2F51", "Compute Engine", "E2 Instance Core running in London", "europe-west2", "h", 0.028092),
			sku("4B19-D0F6-A83C", "Compute Engine", "E2 Instance Ram running in London", "europe-west2", "GiBy.h", 0.003765),
			sku("E3D8-1B67-0C5A", "Compute Engine", "E2 Custom Instance Core running in London", "europe-west2", "h", 0.030201),
			sku("2D6B-7F13-C8E4", "Compute Engine", "E2 Instance Core running in Belgium", "europe-west1", "h", 0.021811),
			sku("F04C-3E9A-6D12", "Compute Engine", "E2 Instance Ram running in Belgium", "europe-west1", "GiBy.h", 0.002923),

			sku("32E2-4EFC-EF9F", "Networking", "Networking Cloud Nat Gateway Uptime", "global", "h", 0.044),
			sku("015F-5732-FFF0", "Networking", "Networking Cloud Nat Data Processing", "global", "GiBy", 0.045),
			sku("AAF7-1D17-BFC4", "Networking", "Networking Private Nat Gateway Uptime", "global", "h", 0.044),
		},
	}
}

// The rate a per-second meter reads at over an hour, as the estimate works it out.
func perHour(price float64) float64 { return price * secondsPerHour }

func egressSKU() cost.SKU {
	s := sku("22EB-AAE8-FBCD", "Cloud Storage", "Download Worldwide Destinations (excluding Asia & Australia)", "global", "GiBy", 0.12)
	s.UpTo = 1024
	return s
}

func estimate(t *testing.T, region string, nodes []ir.Node) cost.Document {
	t.Helper()
	return estimateUsage(t, region, nodes, nil, nil)
}

func estimateUsage(t *testing.T, region string, nodes []ir.Node, edges []ir.Edge, usage cost.Usage) cost.Document {
	t.Helper()
	p := newProject(t, nodes, edges)
	p.Region = region
	doc, err := cost.Estimate(p, Cost(), priceFixture(), usage, time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func item(t *testing.T, doc cost.Document, name string) cost.Priced {
	t.Helper()
	for _, i := range doc.Items {
		if i.Name == name {
			return i
		}
	}
	t.Fatalf("no item named %s in %+v", name, doc.Items)
	return cost.Priced{}
}

// A shared-core tier is one hourly meter, and the connector comes with the first database.
func TestDatabaseCostPricesASharedCoreTierAsOneHourlyMeter(t *testing.T) {
	doc := estimate(t, "europe-west2", []ir.Node{databaseNode(t, "n1", "main-db", ir.DatabaseProps{})})
	want := cost.Priced{
		Name: "main-db", Kind: "database", Summary: "db-f1-micro, postgres 17, zonal, 20 GB",
		Lines: []cost.Line{
			{Label: "instance", Quantity: 730, Unit: "h", UnitPrice: 0.0126, Amount: 9.2, SKU: "7A3E-C2B1-9F10"},
			{Label: "storage ssd", Quantity: 20, Unit: "GB", UnitPrice: 0.204, Amount: 4.08, SKU: "A025-9006-CE7C"},
		},
		Subtotal: 13.28,
	}
	if diff := cmp.Diff(want, item(t, doc, "main-db")); diff != "" {
		t.Errorf("database (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]cost.Omission{{Name: "main-db", Kind: "database", Reason: "backups"}}, doc.NotPriced); diff != "" {
		t.Errorf("not priced (-want +got):\n%s", diff)
	}
}

// A custom tier is its vCPUs and its RAM, each a meter of its own.
func TestDatabaseCostPricesACustomTierAsVCPUsAndRAM(t *testing.T) {
	doc := estimate(t, "europe-west2", []ir.Node{databaseNode(t, "n1", "main-db", ir.DatabaseProps{Size: ir.SizeMedium})})
	want := cost.Priced{
		Name: "main-db", Kind: "database", Summary: "db-custom-2-7680, postgres 17, zonal, 20 GB",
		Lines: []cost.Line{
			{Label: "vcpu", Quantity: 1460, Unit: "vCPU-h", UnitPrice: 0.0535, Amount: 78.11, SKU: "3E1A-85BB-7956"},
			{Label: "memory", Quantity: 5475, Unit: "GB-h", UnitPrice: 0.00905, Amount: 49.55, SKU: "A466-F7CB-DD13"},
			{Label: "storage ssd", Quantity: 20, Unit: "GB", UnitPrice: 0.204, Amount: 4.08, SKU: "A025-9006-CE7C"},
		},
		Subtotal: 131.74,
	}
	if diff := cmp.Diff(want, item(t, doc, "main-db")); diff != "" {
		t.Errorf("database (-want +got):\n%s", diff)
	}
}

// Regional availability is its own set of meters at about twice the zonal rate.
func TestDatabaseCostDoublesForRegionalAvailability(t *testing.T) {
	doc := estimate(t, "europe-west2", []ir.Node{databaseNode(t, "n1", "main-db", ir.DatabaseProps{HighAvailability: true})})
	db := item(t, doc, "main-db")
	if db.Summary != "db-f1-micro, postgres 17, regional, 20 GB" {
		t.Errorf("summary = %q", db.Summary)
	}
	want := []cost.Line{
		{Label: "instance", Quantity: 730, Unit: "h", UnitPrice: 0.0252, Amount: 18.4, SKU: "B2D4-8E7F-1C33"},
		{Label: "storage ssd", Quantity: 20, Unit: "GB", UnitPrice: 0.408, Amount: 8.16, SKU: "3782-B08A-12E4"},
	}
	if diff := cmp.Diff(want, db.Lines); diff != "" {
		t.Errorf("lines (-want +got):\n%s", diff)
	}
}

func TestDatabaseCostFollowsTheEngineAndTheStorage(t *testing.T) {
	doc := estimate(t, "europe-west2", []ir.Node{databaseNode(t, "n1", "main-db", ir.DatabaseProps{Engine: ir.EngineMySQL, Version: "8.0", StorageGB: 100})})
	db := item(t, doc, "main-db")
	if db.Summary != "db-f1-micro, mysql 8.0, zonal, 100 GB" {
		t.Errorf("summary = %q", db.Summary)
	}
	if db.Lines[0].SKU != "E1B7-40D2-3C6F" {
		t.Errorf("instance = %+v", db.Lines[0])
	}
	if got := db.Lines[1]; got.SKU != "8AA6-0F3F-991C" || got.Quantity != 100 || got.Amount != 20.4 {
		t.Errorf("storage = %+v", got)
	}
}

func TestDatabaseCostNamesEveryTierItPrices(t *testing.T) {
	for _, c := range []struct {
		size  ir.Size
		want  string
		lines int
	}{
		{ir.SizeSmall, "db-f1-micro", 2},
		{ir.SizeMedium, "db-custom-2-7680", 3},
		{ir.SizeLarge, "db-custom-4-15360", 3},
	} {
		doc := estimate(t, "europe-west2", []ir.Node{databaseNode(t, "n1", "main-db", ir.DatabaseProps{Size: c.size})})
		db := item(t, doc, "main-db")
		if len(db.Lines) != c.lines {
			t.Errorf("%s: lines = %+v, want %d", c.size, db.Lines, c.lines)
		}
		if db.Summary[:len(c.want)] != c.want {
			t.Errorf("%s: summary = %q", c.size, db.Summary)
		}
	}
}

// Cloud Run prices cpu and memory by the second, so a lookup at minReplicas over 730 hours
// converts the rate for a line that reads like the AWS ones.
func TestServiceCostPricesTheInstancesItHoldsAtMinReplicas(t *testing.T) {
	doc := estimate(t, "europe-west2", []ir.Node{serviceNode(t, "n2", "web", ir.ServiceProps{Public: true})})
	want := cost.Priced{
		Name: "web", Kind: "service", Summary: "1 vCPU, 0.5 GB, 1 instance, public",
		Note: "priced on defaults, no usage set",
		Lines: []cost.Line{
			{Label: "vcpu", Quantity: 730, Unit: "vCPU-h", UnitPrice: perHour(0.0000336), Amount: 88.3, SKU: "085C-A237-027A"},
			{Label: "memory", Quantity: 365, Unit: "GB-h", UnitPrice: perHour(0.0000035), Amount: 4.6, SKU: "600C-3782-6708"},
		},
		Subtotal: 92.9,
	}
	if diff := cmp.Diff(want, item(t, doc, "web")); diff != "" {
		t.Errorf("service (-want +got):\n%s", diff)
	}
	wantOmitted := []cost.Omission{
		{Name: "web", Kind: "service", Reason: "requests"},
		{Name: "web", Kind: "service", Reason: "internet egress"},
	}
	if diff := cmp.Diff(wantOmitted, doc.NotPriced); diff != "" {
		t.Errorf("not priced (-want +got):\n%s", diff)
	}
}

func TestServiceCostScalesWithTheSizeAndTheReplicas(t *testing.T) {
	doc := estimate(t, "europe-west2", []ir.Node{serviceNode(t, "n2", "web", ir.ServiceProps{Size: ir.SizeLarge, MinReplicas: ir.Ptr(3), Public: true})})
	web := item(t, doc, "web")
	if web.Summary != "2 vCPU, 2 GB, 3 instances, public" {
		t.Errorf("summary = %q", web.Summary)
	}
	if got := web.Lines[0]; got.Quantity != 4380 || got.Amount != 529.8 {
		t.Errorf("vcpu = %+v", got)
	}
	if got := web.Lines[1]; got.Quantity != 4380 || got.Amount != 55.19 {
		t.Errorf("memory = %+v", got)
	}
}

// A service scaled to zero at rest holds no instances, so it costs nothing until it serves.
func TestServiceCostIsNothingAtMinReplicasZero(t *testing.T) {
	doc := estimate(t, "europe-west2", []ir.Node{serviceNode(t, "n2", "web", ir.ServiceProps{MinReplicas: ir.Ptr(0), Public: true})})
	web := item(t, doc, "web")
	if web.Subtotal != 0 || web.Lines[0].Quantity != 0 || web.Lines[1].Quantity != 0 {
		t.Errorf("service = %+v", web)
	}
}

// The tier 2 meters serve europe-west2, the tier 1 ones europe-west1, and requests are global.
func TestServiceCostFollowsTheRegionToTheRightPricingTier(t *testing.T) {
	doc := estimateUsage(t, "europe-west1", []ir.Node{serviceNode(t, "n2", "web", ir.ServiceProps{Public: true})}, nil,
		cost.Usage{"web": {Requests: "600/min"}})
	web := item(t, doc, "web")
	want := []cost.Line{
		{Label: "vcpu", Quantity: 730, Unit: "vCPU-h", UnitPrice: perHour(0.000024), Amount: 63.07, SKU: "4856-B847-F1EB"},
		{Label: "memory", Quantity: 365, Unit: "GB-h", UnitPrice: perHour(0.0000025), Amount: 3.29, SKU: "02A2-9231-36A6"},
		{Label: "requests", Quantity: 26280000, Unit: "requests", UnitPrice: 0.0000004, Amount: 10.51, SKU: "2DA5-55D3-E679"},
	}
	if diff := cmp.Diff(want, web.Lines); diff != "" {
		t.Errorf("lines (-want +got):\n%s", diff)
	}
}

// Memorystore prices a GB-hour by the tier and the capacity band the instance falls in.
func TestCacheCostPricesTheCapacityOfItsTier(t *testing.T) {
	doc := estimate(t, "europe-west2", []ir.Node{cacheNode(t, "n3", "sessions", ir.CacheProps{})})
	want := cost.Priced{
		Name: "sessions", Kind: "cache", Summary: "BASIC, 1 GB, Redis 7.2",
		Lines:    []cost.Line{{Label: "capacity", Quantity: 730, Unit: "GB-h", UnitPrice: 0.0592, Amount: 43.22, SKU: "8FF9-D19D-3185"}},
		Subtotal: 43.22,
	}
	if diff := cmp.Diff(want, item(t, doc, "sessions")); diff != "" {
		t.Errorf("cache (-want +got):\n%s", diff)
	}
}

func TestCacheCostFollowsTheTierAndTheBandOfEverySize(t *testing.T) {
	for _, c := range []struct {
		size    ir.Size
		summary string
		sku     string
		amount  float64
	}{
		{ir.SizeSmall, "BASIC, 1 GB, Redis 7.2", "8FF9-D19D-3185", 43.22},
		{ir.SizeMedium, "BASIC, 2 GB, Redis 7.2", "8FF9-D19D-3185", 86.43},
		{ir.SizeLarge, "STANDARD_HA, 5 GB, Redis 7.2", "D706-6EB9-B22E", 324.12},
	} {
		doc := estimate(t, "europe-west2", []ir.Node{cacheNode(t, "n3", "sessions", ir.CacheProps{Size: c.size})})
		got := item(t, doc, "sessions")
		if got.Summary != c.summary || got.Lines[0].SKU != c.sku || got.Lines[0].Amount != c.amount {
			t.Errorf("%s: cache = %+v", c.size, got)
		}
	}
}

// A function costs nothing at rest: every meter is usage.
func TestFunctionCostIsNothingWithoutUsage(t *testing.T) {
	doc := estimate(t, "europe-west2", []ir.Node{functionNode(t, "n4", "handler", ir.FunctionProps{})})
	want := cost.Priced{
		Name: "handler", Kind: "function", Summary: "node, 512 MB, 0.333 vCPU",
		Note: "priced on defaults, no usage set", Lines: []cost.Line{},
	}
	if diff := cmp.Diff(want, item(t, doc, "handler")); diff != "" {
		t.Errorf("function (-want +got):\n%s", diff)
	}
	if doc.Total != 0 {
		t.Errorf("total = %v", doc.Total)
	}
}

// Invocations and the GB-seconds and vCPU-seconds of their runs, priced per second.
func TestFunctionCostPricesInvocationsAndTheirRuns(t *testing.T) {
	doc := estimateUsage(t, "europe-west2", []ir.Node{functionNode(t, "n4", "handler", ir.FunctionProps{})}, nil,
		cost.Usage{"handler": {Invocations: "1000000/month", DurationMs: 200}})
	want := []cost.Line{
		{Label: "invocations", Quantity: 1000000, Unit: "invocations", UnitPrice: 0.0000004, Amount: 0.4, SKU: "92DF-0F0E-630F"},
		{Label: "cpu", Quantity: 66600, Unit: "vCPU-s", UnitPrice: 0.0000336, Amount: 2.24, SKU: "CCB6-0B74-2074"},
		{Label: "memory", Quantity: 100000, Unit: "GB-s", UnitPrice: 0.0000035, Amount: 0.35, SKU: "89AF-3B9D-D104"},
	}
	if diff := cmp.Diff(want, item(t, doc, "handler").Lines); diff != "" {
		t.Errorf("lines (-want +got):\n%s", diff)
	}
}

func TestFunctionCostNamesTheMemoryAndVCPUOfEverySize(t *testing.T) {
	for _, c := range []struct {
		size ir.Size
		want string
	}{
		{ir.SizeSmall, "node, 512 MB, 0.333 vCPU"},
		{ir.SizeMedium, "node, 1024 MB, 0.583 vCPU"},
		{ir.SizeLarge, "node, 2048 MB, 1 vCPU"},
	} {
		doc := estimate(t, "europe-west2", []ir.Node{functionNode(t, "n4", "handler", ir.FunctionProps{Size: c.size})})
		if got := item(t, doc, "handler").Summary; got != c.want {
			t.Errorf("%s: summary = %q, want %q", c.size, got, c.want)
		}
	}
}

// A gateway creates nothing, so it has no meters at all and nothing to omit.
func TestGatewayCostHasNothingToPrice(t *testing.T) {
	doc := estimate(t, "europe-west2", []ir.Node{ir.Node{ID: "n5", Type: ir.NodeGateway, Name: "api", Properties: props(t, ir.GatewayProps{})}})
	want := cost.Priced{
		Name: "api", Kind: "gateway", Summary: "no resources, requests are billed on the routed targets",
		Lines: []cost.Line{},
	}
	if diff := cmp.Diff(want, item(t, doc, "api")); diff != "" {
		t.Errorf("gateway (-want +got):\n%s", diff)
	}
	if len(doc.NotPriced) != 0 || doc.Total != 0 {
		t.Errorf("not priced = %+v, total = %v", doc.NotPriced, doc.Total)
	}
}

// Pub/Sub prices a TiB, a message counting as a kilobyte published and a kilobyte delivered,
// so the line reads in GB at a thousandth of the rate.
func TestQueueCostPricesMessagesAsThroughput(t *testing.T) {
	doc := estimateUsage(t, "europe-west2", []ir.Node{queueNode(t, "n6", "jobs", ir.QueueProps{})}, nil,
		cost.Usage{"jobs": {Messages: "1000000/month"}})
	want := []cost.Line{{
		Label: "throughput (1 KB in, 1 KB out a message)", Quantity: 1.907, Unit: "GB",
		UnitPrice: 0.0390625, Amount: 0.07, SKU: "027D-B6C7-CCA2",
	}}
	if diff := cmp.Diff(want, item(t, doc, "jobs").Lines); diff != "" {
		t.Errorf("lines (-want +got):\n%s", diff)
	}
}

func TestQueueCostIsNothingWithoutMessages(t *testing.T) {
	doc := estimate(t, "europe-west2", []ir.Node{queueNode(t, "n6", "jobs", ir.QueueProps{FIFO: true})})
	jobs := item(t, doc, "jobs")
	if jobs.Summary != "ordered" || jobs.Subtotal != 0 {
		t.Errorf("queue = %+v", jobs)
	}
	if diff := cmp.Diff([]cost.Omission{{Name: "jobs", Kind: "queue", Reason: "throughput (1 KB in, 1 KB out a message)"}}, doc.NotPriced); diff != "" {
		t.Errorf("not priced (-want +got):\n%s", diff)
	}
}

// One requests figure covers both operation classes, split a tenth writes.
func TestBucketCostPricesStorageOperationsAndEgress(t *testing.T) {
	doc := estimateUsage(t, "europe-west2", []ir.Node{bucketNode("n7", "uploads", "{}")}, nil,
		cost.Usage{"uploads": {StorageGb: 500, EgressGb: 100, Requests: "1000000/month"}})
	want := []cost.Line{
		{Label: "storage", Quantity: 500, Unit: "GB", UnitPrice: 0.023, Amount: 11.5, SKU: "BB55-3E5A-405C"},
		{Label: "class A operations (1 in 10)", Quantity: 100000, Unit: "requests", UnitPrice: 0.000005, Amount: 0.5, SKU: "4DBF-185F-A415"},
		{Label: "class B operations (9 in 10)", Quantity: 900000, Unit: "requests", UnitPrice: 0.0000004, Amount: 0.36, SKU: "7870-010B-2763"},
		{Label: "egress", Quantity: 100, Unit: "GB", UnitPrice: 0.12, Amount: 12, SKU: "22EB-AAE8-FBCD"},
	}
	if diff := cmp.Diff(want, item(t, doc, "uploads").Lines); diff != "" {
		t.Errorf("lines (-want +got):\n%s", diff)
	}
	if got := item(t, doc, "uploads").Summary; got != "standard storage, versioned, private" {
		t.Errorf("summary = %q", got)
	}
}

// Egress past the first tier the snapshot holds says so on the line.
func TestBucketCostNotesEgressPastTheFirstTier(t *testing.T) {
	doc := estimateUsage(t, "europe-west2", []ir.Node{bucketNode("n7", "uploads", "{}")}, nil,
		cost.Usage{"uploads": {EgressGb: 2000}})
	egress := item(t, doc, "uploads").Lines[0]
	if egress.Note == "" || egress.Amount != 240 {
		t.Errorf("egress = %+v", egress)
	}
}

// The connector is two e2-micro instances, billed as Compute Engine VMs.
func TestNetworkCostComesWithTheFirstNodeThatNeedsIt(t *testing.T) {
	doc := estimate(t, "europe-west2", []ir.Node{functionNode(t, "n4", "handler", ir.FunctionProps{})})
	if len(doc.Items) != 1 {
		t.Errorf("a function alone brought %+v", doc.Items)
	}

	doc = estimate(t, "europe-west2", []ir.Node{cacheNode(t, "n3", "sessions", ir.CacheProps{})})
	want := cost.Priced{
		Name: networkLabel, Kind: "implicit VPC", Summary: "connector, 2 e2-micro instances",
		Lines: []cost.Line{
			{Label: "connector vcpu", Quantity: 365, Unit: "vCPU-h", UnitPrice: 0.028092, Amount: 10.25, SKU: "C6A7-8E3D-2F51"},
			{Label: "connector memory", Quantity: 1460, Unit: "GB-h", UnitPrice: 0.003765, Amount: 5.5, SKU: "4B19-D0F6-A83C"},
		},
		Subtotal: 15.75,
	}
	if diff := cmp.Diff(want, item(t, doc, networkLabel)); diff != "" {
		t.Errorf("network (-want +got):\n%s", diff)
	}
}

// A caller of a private service sends its egress through the VPC, which brings the NAT: an
// hour for each connector instance, and the data it processes as usage.
func TestNetworkCostAddsTheNatWhenACallerNeedsAllTrafficEgress(t *testing.T) {
	nodes := []ir.Node{
		serviceNode(t, "n2", "web", ir.ServiceProps{Public: false}),
		serviceNode(t, "n8", "caller", ir.ServiceProps{Public: true}),
	}
	edges := []ir.Edge{{From: "n8", To: "n2", Relation: ir.RelCalls}}
	doc := estimateUsage(t, "europe-west2", nodes, edges, cost.Usage{cost.NetworkUsage: {NatGb: 50}})
	network := item(t, doc, networkLabel)
	if network.Summary != "connector, 2 e2-micro instances, Cloud NAT" {
		t.Errorf("summary = %q", network.Summary)
	}
	want := []cost.Line{
		{Label: "connector vcpu", Quantity: 365, Unit: "vCPU-h", UnitPrice: 0.028092, Amount: 10.25, SKU: "C6A7-8E3D-2F51"},
		{Label: "connector memory", Quantity: 1460, Unit: "GB-h", UnitPrice: 0.003765, Amount: 5.5, SKU: "4B19-D0F6-A83C"},
		{Label: "nat gateway (2 instances)", Quantity: 1460, Unit: "VM-h", UnitPrice: 0.044, Amount: 64.24, SKU: "32E2-4EFC-EF9F"},
		{Label: "nat gateway data", Quantity: 50, Unit: "GB", UnitPrice: 0.045, Amount: 2.25, SKU: "015F-5732-FFF0"},
	}
	if diff := cmp.Diff(want, network.Lines); diff != "" {
		t.Errorf("lines (-want +got):\n%s", diff)
	}
}

// Every shape the matchers tell apart resolves to a sku in the fixture, which is what the
// refresh checks against the live catalog.
func TestCatalogueNamesOnlyMetersTheSnapshotHolds(t *testing.T) {
	lookups, err := Cost().Catalogue("europe-west2")
	if err != nil {
		t.Fatal(err)
	}
	if len(lookups) == 0 {
		t.Fatal("the catalogue names no meters")
	}
	snapshot := priceFixture()
	for _, l := range lookups {
		if _, err := snapshot.Find(l); err != nil {
			t.Errorf("%s (%s): %v", l.Label, l.Describe(), err)
		}
	}
}
