package workspace

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/mooncitizen/togen/internal/ir"
)

const (
	ViewsVersion = 1
	OverviewID   = "overview"

	viewsSchemaURL = "https://togen.dev/schema/views.schema.json"
	viewsShown     = "togen/views.json"
)

// Written by just generate: go:embed cannot reach schema/ from here.
//
//go:embed views.schema.json
var viewsSchemaJSON []byte

var viewsSchema = &compiledSchema{url: viewsSchemaURL, raw: viewsSchemaJSON}

type Views struct {
	Version int    `json:"version"`
	Views   []View `json:"views"`
}

type View struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Nodes ViewNodes `json:"nodes"`
}

// Every node in the project, written as "*", or the listed ids.
type ViewNodes struct {
	All bool
	IDs []string
}

func (n ViewNodes) MarshalJSON() ([]byte, error) {
	if n.All {
		return json.Marshal("*")
	}
	if n.IDs == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(n.IDs)
}

func (n *ViewNodes) UnmarshalJSON(raw []byte) error {
	var word string
	if err := json.Unmarshal(raw, &word); err == nil {
		if word != "*" {
			return errors.New(`nodes must be "*" or a list of node ids`)
		}
		*n = ViewNodes{All: true}
		return nil
	}
	var ids []string
	if err := json.Unmarshal(raw, &ids); err != nil {
		return errors.New(`nodes must be "*" or a list of node ids`)
	}
	*n = ViewNodes{IDs: ids}
	return nil
}

func DefaultViews() Views {
	return Views{Version: ViewsVersion, Views: []View{{ID: OverviewID, Name: "Overview", Nodes: ViewNodes{All: true}}}}
}

// A project with no views file has the overview and nothing else (ADR 0008).
func LoadViews(cwd string) (Views, error) {
	path := ViewsPath(cwd)
	if !Exists(path) {
		return DefaultViews(), nil
	}
	raw, err := ReadJSONFile(path, cwd)
	if err != nil {
		return Views{}, err
	}
	views, err := ParseViews(raw)
	if err != nil {
		return Views{}, err
	}
	if errs := viewErrors(views); len(errs) > 0 {
		return Views{}, errs
	}
	return views, nil
}

func ParseViews(raw []byte) (Views, error) {
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return Views{}, &Error{viewsShown + " is not valid JSON"}
	}
	if errs := viewsSchema.check(instance, viewsShown); len(errs) > 0 {
		return Views{}, errs
	}
	var views Views
	if err := json.Unmarshal(raw, &views); err != nil {
		return Views{}, &Error{fmt.Sprintf("%s: %s", viewsShown, err)}
	}
	if views.Version > ViewsVersion {
		return Views{}, &Error{versionMessage(viewsShown, views.Version, ViewsVersion)}
	}
	return views, nil
}

func ValidateViews(views Views, project *ir.Project) ir.Errors {
	errs := viewErrors(views)
	known := make(map[string]bool, len(project.Nodes))
	for _, n := range project.Nodes {
		known[n.ID] = true
	}
	for i, v := range views.Views {
		for j, id := range v.Nodes.IDs {
			if !known[id] {
				errs = append(errs, ir.ValidationError{
					Path:    fmt.Sprintf("views.%d.nodes.%d", i, j),
					Message: fmt.Sprintf("view '%s' refers to missing node '%s'", v.ID, id),
				})
			}
		}
	}
	return errs
}

func viewErrors(views Views) ir.Errors {
	var errs ir.Errors
	seen := make(map[string]bool, len(views.Views))
	for i, v := range views.Views {
		if seen[v.ID] {
			errs = append(errs, ir.ValidationError{Path: fmt.Sprintf("views.%d.id", i), Message: fmt.Sprintf("duplicate view id '%s'", v.ID)})
		}
		seen[v.ID] = true
	}
	if !seen[OverviewID] {
		errs = append(ir.Errors{{Path: "views", Message: "every project has an 'overview' view"}}, errs...)
	}
	return errs
}

func WriteViews(cwd string, views Views) error {
	return WriteJSONFile(ViewsPath(cwd), views)
}
