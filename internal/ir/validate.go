package ir

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

const projectSchemaURL = "https://togen.dev/schema/project.schema.json"

type ValidationError struct {
	Path    string `json:"path"`
	NodeID  string `json:"nodeId,omitempty"`
	EdgeID  string `json:"edgeId,omitempty"`
	Message string `json:"message"`
}

func (e ValidationError) String() string {
	path := e.Path
	if path == "" {
		path = "project"
	}
	switch {
	case e.NodeID != "":
		return fmt.Sprintf("%s (node %s): %s", path, e.NodeID, e.Message)
	case e.EdgeID != "":
		return fmt.Sprintf("%s (edge %s): %s", path, e.EdgeID, e.Message)
	}
	return fmt.Sprintf("%s: %s", path, e.Message)
}

type Errors []ValidationError

func (e Errors) Error() string {
	lines := make([]string, len(e))
	for i, err := range e {
		lines[i] = err.String()
	}
	return strings.Join(lines, "\n")
}

func ValidateProject(raw []byte) (*Project, Errors) {
	sch, err := projectSchema()
	if err != nil {
		return nil, Errors{{Message: err.Error()}}
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, Errors{{Message: fmt.Sprintf("project file is not valid JSON: %v", err)}}
	}
	if err := sch.Validate(instance); err != nil {
		var invalid *jsonschema.ValidationError
		if !errors.As(err, &invalid) {
			return nil, Errors{{Message: err.Error()}}
		}
		return nil, schemaErrors(invalid, instance)
	}
	var p Project
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, Errors{{Message: err.Error()}}
	}
	if errs := semanticErrors(&p); len(errs) > 0 {
		return nil, errs
	}
	out, err := ApplyDefaults(&p)
	if err != nil {
		return nil, Errors{{Message: err.Error()}}
	}
	return out, nil
}

var (
	schemaOnce     sync.Once
	compiledSchema *jsonschema.Schema
	schemaErr      error
	printer        = message.NewPrinter(language.English)
)

func projectSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		var doc any
		doc, schemaErr = jsonschema.UnmarshalJSON(bytes.NewReader(projectSchemaJSON))
		if schemaErr != nil {
			return
		}
		c := jsonschema.NewCompiler()
		if schemaErr = c.AddResource(projectSchemaURL, doc); schemaErr != nil {
			return
		}
		compiledSchema, schemaErr = c.Compile(projectSchemaURL)
	})
	return compiledSchema, schemaErr
}

func schemaErrors(root *jsonschema.ValidationError, instance any) Errors {
	var out Errors
	var walk func(e *jsonschema.ValidationError, loc []string)
	walk = func(e *jsonschema.ValidationError, loc []string) {
		// v6.0.3 aliases a shared buffer into a propertyNames error's location, and the
		// pattern failure beneath it carries none, so both keep the enclosing object's.
		if _, ok := e.ErrorKind.(*kind.PropertyNames); !ok && len(e.InstanceLocation) > 0 {
			loc = e.InstanceLocation
		}
		causes := e.Causes
		if _, ok := e.ErrorKind.(*kind.OneOf); ok {
			causes = discriminated(e, loc)
			if len(causes) == 0 {
				if unknown, ok := unknownNodeType(loc, instance); ok {
					out = append(out, unknown)
					return
				}
			}
		}
		if len(causes) == 0 {
			out = append(out, schemaError(loc, e.ErrorKind.LocalizedString(printer), instance))
			return
		}
		for _, cause := range causes {
			walk(cause, loc)
		}
	}
	walk(root, root.InstanceLocation)
	return out
}

// A node is a oneOf over the seven types, discriminated by a const on one property.
// Only the branch the user meant is worth reporting; the other six failed on the const.
func discriminated(e *jsonschema.ValidationError, loc []string) []*jsonschema.ValidationError {
	var kept []*jsonschema.ValidationError
	for _, cause := range e.Causes {
		if !failsDiscriminator(cause, len(loc)+1) {
			kept = append(kept, cause)
		}
	}
	if len(kept) == 1 {
		return kept
	}
	return nil
}

func failsDiscriminator(e *jsonschema.ValidationError, depth int) bool {
	if _, ok := e.ErrorKind.(*kind.Const); ok && len(e.InstanceLocation) == depth {
		return true
	}
	for _, cause := range e.Causes {
		if failsDiscriminator(cause, depth) {
			return true
		}
	}
	return false
}

// Every branch failed its type const, so the raw oneOf wording says nothing useful.
func unknownNodeType(loc []string, instance any) (ValidationError, bool) {
	if len(loc) != 2 || loc[0] != "nodes" {
		return ValidationError{}, false
	}
	node, ok := itemAt(instance, loc[0], loc[1])
	if !ok {
		return ValidationError{}, false
	}
	given, ok := node["type"].(string)
	if !ok || given == "" || slices.Contains(NodeTypes, NodeType(given)) {
		return ValidationError{}, false
	}
	names := make([]string, len(NodeTypes))
	for i, t := range NodeTypes {
		names[i] = string(t)
	}
	id, _ := node["id"].(string)
	return ValidationError{
		Path:    strings.Join(loc, ".") + ".type",
		NodeID:  id,
		Message: fmt.Sprintf("unknown node type '%s', use one of %s", given, strings.Join(names, ", ")),
	}, true
}

