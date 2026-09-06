package cost

import (
	"strings"
	"testing"
)

func fixture() *Snapshot {
	return &Snapshot{
		Provider: "aws",
		Date:     "2026-09-05",
		Currency: "USD",
		Versions: map[string]map[string]string{"svc": {"eu-west-2": "v1", "us-east-1": "v1"}},
		SKUs: []SKU{
			{ID: "db-euw2", Service: "svc", Attributes: map[string]string{"kind": "db", "regionCode": "eu-west-2"}, Unit: "Hrs", Price: 0.018},
			{ID: "db-use1", Service: "svc", Attributes: map[string]string{"kind": "db", "regionCode": "us-east-1"}, Unit: "Hrs", Price: 0.016},
			{ID: "disk-euw2", Service: "svc", Attributes: map[string]string{"kind": "disk", "regionCode": "eu-west-2"}, Unit: "GB-Mo", Price: 0.133},
			{ID: "disk-use1", Service: "svc", Attributes: map[string]string{"kind": "disk", "regionCode": "us-east-1"}, Unit: "GB-Mo", Price: 0.115},
			{ID: "nat-euw2", Service: "svc", Attributes: map[string]string{"kind": "nat", "regionCode": "eu-west-2", "usagetype": "EUW2-NatGateway-Hours"}, Unit: "Hrs", Price: 0.05},
			{ID: "regional-nat-euw2", Service: "svc", Attributes: map[string]string{"kind": "nat", "regionCode": "eu-west-2", "usagetype": "EUW2-RegionalNatGateway-Hours"}, Unit: "Hrs", Price: 0.05},
			{ID: "nat-use1", Service: "svc", Attributes: map[string]string{"kind": "nat", "regionCode": "us-east-1", "usagetype": "NatGateway-Hours"}, Unit: "Hrs", Price: 0.045},
			{ID: "other-service", Service: "other", Attributes: map[string]string{"kind": "db", "regionCode": "eu-west-2"}, Unit: "Hrs", Price: 9},
			{ID: "requests-euw2", Service: "svc", Attributes: map[string]string{"kind": "requests", "regionCode": "eu-west-2"}, Unit: "Requests", Price: 0.000001, UpTo: 300000000},
		},
	}
}

func lookup(kind, region string) Lookup {
	return Lookup{
		Label:    kind,
		Service:  "svc",
		Filters:  []Filter{{Attribute: "kind", Value: kind}, {Attribute: "regionCode", Value: region}},
		Quantity: 730,
		Unit:     "h",
	}
}

func TestFindReturnsTheOneSkuTheFiltersPick(t *testing.T) {
	sku, err := fixture().Find(lookup("db", "eu-west-2"))
	if err != nil {
		t.Fatal(err)
	}
	if sku.ID != "db-euw2" || sku.Price != 0.018 {
		t.Errorf("sku = %+v", sku)
	}
}

func TestFindNamesTheFiltersWhenNothingMatches(t *testing.T) {
	_, err := fixture().Find(lookup("db", "eu-west-9"))
	if err == nil {
		t.Fatal("no error")
	}
	want := "no aws sku matches db (svc, kind=db, regionCode=eu-west-9)"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

func TestFindRefusesSeveralMatches(t *testing.T) {
	loose := Lookup{Label: "nat", Service: "svc", Filters: []Filter{{Attribute: "kind", Value: "nat"}, {Attribute: "regionCode", Value: "eu-west-2"}}}
	_, err := fixture().Find(loose)
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{"2 aws skus match nat", "the matcher is too loose", "nat-euw2, regional-nat-euw2"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
}

func TestAPatternFilterMatchesTheWholeAttribute(t *testing.T) {
	nat := Lookup{Label: "nat", Service: "svc", Filters: []Filter{
		{Attribute: "kind", Value: "nat"},
		{Attribute: "usagetype", Value: `(\w+-)?NatGateway-Hours`, Pattern: true},
		{Attribute: "regionCode", Value: "eu-west-2"},
	}}
	sku, err := fixture().Find(nat)
	if err != nil {
		t.Fatal(err)
	}
	if sku.ID != "nat-euw2" {
		t.Errorf("sku = %s, want nat-euw2", sku.ID)
	}

	nat.Filters[2].Value = "us-east-1"
	sku, err = fixture().Find(nat)
	if err != nil {
		t.Fatal(err)
	}
	if sku.ID != "nat-use1" {
		t.Errorf("sku = %s, want nat-use1", sku.ID)
	}
}

func TestALookupPicksTheFirstTierOfATieredMeter(t *testing.T) {
	matches := lookup("db", "eu-west-2").Predicate()
	row := map[string]string{"kind": "db", "regionCode": "eu-west-2"}
	if !matches(row) {
		t.Error("a row with no tiers does not match")
	}
	row[StartingRange] = "0"
	if !matches(row) {
		t.Error("the first tier does not match")
	}
	row[StartingRange] = "6000000000"
	if matches(row) {
		t.Error("a later tier matches")
	}
}

func TestFindStaysWithinTheService(t *testing.T) {
	sku, err := fixture().Find(lookup("db", "eu-west-2"))
	if err != nil {
		t.Fatal(err)
	}
	if sku.Service != "svc" {
		t.Errorf("service = %s", sku.Service)
	}
}

func TestDescribeListsTheServiceAndFilters(t *testing.T) {
	l := Lookup{Service: "svc", Filters: []Filter{{Attribute: "a", Value: "1"}, {Attribute: "b", Value: "x.*", Pattern: true}}}
	if got, want := l.Describe(), "svc, a=1, b~x.*"; got != want {
		t.Errorf("describe = %q, want %q", got, want)
	}
}
