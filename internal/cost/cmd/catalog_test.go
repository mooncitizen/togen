package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/resolve/gcp"
)

func loadSlice(t *testing.T, name string) []catalogSKU {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "gcp-"+name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var page catalogPageResponse
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	return page.SKUs
}

// trimSlice fails if any lookup for the region matches no sku or several, so a caller is left
// to check only which skus the filter set kept.
func trimSlice(t *testing.T, service, file, region string) ([]cost.SKU, map[string]string) {
	t.Helper()
	lookups, err := catalogLookups()
	if err != nil {
		t.Fatal(err)
	}
	wanted := lookups[service][region]
	if len(wanted) == 0 {
		t.Fatalf("no %s lookups for %s", service, region)
	}
	kept, versions, misses, err := trimCatalog(service, loadSlice(t, file), map[string][]cost.Lookup{region: wanted})
	if err != nil {
		t.Fatal(err)
	}
	if len(misses) > 0 {
		t.Fatalf("%s in %s:\n  %s", service, region, strings.Join(misses, "\n  "))
	}
	return kept, versions
}

func keptIDs(skus []cost.SKU) []string {
	ids := make([]string, len(skus))
	for i, sku := range skus {
		ids[i] = sku.ID
	}
	slices.Sort(ids)
	return ids
}

// The slices under testdata are cut from the Cloud Billing Catalog: for each service the skus
// the matchers should pick in europe-west2, and the neighbours a loose filter would catch.
// Cloud SQL has extended support, Enterprise Plus and N4 vCPUs, low cost storage, a fixed
// 1 vCPU + 3.75GB tier, a committed term, a data transfer meter and a Belgium row; Cloud Run
// has the min-instance, instance-based, jobs and instances meters and the tier 1 rows that
// serve other regions; Cloud Run Functions has the min-instance and 1st gen meters; Cloud
// Storage has autoclass, HNS, nearline, the dual-region name and the APAC egress meter;
// Compute Engine has spot, custom, N1 and committed cores and an egress meter; Memorystore has
// the cluster and standard node rows; Networking has private NAT and NAT IP uptime.
func TestTrimCatalogKeepsOnlyTheSkusTheLookupsPick(t *testing.T) {
	for _, c := range []struct {
		service string
		file    string
		want    []string
	}{
		{"Cloud SQL", "cloud-sql", []string{
			"0A9E-7D45-B21C", "3782-B08A-12E4", "3E1A-85BB-7956", "5F0C-2B9A-77E1",
			"6F5E-33D5-3944", "7A3E-C2B1-9F10", "84E2-CA20-196B", "8AA6-0F3F-991C",
			"9D31-6C0E-4AB2", "A025-9006-CE7C", "A466-F7CB-DD13", "B2D4-8E7F-1C33",
			"B3E0-7C8F-34BF", "CF85-986A-75A9", "D3C0-6698-D7D2", "E1B7-40D2-3C6F",
		}},
		{"Cloud Run", "cloud-run", []string{"085C-A237-027A", "2DA5-55D3-E679", "600C-3782-6708"}},
		{"Cloud Run Functions", "cloud-run-functions", []string{"89AF-3B9D-D104", "92DF-0F0E-630F", "CCB6-0B74-2074"}},
		{"Cloud Storage", "cloud-storage", []string{"22EB-AAE8-FBCD", "4DBF-185F-A415", "7870-010B-2763", "BB55-3E5A-405C"}},
		{"Compute Engine", "compute-engine", []string{"4B19-D0F6-A83C", "C6A7-8E3D-2F51"}},
		{"Networking", "networking", []string{"015F-5732-FFF0", "32E2-4EFC-EF9F"}},
		{"Cloud Pub/Sub", "cloud-pub-sub", []string{"027D-B6C7-CCA2"}},
		{"Cloud Memorystore for Redis", "cloud-memorystore-for-redis", []string{"8FF9-D19D-3185", "D706-6EB9-B22E"}},
	} {
		t.Run(c.service, func(t *testing.T) {
			kept, versions := trimSlice(t, c.service, c.file, "europe-west2")
			if diff := cmp.Diff(c.want, keptIDs(kept)); diff != "" {
				t.Errorf("skus (-want +got):\n%s", diff)
			}
			if versions["europe-west2"] != "2026-08-01" {
				t.Errorf("version = %q, want the newest effective date of the skus kept", versions["europe-west2"])
			}
		})
	}
}

// Cloud SQL prices a shared-core tier as one hourly meter and a custom tier as its vCPUs and
// its RAM, each set again at about twice the rate for regional availability.
func TestTrimCatalogTellsTheCloudSQLMetersApart(t *testing.T) {
	kept, _ := trimSlice(t, "Cloud SQL", "cloud-sql", "europe-west2")
	rates := map[string]float64{}
	for _, sku := range kept {
		rates[sku.Attributes["description"]] = sku.Price
	}
	for _, c := range []struct{ zonal, regional string }{
		{"Cloud SQL for PostgreSQL: Zonal - vCPU in London", "Cloud SQL for PostgreSQL: Regional - vCPU in London"},
		{"Cloud SQL for PostgreSQL: Zonal - RAM in London", "Cloud SQL for PostgreSQL: Regional - RAM in London"},
		{"Cloud SQL for MySQL: Zonal - Micro instance in London", "Cloud SQL for MySQL: Regional - Micro instance in London"},
		{"Cloud SQL for MySQL: Zonal - Standard storage in London", "Cloud SQL for MySQL: Regional - Standard storage in London"},
	} {
		if rates[c.zonal] == 0 || rates[c.regional] <= rates[c.zonal] {
			t.Errorf("%s at %v, %s at %v", c.zonal, rates[c.zonal], c.regional, rates[c.regional])
		}
	}
}

