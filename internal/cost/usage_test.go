package cost

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseRateNormalisesEveryPeriodToAMonth(t *testing.T) {
	for text, want := range map[string]float64{
		"1/min":      43800,
		"500/min":    21900000,
		"1/hour":     730,
		"12/hour":    8760,
		"1/day":      30,
		"1000/day":   30417,
		"1/month":    1,
		"2M/month":   2000000,
		"20k/day":    608333,
		"1.5M/month": 1500000,
		"0.5k/hour":  365000,
		"0/month":    0,
	} {
		got, err := ParseRate(text)
		if err != nil {
			t.Errorf("%s: %v", text, err)
		} else if got != want {
			t.Errorf("%s = %v, want %v", text, got, want)
		}
	}
}

func TestParseRateRefusesWhatIsNotARate(t *testing.T) {
	for _, text := range []string{"", "500", "500/", "/min", "500/sec", "500/week", "500/Month", "2m/month", "2G/month", "-5/min", "five/min", "500 /min", "1,000/day"} {
		if _, err := ParseRate(text); err == nil {
			t.Errorf("%q parsed", text)
		} else if !strings.Contains(err.Error(), RateHelp) {
			t.Errorf("%q: %v", text, err)
		}
	}
}

func TestAnUnsetRateIsZero(t *testing.T) {
	got, err := Rate("").PerMonth()
	if err != nil || got != 0 {
		t.Errorf("empty rate = %v, %v", got, err)
	}
}

func TestMergePrefersTheWrittenField(t *testing.T) {
	derived := Usage{
		"orders": {Requests: "1000/month", EgressGb: 5},
		"worker": {Invocations: "40/month"},
	}
	written := Usage{
		"orders": {Requests: "9/month"},
		"assets": {StorageGb: 20},
	}
	got := Merge(derived, written)
	if got["orders"].Requests != "9/month" {
		t.Fatalf("written requests should win: %+v", got["orders"])
	}
	if got["orders"].EgressGb != 5 {
		t.Fatalf("derived egress should survive: %+v", got["orders"])
	}
	if got["worker"].Invocations != "40/month" {
		t.Fatalf("a node absent from the written block keeps its derived record: %+v", got["worker"])
	}
	if got["assets"].StorageGb != 20 {
		t.Fatalf("a node only in the written block survives: %+v", got["assets"])
	}
}

func TestMergeWithNothingDerived(t *testing.T) {
	written := Usage{"orders": {Requests: "9/month"}}
	if got := Merge(nil, written); got["orders"].Requests != "9/month" {
		t.Fatalf("got %+v", got)
	}
}

func TestNodeUsageNamesTheKeysItSets(t *testing.T) {
	if got := (NodeUsage{}).Keys(); len(got) != 0 {
		t.Errorf("keys of nothing = %v", got)
	}
	want := []string{"requests", "invocations", "durationMs", "messages", "storageGb", "egressGb", "natGb"}
	all := NodeUsage{Requests: "1/min", Invocations: "1/min", DurationMs: 1, Messages: "1/min", StorageGb: 1, EgressGb: 1, NatGb: 1}
	if diff := cmp.Diff(want, all.Keys()); diff != "" {
		t.Errorf("keys (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"invocations", "durationMs"}, (NodeUsage{Invocations: "2M/month", DurationMs: 300}).Keys()); diff != "" {
		t.Errorf("keys (-want +got):\n%s", diff)
	}
}
