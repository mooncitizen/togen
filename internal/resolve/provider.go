package resolve

import "togen/internal/ir"

type NameLimit struct {
	Type string
	Arg  string
	Max  int
}

type Provider interface {
	Name() ir.CloudProvider
	ProviderBlock(p *ir.Project) ir.Provider
	NameLimits() []NameLimit
	ResolveNode(ctx *Context, n ir.Node) (*Handle, bool)
	ResolveEdge(ctx *Context, e ir.Edge, from, to *Handle)
}
