package ir

import (
	"fmt"
	"slices"
)

func ValidateGraph(g *Graph) Errors {
	v := graphChecker{
		known: map[RefKind]map[string]bool{
			RefResource: {},
			RefData:     {},
		},
	}
	for _, d := range g.Data {
		v.declare(RefData, ID{Type: d.Type, Name: d.Name}, "data source")
	}
	for _, r := range g.Resources {
		v.declare(RefResource, ID{Type: r.Type, Name: r.Name}, "resource")
	}
	for _, d := range g.Data {
		v.walkAttrs(d.Args, "data."+ID{Type: d.Type, Name: d.Name}.String())
	}
	for _, r := range g.Resources {
		id := ID{Type: r.Type, Name: r.Name}
		v.walkAttrs(r.Args, id.String())
		for _, dep := range r.DependsOn {
			if !v.known[RefResource][dep.String()] {
				v.report(id.String(), fmt.Sprintf("depends_on refers to unknown resource '%s'", dep))
			}
		}
	}
	if len(g.Providers) == 0 {
		v.report("providers", "the graph has no providers")
	}
	providers := map[string]bool{}
	for _, p := range g.Providers {
		if providers[p.Name] {
			v.report("provider."+p.Name, fmt.Sprintf("duplicate provider '%s'", p.Name))
		}
		providers[p.Name] = true
		v.walkAttrs(p.Config, "provider."+p.Name)
	}
	for _, variable := range g.Variables {
		v.walk(variable.Default, "variable."+variable.Name)
	}
	for _, o := range g.Outputs {
		v.walk(o.Value, "output."+o.Name)
	}
	return v.errs
}

type graphChecker struct {
	known map[RefKind]map[string]bool
	errs  Errors
}

func (v *graphChecker) report(path, message string) {
	v.errs = append(v.errs, ValidationError{Path: path, Message: message})
}

func (v *graphChecker) declare(kind RefKind, id ID, label string) {
	if v.known[kind][id.String()] {
		v.report(id.String(), fmt.Sprintf("duplicate %s '%s'", label, id))
	}
	v.known[kind][id.String()] = true
}

func (v *graphChecker) walkAttrs(attrs Attrs, path string) {
	for _, a := range attrs {
		v.walk(a.Value, path+"."+a.Key)
	}
}

func (v *graphChecker) walk(value Value, path string) {
	switch x := value.(type) {
	case nil:
	case List:
		for i, item := range x {
			v.walk(item, fmt.Sprintf("%s[%d]", path, i))
		}
	case Map:
		v.walkAttrs(Attrs(x), path)
	case Block:
		for i, entry := range x {
			v.walkAttrs(entry, fmt.Sprintf("%s[%d]", path, i))
		}
	case JSON:
		v.walk(x.Value, path)
	case Concat:
		for i, part := range x.Parts {
			v.walk(part, fmt.Sprintf("%s[%d]", path, i))
		}
	case Call:
		if !slices.Contains(Calls, x.Fn) {
			v.report(path, fmt.Sprintf("unknown function '%s'", x.Fn))
		}
		for i, arg := range x.Args {
			v.walk(arg, fmt.Sprintf("%s[%d]", path, i))
		}
	case Ref:
		if v.known[x.Kind][x.Target.String()] {
			return
		}
		what := "resource"
		if x.Kind == RefData {
			what = "data source"
		}
		v.report(path, fmt.Sprintf("reference to unknown %s '%s'", what, x.Target))
	}
}
