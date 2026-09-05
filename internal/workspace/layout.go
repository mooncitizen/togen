package workspace

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	LayoutVersion = 2

	layoutSchemaURL = "https://togen.dev/schema/layout.schema.json"
	layoutShown     = "togen/layout.json"
)

// Written by just generate: go:embed cannot reach schema/ from here.
//
//go:embed layout.schema.json
var layoutSchemaJSON []byte

var layoutSchema = &compiledSchema{url: layoutSchemaURL, raw: layoutSchemaJSON}

type Layout struct {
	Version int                   `json:"version"`
	Views   map[string]ViewLayout `json:"views"`
}

type ViewLayout struct {
	Nodes    map[string]Position `json:"nodes"`
	Viewport Viewport            `json:"viewport"`
}

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Viewport struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}

func DefaultViewport() Viewport { return Viewport{Zoom: 1} }

func DefaultLayout() Layout {
	return Layout{
		Version: LayoutVersion,
		Views:   map[string]ViewLayout{OverviewID: {Nodes: map[string]Position{}, Viewport: DefaultViewport()}},
	}
}

func LoadLayout(cwd string) (Layout, error) {
	raw, err := ReadJSONFile(LayoutPath(cwd), cwd)
	if err != nil {
		return Layout{}, err
	}
	return ParseLayout(raw)
}

// A version 1 file is one drawing of the whole project, which is what the
// overview is (ADR 0008), so it reads as version 2 with that one view.
func ParseLayout(raw []byte) (Layout, error) {
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return Layout{}, &Error{layoutShown + " is not valid JSON"}
	}
	if errs := layoutSchema.check(instance, layoutShown); len(errs) > 0 {
		return Layout{}, errs
	}
	var doc struct {
		Version  int                   `json:"version"`
		Nodes    map[string]Position   `json:"nodes"`
		Viewport Viewport              `json:"viewport"`
		Views    map[string]ViewLayout `json:"views"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Layout{}, &Error{fmt.Sprintf("%s: %s", layoutShown, err)}
	}

	layout := Layout{Version: LayoutVersion, Views: map[string]ViewLayout{}}
	switch doc.Version {
	case 1:
		layout.Views[OverviewID] = placed(ViewLayout{Nodes: doc.Nodes, Viewport: doc.Viewport})
	case LayoutVersion:
		for id, view := range doc.Views {
			layout.Views[id] = placed(view)
		}
	default:
		return Layout{}, &Error{versionMessage(layoutShown, doc.Version, LayoutVersion)}
	}
	return layout, nil
}

// A zero viewport is a missing one: the schema does not let zoom be 0.
func placed(view ViewLayout) ViewLayout {
	if view.Nodes == nil {
		view.Nodes = map[string]Position{}
	}
	if view.Viewport == (Viewport{}) {
		view.Viewport = DefaultViewport()
	}
	return view
}

func WriteLayout(cwd string, layout Layout) error {
	return WriteJSONFile(LayoutPath(cwd), layout)
}
