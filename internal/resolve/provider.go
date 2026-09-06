package resolve

import "github.com/mooncitizen/togen/internal/ir"

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

// ProviderBlock has no context to declare through, so a provider whose block reads a
// variable declares it here and Run adds it ahead of anything the nodes declare.
type VariableProvider interface {
	Variables(p *ir.Project) []ir.Variable
}