func schemaError(loc []string, message string, instance any) ValidationError {
	out := ValidationError{Path: strings.Join(loc, "."), Message: message}
	if len(loc) >= 2 {
		id := idAt(instance, loc[0], loc[1])
		switch loc[0] {
		case "nodes":
			out.NodeID = id
		case "edges":
			out.EdgeID = id
		}
	}
	return out
}

func idAt(instance any, collection, index string) string {
	item, ok := itemAt(instance, collection, index)
	if !ok {
		return ""
	}
	id, _ := item["id"].(string)
	return id
}

func itemAt(instance any, collection, index string) (map[string]any, bool) {
	doc, ok := instance.(map[string]any)
	if !ok {
		return nil, false
	}
	items, ok := doc[collection].([]any)
	if !ok {
		return nil, false
	}
	i, err := strconv.Atoi(index)
	if err != nil || i < 0 || i >= len(items) {
		return nil, false
	}
	item, ok := items[i].(map[string]any)
	return item, ok
}

// A provider with no engine table has no opinion yet, so nothing is checked for it.
func engineVersionError(p *Project, n *Node, i int) (ValidationError, bool) {
	if n.Type != NodeDatabase {
		return ValidationError{}, false
	}
	props, err := NodeProps[DatabaseProps](*n)
	if err != nil || props.Version == "" {
		return ValidationError{}, false
	}
	engine := props.Engine
	if engine == "" {
		engine = EnginePostgres
	}
	table, ok := Engines[p.Provider]
	if !ok {
		return ValidationError{}, false
	}
	info, ok := table[engine]
	if !ok || slices.Contains(info.Versions, props.Version) {
		return ValidationError{}, false
	}
	return ValidationError{
		Path:   fmt.Sprintf("nodes.%d.properties.version", i),
		NodeID: n.ID,
		Message: fmt.Sprintf("database '%s': %s version '%s' is not supported on %s (use one of %s)",
			n.Name, engine, props.Version, p.Provider, strings.Join(info.Versions, ", ")),
	}, true
}

func semanticErrors(p *Project) Errors {
	var errs Errors
	byID := make(map[string]*Node, len(p.Nodes))
	names := make(map[string]bool, len(p.Nodes))
	gateways := 0

	for i := range p.Nodes {
		n := &p.Nodes[i]
		path := fmt.Sprintf("nodes.%d", i)
		if _, ok := byID[n.ID]; ok {
			errs = append(errs, ValidationError{Path: path, NodeID: n.ID, Message: fmt.Sprintf("duplicate node id '%s'", n.ID)})
		}
		if names[n.Name] {
			errs = append(errs, ValidationError{Path: path, NodeID: n.ID, Message: fmt.Sprintf("duplicate node name '%s'", n.Name)})
		}
		if err, bad := engineVersionError(p, n, i); bad {
			errs = append(errs, err)
		}
		byID[n.ID] = n
		names[n.Name] = true
	}

	for i := range p.Nodes {
		if p.Nodes[i].Type != NodeGateway {
			continue
		}
		gateways++
		if gateways > 1 {
			errs = append(errs, ValidationError{Path: "nodes", NodeID: p.Nodes[i].ID, Message: "a project can have at most one gateway"})
		}
	}

	seen := make(map[string]bool, len(p.Edges))
	edgeIDs := make(map[string]bool, len(p.Edges))
	for i, e := range p.Edges {
		path := fmt.Sprintf("edges.%d", i)
		if edgeIDs[e.ID] {
			errs = append(errs, ValidationError{Path: path, EdgeID: e.ID, Message: fmt.Sprintf("duplicate edge id '%s'", e.ID)})
		}
		edgeIDs[e.ID] = true

		from, fromOK := byID[e.From]
		to, toOK := byID[e.To]
		if !fromOK {
			errs = append(errs, ValidationError{Path: path, EdgeID: e.ID, Message: fmt.Sprintf("edge refers to missing node '%s'", e.From)})
		}
		if !toOK {
			errs = append(errs, ValidationError{Path: path, EdgeID: e.ID, Message: fmt.Sprintf("edge refers to missing node '%s'", e.To)})
		}
		if !fromOK || !toOK {
			continue
		}
		if from.ID == to.ID {
			errs = append(errs, ValidationError{Path: path, EdgeID: e.ID, Message: "an edge cannot connect a node to itself"})
			continue
		}
		if !slices.Contains(LegalRelations(from.Type, to.Type), e.Relation) {
			errs = append(errs, ValidationError{Path: path, EdgeID: e.ID, Message: fmt.Sprintf("a %s cannot have a '%s' edge to a %s", from.Type, e.Relation, to.Type)})
		}
		key := e.From + "|" + e.To + "|" + string(e.Relation)
		if seen[key] {
			errs = append(errs, ValidationError{Path: path, EdgeID: e.ID, Message: fmt.Sprintf("duplicate '%s' edge from '%s' to '%s'", e.Relation, from.Name, to.Name)})
		}
		seen[key] = true
	}

	return errs
}
