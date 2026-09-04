package ir

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

func PropsFor(t NodeType) (any, bool) {
	switch t {
	case NodeService:
		return &ServiceProps{}, true
	case NodeFunction:
		return &FunctionProps{}, true
	case NodeDatabase:
		return &DatabaseProps{}, true
	case NodeGateway:
		return &GatewayProps{}, true
	case NodeQueue:
		return &QueueProps{}, true
	case NodeBucket:
		return &BucketProps{}, true
	case NodeCache:
		return &CacheProps{}, true
	}
	return nil, false
}

func ApplyDefaults(p *Project) (*Project, error) {
	out := *p
	out.Nodes = make([]Node, len(p.Nodes))
	for i, n := range p.Nodes {
		props, ok := PropsFor(n.Type)
		if !ok {
			return nil, fmt.Errorf("node %s: unknown type %q", n.ID, n.Type)
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
