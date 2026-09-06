package azure

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func resolveDataAccess(ctx *resolve.Context, edge ir.Edge, from, to *resolve.Handle) {
	switch from.Exports.(type) {
	case resolve.FunctionExports, resolve.ServiceExports:
	default:
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s from a %s is not supported by the azure resolver yet", edge.Relation, from.Node.Type),
		})
		return
	}
	switch target := to.Exports.(type) {
	case resolve.DatabaseExports:
		connectDatabase(ctx, from, to, target)
	case resolve.BucketExports:
		grantBucket(ctx, edge, from, to, target)
	case resolve.CacheExports:
		connectCache(ctx, from, to, target)
	default:
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s to a %s is not supported by the azure resolver yet", edge.Relation, to.Node.Type),
		})
	}
}

// The server answers only from inside the virtual network, so the caller is marked as needing
// it and its own resolver decides what that means: a Container App joins, a function app on
// the consumption plan cannot and gets the settings all the same.
func connectDatabase(ctx *resolve.Context, from, to *resolve.Handle, target resolve.DatabaseExports) {
	port, ok := target.Port.(ir.Number)
	if !ok {
		ctx.Fail(fmt.Sprintf("database '%s' exports a port that is not a number", to.Node.Name))
	}
	from.NeedsNetwork = true
	prefix := strings.ToUpper(ctx.Local(to.Node.Name))
	from.SetEnv(prefix+"_HOST", target.Host)
	from.SetEnv(prefix+"_PORT", ir.Str(strconv.FormatFloat(float64(port), 'f', -1, 64)))
	from.SetEnv(prefix+"_NAME", target.Name)
	from.SetEnv(prefix+"_USER", target.User)
	from.SetEnv(prefix+"_PASSWORD", target.Password)
}

// The cache answers on its public endpoint over TLS, so nothing joins the network. The port
// is the ssl_port attribute rather than a literal, which is why it is not a string here.
func connectCache(ctx *resolve.Context, from, to *resolve.Handle, target resolve.CacheExports) {
	prefix := strings.ToUpper(ctx.Local(to.Node.Name))
	from.SetEnv(prefix+"_HOST", target.Host)
	from.SetEnv(prefix+"_PORT", target.Port)
	from.SetEnv(prefix+"_PASSWORD", target.Password)
}

const (
	blobReaderRole      = "Storage Blob Data Reader"
	blobContributorRole = "Storage Blob Data Contributor"
)

// Blobs are reached over the account's public endpoint with the caller's managed identity, so
// an edge is a role assignment and the names the caller addresses. The account is the scope:
// it holds this one container and nothing else, and the account id has the same shape in every
// provider version where the container's has not.
func grantBucket(ctx *resolve.Context, edge ir.Edge, from, to *resolve.Handle, target resolve.BucketExports) {
	var principal ir.Value
	switch caller := from.Exports.(type) {
	case resolve.FunctionExports:
		principal = caller.PrincipalID
	case resolve.ServiceExports:
		principal = caller.PrincipalID
	}
	if edge.Relation == ir.RelReads {
		assignRole(ctx, from, to, "read", blobReaderRole, target.Scope, principal)
	} else {
		assignRole(ctx, from, to, "write", blobContributorRole, target.Scope, principal)
	}
	prefix := strings.ToUpper(ctx.Local(to.Node.Name))
	from.SetEnv(prefix+"_ACCOUNT", target.Account)
	from.SetEnv(prefix+"_CONTAINER", target.Container)
}
