package resolve

import (
	"fmt"

	"togen/internal/ir"
)

func Run(p *ir.Project, prov Provider) (*ir.Graph, error) {
	project, err := ir.ApplyDefaults(p)
	if err != nil {
		return nil, &ResolveError{Errors: ir.Errors{{Message: err.Error()}}}
	}
	if project.Provider != prov.Name() {
		return nil, &ResolveError{Errors: ir.Errors{{
			Path:    "provider",
			Message: fmt.Sprintf("the %s resolver cannot resolve a '%s' project", prov.Name(), project.Provider),
		}}}
	}

	ctx := NewContext(project)
	if failure := resolveAll(ctx, prov); failure != nil {
		return nil, failure
	}
	checkNameLimits(ctx, prov.NameLimits())
	if len(ctx.Errors) > 0 {
		return nil, &ResolveError{Errors: ctx.Errors}
	}

	g := &ir.Graph{
		TerraformVersion: ">= 1.5",
		Provider:         prov.ProviderBlock(project),
		Data:             ctx.DataSources(),
		Resources:        ctx.Resources(),
		Variables:        ctx.Variables,
		Outputs:          ctx.Outputs,
	}
	if errs := ir.ValidateGraph(g); len(errs) > 0 {
		return nil, &ResolveError{Errors: errs}
	}
	return g, nil
}

func resolveAll(ctx *Context, prov Provider) (failure *ResolveError) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		fail, ok := r.(internalError)
		if !ok {
			panic(r)
		}
		failure = &ResolveError{Errors: ir.Errors{fail.err}}
	}()

	for _, n := range ctx.Project.Nodes {
		if h, ok := prov.ResolveNode(ctx, n); ok {
			ctx.SetHandle(n.ID, h)
		}
	}
	// A missing handle means the node at that end already reported why it could not resolve.
	for _, e := range ctx.Project.Edges {
		from, hasFrom := ctx.Handle(e.From)
		to, hasTo := ctx.Handle(e.To)
		if !hasFrom || !hasTo {
			continue
		}
		prov.ResolveEdge(ctx, e, from, to)
	}
	for _, n := range ctx.Project.Nodes {
		if h, ok := ctx.Handle(n.ID); ok && h.Finalise != nil {
			h.Finalise()
		}
	}
	return nil
}

func checkNameLimits(ctx *Context, limits []NameLimit) {
	for _, r := range ctx.Resources() {
		for _, limit := range limits {
			if r.Type != limit.Type {
				continue
			}
			v, ok := r.Args.Get(limit.Arg)
			if !ok {
				continue
			}
			s, ok := v.(ir.String)
			if !ok || len(s) <= limit.Max {
				continue
			}
			ctx.Report(ir.ValidationError{
				NodeID: r.SourceNode,
				Message: fmt.Sprintf(
					"%s name '%s' is %d characters, the limit is %d. Shorten the project, environment or node name.",
					r.Type, s, len(s), limit.Max),
			})
		}
	}
}
