package azure

import (
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

type Group struct {
	ID       ir.ID
	Name     ir.Value
	Location ir.Value
}

const groupLabel = "resource group"

func ensureGroup(ctx *resolve.Context) *Group {
	if g, ok := ctx.Scratch[groupLabel].(*Group); ok {
		return g
	}
	r := ctx.Add(ir.Resource{
		Type:        "azurerm_resource_group",
		Name:        "main",
		SourceLabel: groupLabel,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named("rg"))),
			ir.A("location", ir.Str(ctx.Project.Region)),
		},
	})
	id := ir.ID{Type: r.Type, Name: r.Name}
	g := &Group{ID: id, Name: ir.R(id, ir.Field("name")), Location: ir.R(id, ir.Field("location"))}
	ctx.Scratch[groupLabel] = g
	return g
}

// Located opens the arguments of a resource that takes its group and its location from here.
// Grouped is for the ones whose parent already has a location, such as a subnet.
func (g *Group) Located(name string, args ...ir.Attr) ir.Attrs {
	return append(ir.Attrs{
		ir.A("name", ir.Str(name)),
		ir.A("resource_group_name", g.Name),
		ir.A("location", g.Location),
	}, args...)
}

func (g *Group) Grouped(name string, args ...ir.Attr) ir.Attrs {
	return append(ir.Attrs{
		ir.A("name", ir.Str(name)),
		ir.A("resource_group_name", g.Name),
	}, args...)
}