// A global meter is kept once under the global region so every region's lookup finds it; a
// regional one lists the regions it serves among the ones asked for.
func TestTrimCatalogRecordsTheRegionsASkuServes(t *testing.T) {
	kept, _ := trimSlice(t, "Cloud Run", "cloud-run", "europe-west2")
	got := map[string]string{}
	for _, sku := range kept {
		got[sku.ID] = sku.Attributes["region"]
	}
	want := map[string]string{
		"2DA5-55D3-E679": "global",
		"085C-A237-027A": "europe-west2",
		"600C-3782-6708": "europe-west2",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("regions (-want +got):\n%s", diff)
	}
}

// A meter with a free allowance opens with a tier at nothing. The estimate leaves free tiers
// out, so the rate kept is the first that charges.
func TestTrimCatalogSkipsAFreeOpeningTier(t *testing.T) {
	kept, _ := trimSlice(t, "Cloud Pub/Sub", "cloud-pub-sub", "europe-west2")
	want := cost.SKU{
		ID:      "027D-B6C7-CCA2",
		Service: "Cloud Pub/Sub",
		Unit:    "TiBy",
		Price:   40,
		Attributes: map[string]string{
			"resourceFamily": "ApplicationServices", "resourceGroup": "MessageDelivery",
			"usageType": "OnDemand", "description": "Message Delivery Basic", "region": "global",
		},
	}
	if diff := cmp.Diff([]cost.SKU{want}, kept); diff != "" {
		t.Errorf("sku (-want +got):\n%s", diff)
	}
}

// A unit price is units plus nanos, a billionth each.
func TestTrimCatalogReadsUnitsAndNanos(t *testing.T) {
	kept, _ := trimSlice(t, "Compute Engine", "compute-engine", "europe-west2")
	got := map[string]cost.SKU{}
	for _, sku := range kept {
		got[sku.ID] = sku
	}
	if core := got["C6A7-8E3D-2F51"]; core.Price != 0.028092 || core.Unit != "h" {
		t.Errorf("core = %v %s, want 0.028092 an h", core.Price, core.Unit)
	}
	if ram := got["4B19-D0F6-A83C"]; ram.Price != 0.003765 || ram.Unit != "GiBy.h" {
		t.Errorf("ram = %v %s, want 0.003765 a GiBy.h", ram.Price, ram.Unit)
	}
}

func TestTrimCatalogReportsALookupThatMatchesNothing(t *testing.T) {
	lookups, err := catalogLookups()
	if err != nil {
		t.Fatal(err)
	}
	wanted := lookups["Cloud SQL"]["europe-west2"]
	_, _, misses, err := trimCatalog("Cloud SQL", nil, map[string][]cost.Lookup{"europe-west2": wanted})
	if err != nil {
		t.Fatal(err)
	}
	if len(misses) != len(wanted) {
		t.Fatalf("misses = %d, want one per lookup (%d)", len(misses), len(wanted))
	}
	if !strings.Contains(misses[0], "matches nothing") {
		t.Errorf("miss = %q", misses[0])
	}
}

func TestTrimCatalogReportsALookupThatMatchesSeveralSkus(t *testing.T) {
	loose := cost.Lookup{Label: "loose", Service: "Cloud Storage", Filters: []cost.Filter{
		{Attribute: "description", Value: `.*Standard Storage London`, Pattern: true},
	}}
	_, _, misses, err := trimCatalog("Cloud Storage", loadSlice(t, "cloud-storage"), map[string][]cost.Lookup{"europe-west2": {loose}})
	if err != nil {
		t.Fatal(err)
	}
	if len(misses) != 1 || !strings.Contains(misses[0], "matches 2 skus") {
		t.Errorf("misses = %v", misses)
	}
}

func TestRunCatalogRefusesToRunWithoutAKey(t *testing.T) {
	t.Setenv(catalogKeyVar, "")
	err := runCatalog(false)
	if err == nil || !strings.Contains(err.Error(), catalogKeyVar) {
		t.Errorf("error = %v, want one naming %s", err, catalogKeyVar)
	}
}

func TestEveryServiceTheMatchersNameHasACatalogID(t *testing.T) {
	lookups, err := catalogLookups()
	if err != nil {
		t.Fatal(err)
	}
	for service := range lookups {
		if _, ok := gcp.CatalogServices[service]; !ok {
			t.Errorf("the matchers name %s, which the refresh has no catalog id for", service)
		}
	}
	if len(lookups) != len(gcp.CatalogServices) {
		t.Errorf("the matchers name %d services, the refresh knows %d", len(lookups), len(gcp.CatalogServices))
	}
}
