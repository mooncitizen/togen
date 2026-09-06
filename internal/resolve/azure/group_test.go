package azure

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
)

func TestResourceGroupIsNamedForTheProjectAndPlacedInItsRegion(t *testing.T) {
	ctx := newContext(t, nil)
	g := ensureGroup(ctx)

	if diff := cmp.Diff(groupID, g.ID); diff != "" {
		t.Errorf("id (-want +got):\n%s", diff)
	}
	r := firstOfType(t, ctx, "azurerm_resource_group")
	want := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-rg")),
		ir.A("location", ir.Str("uksouth")),
	}
	if diff := cmp.Diff(want, r.Args); diff != "" {
		t.Errorf("args (-want +got):\n%s", diff)
	}
	if r.SourceLabel != "resource group" || r.SourceNode != "" {
		t.Errorf("source = %q/%q", r.SourceNode, r.SourceLabel)
	}
}

func TestResourceGroupIsCreatedOncePerContext(t *testing.T) {
	ctx := newContext(t, nil)
	a := ensureGroup(ctx)
	b := ensureGroup(ctx)
	if a != b {
		t.Error("ensureGroup built a second group")
	}
	if got := countOfType(ctx, "azurerm_resource_group"); got != 1 {
		t.Errorf("resource groups = %d", got)
	}
}

func TestGroupPlacesResourcesByReference(t *testing.T) {
	g := ensureGroup(newContext(t, nil))
	sku := ir.A("sku", ir.Str("Basic"))

	wantLocated := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-jobs")),
		ir.A("resource_group_name", ir.R(groupID, ir.Field("name"))),
		ir.A("location", ir.R(groupID, ir.Field("location"))),
		sku,
	}
	if diff := cmp.Diff(wantLocated, g.Located("shop-dev-jobs", sku)); diff != "" {
		t.Errorf("Located (-want +got):\n%s", diff)
	}

	wantGrouped := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-jobs")),
		ir.A("resource_group_name", ir.R(groupID, ir.Field("name"))),
		sku,
	}
	if diff := cmp.Diff(wantGrouped, g.Grouped("shop-dev-jobs", sku)); diff != "" {
		t.Errorf("Grouped (-want +got):\n%s", diff)
	}
}
