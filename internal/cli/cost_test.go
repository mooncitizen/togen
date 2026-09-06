package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/workspace"
)

func TestCostPrintsATableForAValidProject(t *testing.T) {
	cwd := generateCwd(t)
	result := Cost(cwd, false)
	if result.Code != 0 {
		t.Fatalf("code = %d, lines = %v", result.Code, result.Lines)
	}
	if result.Lines[0] != "main-db  database      db.t4g.micro, postgres 17, single-AZ, 20 GB" {
		t.Errorf("first line = %q", result.Lines[0])
	}
	var priced, omitted []string
	for _, line := range result.Lines {
		switch {
		case strings.HasPrefix(line, "  instance") || strings.HasPrefix(line, "  storage gp2") || strings.HasPrefix(line, "  nat gateway"):
			priced = append(priced, strings.Fields(line)[0])
		case strings.HasPrefix(line, "  api") || strings.HasPrefix(line, "  handler"):
			omitted = append(omitted, strings.Join(strings.Fields(line), " "))
		}
	}
	if diff := cmp.Diff([]string{"instance", "storage", "nat"}, priced); diff != "" {
		t.Errorf("priced lines (-want +got):\n%s", diff)
	}
	wantOmitted := []string{
		"api gateway no aws prices for this node type yet",
		"handler function no aws prices for this node type yet",
	}
	if diff := cmp.Diff(wantOmitted, omitted); diff != "" {
		t.Errorf("not priced (-want +got):\n%s", diff)
	}
	last := result.Lines[len(result.Lines)-1]
	if !strings.HasPrefix(last, "total  ") || !strings.Contains(last, " USD/month  eu-west-2, list prices from ") || !strings.HasSuffix(last, ", estimate not a quote") {
		t.Errorf("last line = %q", last)
	}
}

func TestCostPrintsTheDocumentAsJSON(t *testing.T) {
	cwd := generateCwd(t)
	result := Cost(cwd, true)
	if result.Code != 0 {
		t.Fatalf("code = %d, lines = %v", result.Code, result.Lines)
	}
	var doc cost.Document
	if err := json.Unmarshal([]byte(strings.Join(result.Lines, "\n")), &doc); err != nil {
		t.Fatalf("parse: %v\n%s", err, strings.Join(result.Lines, "\n"))
	}
	if doc.Provider != "aws" || doc.Region != "eu-west-2" || doc.Currency != "USD" || doc.SnapshotDate == "" {
		t.Errorf("document = %+v", doc)
	}
	if len(doc.Items) != 2 || doc.Items[0].Name != "main-db" || len(doc.Items[0].Lines) != 2 || doc.Items[1].Name != "network" {
		t.Errorf("items = %+v", doc.Items)
	}
	if want := doc.Items[0].Subtotal + doc.Items[1].Subtotal; doc.Total != want || doc.Total <= 0 {
		t.Errorf("total = %v, want %v", doc.Total, want)
	}
	if doc.Note != "list prices from "+doc.SnapshotDate+", estimate not a quote" {
		t.Errorf("note = %q", doc.Note)
	}
}

func TestCostReportsValidationErrors(t *testing.T) {
	cwd := generateCwd(t)
	project := exampleProject()
	project["edges"] = []any{map[string]any{"id": "e1", "from": "n1", "to": "zz", "relation": "routes"}}
	writeProject(t, cwd, project)

	result := Cost(cwd, false)
	if result.Code != 1 {
		t.Fatalf("code = %d, want 1", result.Code)
	}
	want := []string{"edges.0 (edge e1): edge refers to missing node 'zz'"}
	if diff := cmp.Diff(want, result.Lines); diff != "" {
		t.Errorf("lines (-want +got):\n%s", diff)
	}
}

// The gcp resolver refuses every node type for now, so only an empty project reaches the estimate.
func TestCostPricesNothingForAProviderWithoutPrices(t *testing.T) {
	cwd := generateCwd(t)
	project := exampleProject()
	project["provider"] = "gcp"
	project["region"] = "europe-west2"
	project["nodes"] = []any{}
	project["edges"] = []any{}
	writeProject(t, cwd, project)

	result := Cost(cwd, false)
	if result.Code != 0 {
		t.Fatalf("code = %d, lines = %v", result.Code, result.Lines)
	}
	want := []string{
		"",
		"total  0.00 USD/month  europe-west2, no gcp prices are bundled yet",
	}
	if diff := cmp.Diff(want, result.Lines); diff != "" {
		t.Errorf("lines (-want +got):\n%s", diff)
	}
}

func TestCostReportsAMissingProject(t *testing.T) {
	cwd := t.TempDir()
	result := Cost(cwd, false)
	if result.Code != 1 {
		t.Fatalf("code = %d, want 1", result.Code)
	}
	if want := "togen/project.json not found. Run 'togen init' first."; result.Lines[0] != want {
		t.Errorf("line = %q, want %q", result.Lines[0], want)
	}
}

func TestCostNotesTheLegacyConfig(t *testing.T) {
	cwd := generateCwd(t)
	if result := Cost(cwd, false); result.Note != "" {
		t.Errorf("note = %q, want none", result.Note)
	}
	legacyConfig(t, cwd)
	result := Cost(cwd, false)
	if result.Code != 0 {
		t.Fatalf("code = %d, lines = %v", result.Code, result.Lines)
	}
	if result.Note != workspace.LegacyNote {
		t.Errorf("note = %q, want %q", result.Note, workspace.LegacyNote)
	}
}
