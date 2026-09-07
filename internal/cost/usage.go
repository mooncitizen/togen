package cost

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
)

// A rate is a count per period, such as 500/min or 2M/month, and every period is normalised
// to a 730 hour month.
const (
	RatePattern   = `^[0-9]+(\.[0-9]+)?[kM]?/(min|hour|day|month)$`
	RateHelp      = "must be a number per period, such as 500/min, 20k/day or 2M/month"
	HoursPerMonth = 730

	// The usage entry for the network the resolver adds, which is no node of the project's.
	NetworkUsage = "network"
	DefaultsNote = "priced on defaults, no usage set"
)

var (
	rateForm = regexp.MustCompile(RatePattern)
	perMonth = map[string]float64{
		"min":   60 * HoursPerMonth,
		"hour":  HoursPerMonth,
		"day":   HoursPerMonth / 24.0,
		"month": 1,
	}
	scale = map[string]float64{"": 1, "k": 1e3, "M": 1e6}
)

type Rate string

// The result is a whole count: a rate is requests, invocations or messages. Unset is zero.
func (r Rate) PerMonth() (float64, error) {
	if r == "" {
		return 0, nil
	}
	return ParseRate(string(r))
}

func ParseRate(text string) (float64, error) {
	if !rateForm.MatchString(text) {
		return 0, fmt.Errorf("%q %s", text, RateHelp)
	}
	count, period, _ := strings.Cut(text, "/")
	suffix := ""
	if last := count[len(count)-1]; last == 'k' || last == 'M' {
		suffix, count = string(last), count[:len(count)-1]
	}
	n, err := strconv.ParseFloat(count, 64)
	if err != nil {
		return 0, fmt.Errorf("%q %s", text, RateHelp)
	}
	return math.Round(n * scale[suffix] * perMonth[period]), nil
}

type Usage map[string]NodeUsage

type NodeUsage struct {
	Requests    Rate    `json:"requests,omitempty" yaml:"requests,omitempty" jsonschema:"description=Requests a gateway or a service serves or a bucket takes (a rate)"`
	Invocations Rate    `json:"invocations,omitempty" yaml:"invocations,omitempty" jsonschema:"description=Times a function runs (a rate)"`
	DurationMs  float64 `json:"durationMs,omitempty" yaml:"durationMs,omitempty" jsonschema:"minimum=0,description=Milliseconds one run of a function takes"`
	Messages    Rate    `json:"messages,omitempty" yaml:"messages,omitempty" jsonschema:"description=Messages a queue carries (a rate)"`
	StorageGb   float64 `json:"storageGb,omitempty" yaml:"storageGb,omitempty" jsonschema:"minimum=0,description=Gigabytes a bucket holds over the month"`
	EgressGb    float64 `json:"egressGb,omitempty" yaml:"egressGb,omitempty" jsonschema:"minimum=0,description=Gigabytes a bucket or service sends to the internet a month"`
	NatGb       float64 `json:"natGb,omitempty" yaml:"natGb,omitempty" jsonschema:"minimum=0,description=Gigabytes the network's NAT gateway processes a month"`
}

// The keys each node type takes; a type missing here takes none.
var UsageKeys = map[ir.NodeType][]string{
	ir.NodeGateway:  {"requests"},
	ir.NodeFunction: {"invocations", "durationMs"},
	ir.NodeQueue:    {"messages"},
	ir.NodeBucket:   {"storageGb", "egressGb", "requests"},
	ir.NodeService:  {"egressGb", "requests"},
}

var NetworkKeys = []string{"natGb"}

func (u NodeUsage) Keys() []string {
	var out []string
	for _, k := range []struct {
		name string
		set  bool
	}{
		{"requests", u.Requests != ""},
		{"invocations", u.Invocations != ""},
		{"durationMs", u.DurationMs != 0},
		{"messages", u.Messages != ""},
		{"storageGb", u.StorageGb != 0},
		{"egressGb", u.EgressGb != 0},
		{"natGb", u.NatGb != 0},
	} {
		if k.set {
			out = append(out, k.name)
		}
	}
	return out
}

func (u NodeUsage) Rates() map[string]Rate {
	return map[string]Rate{"requests": u.Requests, "invocations": u.Invocations, "messages": u.Messages}
}

// A field set by hand in togen.yml wins; the rest of the record is what the simulation
// worked out (ADR 0011).
func Merge(derived, written Usage) Usage {
	out := make(Usage, len(derived)+len(written))
	for name, entry := range derived {
		out[name] = entry
	}
	for name, entry := range written {
		merged := out[name]
		if entry.Requests != "" {
			merged.Requests = entry.Requests
		}
		if entry.Invocations != "" {
			merged.Invocations = entry.Invocations
		}
		if entry.DurationMs != 0 {
			merged.DurationMs = entry.DurationMs
		}
		if entry.Messages != "" {
			merged.Messages = entry.Messages
		}
		if entry.StorageGb != 0 {
			merged.StorageGb = entry.StorageGb
		}
		if entry.EgressGb != 0 {
			merged.EgressGb = entry.EgressGb
		}
		if entry.NatGb != 0 {
			merged.NatGb = entry.NatGb
		}
		out[name] = merged
	}
	return out
}
