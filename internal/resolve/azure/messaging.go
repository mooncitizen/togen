package azure

import (
	"fmt"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

const (
	senderRole   = "Azure Service Bus Data Sender"
	receiverRole = "Azure Service Bus Data Receiver"
)

// A queue is reached over the public endpoint with the caller's managed identity, so an edge
// is a role assignment on the queue and the settings the caller connects with. A function
// consumes through the Service Bus trigger, whose identity connection is the app setting
// <NAME>__fullyQualifiedNamespace, and a service polls the queue itself.
func resolveMessaging(ctx *resolve.Context, edge ir.Edge, from, to *resolve.Handle) {
	var principal ir.Value
	switch caller := from.Exports.(type) {
	case resolve.FunctionExports:
		principal = caller.PrincipalID
	case resolve.ServiceExports:
		principal = caller.PrincipalID
	default:
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s from a %s is not supported by the azure resolver yet", edge.Relation, from.Node.Type),
		})
		return
	}
	queue, ok := to.Exports.(resolve.QueueExports)
	if !ok {
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s to a %s is not supported by the azure resolver yet", edge.Relation, to.Node.Type),
		})
		return
	}
	prefix := strings.ToUpper(ctx.Local(to.Node.Name))

	if edge.Relation == ir.RelPublishes {
		assignRole(ctx, from, to, "send", senderRole, queue.Scope, principal)
		from.SetEnv(prefix+"_QUEUE", queue.Name)
		from.SetEnv(prefix+"_NAMESPACE", queue.Namespace)
		return
	}

	assignRole(ctx, from, to, "receive", receiverRole, queue.Scope, principal)
	if _, isFunction := from.Exports.(resolve.FunctionExports); isFunction {
		from.SetEnv(prefix+"__fullyQualifiedNamespace", queue.Namespace)
		from.SetEnv(prefix+"_QUEUE", queue.Name)
		return
	}
	from.SetEnv(prefix+"_QUEUE", queue.Name)
	from.SetEnv(prefix+"_NAMESPACE", queue.Namespace)
}

func assignRole(ctx *resolve.Context, from, to *resolve.Handle, verb, role string, scope, principal ir.Value) {
	id := ir.ID{
		Type: "azurerm_role_assignment",
		Name: ctx.Local(from.Node.Name) + "_" + ctx.Local(to.Node.Name) + "_" + verb,
	}
	if ctx.HasResource(id) {
		return
	}
	ctx.Add(ir.Resource{
		Type:        id.Type,
		Name:        id.Name,
		SourceNode:  from.Node.ID,
		SourceLabel: from.Node.Name,
		Args: ir.Attrs{
			ir.A("scope", scope),
			ir.A("role_definition_name", ir.Str(role)),
			ir.A("principal_id", principal),
			// Named so the assignment does not wait on directory replication of a fresh identity.
			ir.A("principal_type", ir.Str("ServicePrincipal")),
		},
	})
}
