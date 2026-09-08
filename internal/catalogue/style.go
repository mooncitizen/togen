package catalogue

import (
	"fmt"
	"slices"
)

type Shape string

const (
	ShapeCard     Shape = "card"
	ShapeCylinder Shape = "cylinder"
	ShapeHexagon  Shape = "hexagon"
	ShapeCircle   Shape = "circle"
)

var Shapes = []Shape{ShapeCard, ShapeCylinder, ShapeHexagon, ShapeCircle}

// A box the canvas draws around nodes: the implicit network, and on Azure the resource group.
type Boundary struct {
	Kind  string `json:"kind"`
	Color string `json:"color"`
}

// How one entry is drawn. The colours are the category colours each vendor uses in its
// architecture icon set, and the icon ids name an icon in that set.
type Look struct {
	Color string `json:"color"`
	Icon  string `json:"icon"`
	Shape Shape  `json:"shape"`
}

func (l Look) check(where string) error {
	switch {
	case l.Color == "":
		return fmt.Errorf("%s has no colour", where)
	case l.Icon == "":
		return fmt.Errorf("%s has no icon", where)
	case !slices.Contains(Shapes, l.Shape):
		return fmt.Errorf("%s has shape '%s', which is not one of %v", where, l.Shape, Shapes)
	}
	return nil
}

// Kind is a Look with the service's own name, which is the shape schema/styles.json publishes.
type Kind struct {
	Color    string `json:"color"`
	Icon     string `json:"icon"`
	Shape    Shape  `json:"shape"`
	Resource string `json:"resource"`
}

type Scheme struct {
	Accent        string          `json:"accent"`
	Network       Boundary        `json:"network"`
	ResourceGroup *Boundary       `json:"resourceGroup,omitempty"`
	Kinds         map[string]Kind `json:"kinds"`
}
