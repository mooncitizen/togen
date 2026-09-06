package gcp

import (
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

// Nothing stands in front of a Cloud Run URL (ADR 0010), so a gateway is only the routes that
// leave it. The handle exists for those routes to find.
func resolveGateway(node ir.Node) *resolve.Handle {
	return &resolve.Handle{Node: node, Exports: resolve.GatewayExports{}}
}
