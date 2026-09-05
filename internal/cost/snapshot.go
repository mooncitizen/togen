// Package cost estimates a project's monthly bill from a bundled snapshot of list prices.
package cost

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sync"
	"time"

	"github.com/mooncitizen/togen/internal/ir"
)

//go:embed prices/*.json
var bundled embed.FS

const (
	MaxAge     = 90 * 24 * time.Hour
	DateLayout = "2006-01-02"
)

type Snapshot struct {
	Provider ir.CloudProvider `json:"provider"`
	Source   string           `json:"source"`
	Date     string           `json:"date"`
	Currency string           `json:"currency"`
	// Offer file version per service per region, as the provider names it.
	Versions map[string]map[string]string `json:"versions"`
	SKUs     []SKU                        `json:"skus,omitempty"`
}

type SKU struct {
	ID         string            `json:"sku"`
	Service    string            `json:"service"`
	Attributes map[string]string `json:"attributes"`
	Unit       string            `json:"unit"`
	Price      float64           `json:"price"`
}

func Load(raw []byte) (*Snapshot, error) {
	var s Snapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("price snapshot: %w", err)
	}
	if s.Provider == "" || s.Currency == "" {
		return nil, fmt.Errorf("price snapshot: provider and currency are required")
	}
	if _, err := time.Parse(DateLayout, s.Date); err != nil {
		return nil, fmt.Errorf("price snapshot for %s: date %q is not YYYY-MM-DD", s.Provider, s.Date)
	}
	if len(s.SKUs) == 0 {
		return nil, fmt.Errorf("price snapshot for %s has no skus", s.Provider)
	}
	seen := map[string]bool{}
	for i, sku := range s.SKUs {
		switch {
		case sku.ID == "" || sku.Service == "" || sku.Unit == "":
			return nil, fmt.Errorf("price snapshot for %s: sku %d needs an id, a service and a unit", s.Provider, i)
		case sku.Price < 0:
			return nil, fmt.Errorf("price snapshot for %s: sku %s has a negative price", s.Provider, sku.ID)
		case seen[sku.Service+" "+sku.ID]:
			return nil, fmt.Errorf("price snapshot for %s: sku %s appears twice", s.Provider, sku.ID)
		}
		seen[sku.Service+" "+sku.ID] = true
	}
	return &s, nil
}

func (s *Snapshot) Taken() time.Time {
	taken, _ := time.Parse(DateLayout, s.Date)
	return taken
}

func (s *Snapshot) Regions() []string {
	var out []string
	for _, regions := range s.Versions {
		for region := range regions {
			if !slices.Contains(out, region) {
				out = append(out, region)
			}
		}
	}
	slices.Sort(out)
	return out
}

// Prices move slowly, so an old snapshot is a warning rather than a refusal.
func (s *Snapshot) Warning(now time.Time) string {
	age := now.Sub(s.Taken())
	if age <= MaxAge {
		return ""
	}
	return fmt.Sprintf("the bundled %s prices are %d days old (taken %s)", s.Provider, int(age.Hours()/24), s.Date)
}

var (
	once      sync.Once
	snapshots map[ir.CloudProvider]*Snapshot
	loadErr   error
)

// Bundled returns nil without an error for a provider with no snapshot yet.
func Bundled(provider ir.CloudProvider) (*Snapshot, error) {
	once.Do(func() { snapshots, loadErr = loadBundled() })
	if loadErr != nil {
		return nil, loadErr
	}
	return snapshots[provider], nil
}

func loadBundled() (map[ir.CloudProvider]*Snapshot, error) {
	out := map[ir.CloudProvider]*Snapshot{}
	entries, err := fs.ReadDir(bundled, "prices")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		raw, err := bundled.ReadFile(path.Join("prices", entry.Name()))
		if err != nil {
			return nil, err
		}
		s, err := Load(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name(), err)
		}
		out[s.Provider] = s
	}
	return out, nil
}
