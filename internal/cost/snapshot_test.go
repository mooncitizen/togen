package cost

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
)

func encodeFixture(t *testing.T, mutate func(doc map[string]any)) []byte {
	t.Helper()
	raw, err := json.Marshal(fixture())
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	mutate(doc)
	raw, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestLoadReadsAWellFormedSnapshot(t *testing.T) {
	s, err := Load(encodeFixture(t, func(map[string]any) {}))
	if err != nil {
		t.Fatal(err)
	}
	if s.Provider != "aws" || s.Date != "2026-09-05" || len(s.SKUs) != 9 {
		t.Errorf("snapshot = %+v", s)
	}
	if diff := cmp.Diff([]string{"eu-west-2", "us-east-1"}, s.Regions()); diff != "" {
		t.Errorf("regions (-want +got):\n%s", diff)
	}
}

func TestLoadRefusesABrokenSnapshot(t *testing.T) {
	for _, c := range []struct {
		name   string
		mutate func(doc map[string]any)
		want   string
	}{
		{"no provider", func(doc map[string]any) { doc["provider"] = "" }, "provider and currency are required"},
		{"bad date", func(doc map[string]any) { doc["date"] = "5 September 2026" }, `date "5 September 2026" is not YYYY-MM-DD`},
		{"no skus", func(doc map[string]any) { doc["skus"] = []any{} }, "has no skus"},
		{"sku without a unit", func(doc map[string]any) {
			doc["skus"].([]any)[0].(map[string]any)["unit"] = ""
		}, "sku 0 needs an id, a service and a unit"},
		{"negative price", func(doc map[string]any) {
			doc["skus"].([]any)[0].(map[string]any)["price"] = -1
		}, "has a negative price"},
		{"duplicate sku", func(doc map[string]any) {
			skus := doc["skus"].([]any)
			doc["skus"] = append(skus, skus[0])
		}, "sku db-euw2 appears twice"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := Load(encodeFixture(t, c.mutate))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want it to contain %q", err, c.want)
			}
		})
	}
	if _, err := Load([]byte("{ not json")); err == nil {
		t.Error("malformed JSON loaded")
	}
}

func TestWarningAppearsAfterNinetyDays(t *testing.T) {
	s := fixture()
	taken := s.Taken()
	if got := s.Warning(taken.Add(89 * 24 * time.Hour)); got != "" {
		t.Errorf("warning at 89 days = %q", got)
	}
	if got := s.Warning(taken.Add(90 * 24 * time.Hour)); got != "" {
		t.Errorf("warning at 90 days = %q", got)
	}
	want := "the bundled aws prices are 91 days old (taken 2026-09-05)"
	if got := s.Warning(taken.Add(91 * 24 * time.Hour)); got != want {
		t.Errorf("warning at 91 days = %q, want %q", got, want)
	}
}

func TestBundledCarriesAnAwsSnapshotAndNothingForGcp(t *testing.T) {
	s, err := Bundled(ir.ProviderAWS)
	if err != nil {
		t.Fatal(err)
	}
	if s == nil || s.Provider != ir.ProviderAWS || s.Currency != "USD" {
		t.Fatalf("snapshot = %+v", s)
	}
	for _, region := range []string{"eu-west-2", "us-east-1", "ap-northeast-3"} {
		if _, err := s.Find(Lookup{Label: "nat", Service: "AmazonEC2", Filters: []Filter{
			{Attribute: "productFamily", Value: "NAT Gateway"},
			{Attribute: "usagetype", Value: `(\w+-)?NatGateway-Hours`, Pattern: true},
			{Attribute: "regionCode", Value: region},
		}}); err != nil {
			t.Error(err)
		}
	}
	none, err := Bundled(ir.ProviderGCP)
	if err != nil || none != nil {
		t.Errorf("gcp = %v, %v", none, err)
	}
}
