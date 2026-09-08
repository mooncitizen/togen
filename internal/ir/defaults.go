package ir

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// The types whose properties are a Go struct, which is every type a resolver generates
// from. A draw-only catalogue entry declares its properties as JSON Schema instead.
var propsStructs = map[NodeType]func() any{
	NodeService:  func() any { return &ServiceProps{} },
	NodeFunction: func() any { return &FunctionProps{} },
	NodeDatabase: func() any { return &DatabaseProps{} },
	NodeGateway:  func() any { return &GatewayProps{} },
	NodeQueue:    func() any { return &QueueProps{} },
	NodeBucket:   func() any { return &BucketProps{} },
	NodeCache:    func() any { return &CacheProps{} },
}

func PropsFor(t NodeType) (any, bool) {
	make, ok := propsStructs[t]
	if !ok {
		return nil, false
	}
	return make(), true
}

func TypesWithProps() []NodeType {
	out := make([]NodeType, 0, len(propsStructs))
	for t := range propsStructs {
		out = append(out, t)
	}
	slices.Sort(out)
	return out
}

func ApplyDefaults(p *Project) (*Project, error) {
	out := *p
	out.Nodes = make([]Node, len(p.Nodes))
	for i, n := range p.Nodes {
		// A draw-only type has no struct to default from, so its properties stand as
		// written. The schema rejects a type no provider has before this runs.
		props, ok := PropsFor(n.Type)
		if !ok {
			out.Nodes[i] = n
			continue
		}
		if err := setTagDefaults(props); err != nil {
			return nil, err
		}
		if len(n.Properties) > 0 {
			if err := json.Unmarshal(n.Properties, props); err != nil {
				return nil, fmt.Errorf("node %s: %w", n.ID, err)
			}
		}
		fillEnv(props)
		raw, err := marshalEveryField(props)
		if err != nil {
			return nil, err
		}
		n.Properties = raw
		out.Nodes[i] = n
	}
	out.Edges = make([]Edge, len(p.Edges))
	for i, e := range p.Edges {
		if e.Relation == RelRoutes {
			if e.Properties.Path == "" {
				e.Properties.Path = "/"
			}
			if len(e.Properties.Methods) == 0 {
				e.Properties.Methods = []Method{MethodAny}
			}
		}
		out.Edges[i] = e
	}
	return &out, nil
}

func NodeProps[T any](n Node) (T, error) {
	var v T
	if len(n.Properties) > 0 {
		if err := json.Unmarshal(n.Properties, &v); err != nil {
			return v, err
		}
	}
	fillEnv(&v)
	return v, nil
}

// Properties are written out in full rather than through omitempty, so applying defaults twice
// gives the same answer. A property like versioning, false against a default of true, would
// otherwise vanish on the way out and come back as the default on the next pass.
func marshalEveryField(ptr any) ([]byte, error) {
	v := reflect.ValueOf(ptr).Elem()
	t := v.Type()
	out := make(map[string]any, t.NumField())
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		switch name {
		case "-":
			continue
		case "":
			name = f.Name
		}
		out[name] = v.Field(i).Interface()
	}
	return json.Marshal(out)
}

func setTagDefaults(ptr any) error {
	v := reflect.ValueOf(ptr).Elem()
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		def, ok := tagValue(f.Tag.Get("jsonschema"), "default")
		if !ok {
			continue
		}
		fv := v.Field(i)
		switch fv.Kind() {
		case reflect.String:
			fv.SetString(def)
		case reflect.Int:
			n, err := strconv.Atoi(def)
			if err != nil {
				return fmt.Errorf("%s.%s: default %q is not a whole number", t.Name(), f.Name, def)
			}
			fv.SetInt(int64(n))
		case reflect.Bool:
			b, err := strconv.ParseBool(def)
			if err != nil {
				return fmt.Errorf("%s.%s: default %q is not a boolean", t.Name(), f.Name, def)
			}
			fv.SetBool(b)
		case reflect.Pointer:
			if fv.Type().Elem().Kind() != reflect.Int {
				return fmt.Errorf("%s.%s: no default handling for %s", t.Name(), f.Name, fv.Type())
			}
			n, err := strconv.Atoi(def)
			if err != nil {
				return fmt.Errorf("%s.%s: default %q is not a whole number", t.Name(), f.Name, def)
			}
			fv.Set(reflect.ValueOf(&n))
		}
	}
	return nil
}

func fillEnv(ptr any) {
	v := reflect.ValueOf(ptr).Elem()
	if v.Kind() != reflect.Struct {
		return
	}
	f := v.FieldByName("Env")
	if f.IsValid() && f.Kind() == reflect.Map && f.IsNil() {
		f.Set(reflect.MakeMap(f.Type()))
	}
}

func tagValue(tag, key string) (string, bool) {
	for _, part := range strings.Split(tag, ",") {
		if k, val, ok := strings.Cut(part, "="); ok && k == key {
			return val, true
		}
	}
	return "", false
}
