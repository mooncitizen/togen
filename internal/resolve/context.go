package resolve

import (
	"fmt"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
)

type Context struct {
	Project   *ir.Project
	Variables []ir.Variable
	Outputs   []ir.Output
	Errors    ir.Errors
	Scratch   map[string]any

	// Pointers, because a node resolver keeps one and mutates Args at finalise.
	resources []*ir.Resource
	data      []*ir.DataSource
	declared  map[string]bool
	handles   map[string]*Handle
}

func NewContext(p *ir.Project) *Context {
	return &Context{
		Project:  p,
		Scratch:  map[string]any{},
		declared: map[string]bool{},
		handles:  map[string]*Handle{},
	}
}

func (c *Context) Prefix() string { return c.Project.Name + "-" + c.Project.Environment }

func (c *Context) Named(node string) string { return c.Prefix() + "-" + node }

func (c *Context) Local(node string) string { return strings.ReplaceAll(node, "-", "_") }

func (c *Context) Add(r ir.Resource) *ir.Resource {
	c.declare("resource", ir.ID{Type: r.Type, Name: r.Name})
	c.resources = append(c.resources, &r)
	return &r
}

func (c *Context) AddData(d ir.DataSource) *ir.DataSource {
	c.declare("data", ir.ID{Type: d.Type, Name: d.Name})
	c.data = append(c.data, &d)
	return &d
}

func (c *Context) declare(kind string, id ir.ID) {
	key := kind + "." + id.String()
	if c.declared[key] {
		c.Fail(fmt.Sprintf("%s '%s' was produced twice", kind, id))
	}
	c.declared[key] = true
}

func (c *Context) AddVariable(v ir.Variable) { c.Variables = append(c.Variables, v) }

func (c *Context) AddOutput(o ir.Output) { c.Outputs = append(c.Outputs, o) }

func (c *Context) Report(err ir.ValidationError) { c.Errors = append(c.Errors, err) }

func (c *Context) Fail(msg string) {
	panic(internalError{err: ir.ValidationError{Message: msg}})
}

func (c *Context) Handle(nodeID string) (*Handle, bool) {
	h, ok := c.handles[nodeID]
	return h, ok
}

func (c *Context) SetHandle(nodeID string, h *Handle) { c.handles[nodeID] = h }

func (c *Context) HasResource(id ir.ID) bool {
	return c.declared["resource."+id.String()]
}

func (c *Context) Resource(id ir.ID) (*ir.Resource, bool) {
	for _, r := range c.resources {
		if r.Type == id.Type && r.Name == id.Name {
			return r, true
		}
	}
	return nil, false
}

func (c *Context) Resources() []ir.Resource {
	out := make([]ir.Resource, len(c.resources))
	for i, r := range c.resources {
		out[i] = *r
	}
	return out
}

func (c *Context) DataSources() []ir.DataSource {
	out := make([]ir.DataSource, len(c.data))
	for i, d := range c.data {
		out[i] = *d
	}
	return out
}
