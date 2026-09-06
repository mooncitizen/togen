package cost

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mooncitizen/togen/internal/ir"
)

type Item struct {
	Name    string
	Kind    string
	Summary string
	Lookups []Lookup
	// Meters the usage block drives, with the quantity it gives. One left at zero is listed as
	// not priced rather than as a line; the refresh keeps every one of them in the snapshot.
	Usage []Lookup
	// Charges the item incurs that no lookup covers, so the output can say so.
	Unpriced []string
}

// Node reports false for a type with no matcher yet. Catalogue lists every lookup the
// matchers could ask for in a region, so the refresh can check each against the offer files.
type Matchers interface {
	Node(n ir.Node, region string, usage NodeUsage) (Item, bool, error)
	Implicit(p *ir.Project, usage Usage) []Item
	Catalogue(region string) ([]Lookup, error)
}

type Document struct {
	Provider     ir.CloudProvider `json:"provider"`
	Region       string           `json:"region"`
	Currency     string           `json:"currency"`
	Items        []Priced         `json:"items"`
	NotPriced    []Omission       `json:"notPriced"`
	Total        float64          `json:"total"`
	SnapshotDate string           `json:"snapshotDate,omitempty"`
	Note         string           `json:"note"`
	Warning      string           `json:"warning,omitempty"`
}

type Priced struct {
	Name     string  `json:"name"`
	Kind     string  `json:"kind"`
	Summary  string  `json:"summary,omitempty"`
	Note     string  `json:"note,omitempty"`
	Lines    []Line  `json:"lines"`
	Subtotal float64 `json:"subtotal"`
}

type Line struct {
	Label     string  `json:"label"`
	Quantity  float64 `json:"quantity"`
	Unit      string  `json:"unit"`
	UnitPrice float64 `json:"unitPrice"`
	Amount    float64 `json:"amount"`
	SKU       string  `json:"sku"`
	// Set when the quantity outgrows the first tier the snapshot holds the rate of.
	Note string `json:"note,omitempty"`
}

type Omission struct {
	Name   string `json:"name"`
	Kind   string `json:"kind,omitempty"`
	Reason string `json:"reason"`
}

// A nil Matchers means no prices are bundled for the provider, and every node comes back as
// not priced. An item with usage meters and no usage entry is priced on defaults and says so.
func Estimate(project *ir.Project, m Matchers, snapshot *Snapshot, usage Usage, now time.Time) (Document, error) {
	project, err := ir.ApplyDefaults(project)
	if err != nil {
		return Document{}, err
	}
	doc := Document{
		Provider:  project.Provider,
		Region:    project.Region,
		Currency:  "USD",
		Items:     []Priced{},
		NotPriced: []Omission{},
		Note:      fmt.Sprintf("no %s prices are bundled yet", project.Provider),
	}
	if m == nil {
		for _, n := range project.Nodes {
			doc.NotPriced = append(doc.NotPriced, Omission{Name: n.Name, Kind: string(n.Type), Reason: doc.Note})
		}
		return doc, nil
	}
	if snapshot == nil {
		return Document{}, fmt.Errorf("no %s price snapshot is bundled", project.Provider)
	}
	if !slices.Contains(snapshot.Regions(), project.Region) {
		return Document{}, fmt.Errorf("no %s prices are bundled for region '%s'", project.Provider, project.Region)
	}
	doc.Currency = snapshot.Currency
	doc.SnapshotDate = snapshot.Date
	doc.Note = fmt.Sprintf("list prices from %s, estimate not a quote", snapshot.Date)
	doc.Warning = snapshot.Warning(now)

	var items []Item
	for _, n := range project.Nodes {
		item, ok, err := m.Node(n, project.Region, usage[n.Name])
		if err != nil {
			return Document{}, err
		}
		if !ok {
			doc.NotPriced = append(doc.NotPriced, Omission{
				Name:   n.Name,
				Kind:   string(n.Type),
				Reason: fmt.Sprintf("no %s prices for this node type yet", project.Provider),
			})
			continue
		}
		items = append(items, item)
	}
	items = append(items, m.Implicit(project, usage)...)

	for _, item := range items {
		priced, omitted, err := price(item, snapshot)
		if err != nil {
			return Document{}, fmt.Errorf("%s: %w", item.Name, err)
		}
		if _, set := usage[item.Name]; !set && len(item.Usage) > 0 {
			priced.Note = DefaultsNote
		}
		doc.Items = append(doc.Items, priced)
		doc.Total = round(doc.Total + priced.Subtotal)
		for _, reason := range append(omitted, item.Unpriced...) {
			doc.NotPriced = append(doc.NotPriced, Omission{Name: item.Name, Kind: item.Kind, Reason: reason})
		}
	}
	return doc, nil
}

// The usage meters left at zero come back by label, for the not priced list.
func price(item Item, snapshot *Snapshot) (Priced, []string, error) {
	out := Priced{Name: item.Name, Kind: item.Kind, Summary: item.Summary, Lines: []Line{}}
	var omitted []string
	for i, l := range append(slices.Clone(item.Lookups), item.Usage...) {
		if i >= len(item.Lookups) && l.Quantity == 0 {
			omitted = append(omitted, l.Label)
			continue
		}
		sku, err := snapshot.Find(l)
		if err != nil {
			return Priced{}, nil, err
		}
		amount := round(l.Quantity * sku.Price)
		line := Line{
			Label:     l.Label,
			Quantity:  l.Quantity,
			Unit:      l.Unit,
			UnitPrice: sku.Price,
			Amount:    amount,
			SKU:       sku.ID,
		}
		if sku.UpTo > 0 && l.Quantity > sku.UpTo {
			line.Note = fmt.Sprintf("past the first tier of %s %s, priced at its rate", grouped(sku.UpTo), l.Unit)
		}
		out.Lines = append(out.Lines, line)
		out.Subtotal = round(out.Subtotal + amount)
	}
	return out, omitted, nil
}

func grouped(n float64) string {
	digits := strconv.FormatFloat(n, 'f', 0, 64)
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	return b.String()
}

// Amounts are rounded to the cent as they are made, so the column adds up as printed.
func round(amount float64) float64 {
	return math.Round(amount*100) / 100
}
