package azure

import (
	"fmt"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

const (
	namespaceLabel = "service bus namespace"
	namespaceHost  = ".servicebus.windows.net"
	// Service Bus parks a message after this many failed deliveries, as SQS does after five.
	maxDeliveryCount = 5
)

// One namespace per project, made by the first queue. The Basic tier has no sessions, so a
// single fifo queue moves the whole namespace to Standard.
func ensureNamespace(ctx *resolve.Context) ir.ID {
	if id, ok := ctx.Scratch[namespaceLabel].(ir.ID); ok {
		return id
	}
	group := ensureGroup(ctx)
	sku := "Basic"
	for _, n := range ctx.Project.Nodes {
		if n.Type == ir.NodeQueue && resolve.Props[ir.QueueProps](ctx, n).FIFO {
			sku = "Standard"
		}
	}
	r := ctx.Add(ir.Resource{
		Type:        "azurerm_servicebus_namespace",
		Name:        "main",
		SourceLabel: namespaceLabel,
		Args:        group.Located(ctx.Named("bus"), ir.A("sku", ir.Str(sku))),
	})
	id := ir.ID{Type: r.Type, Name: r.Name}
	ctx.Scratch[namespaceLabel] = id
	ctx.AddOutput(ir.Output{
		Name:        "servicebus_namespace",
		Description: "Fully qualified Service Bus namespace the queues live in",
		Value:       namespaceFQDN(id),
	})
	return id
}

func namespaceFQDN(namespace ir.ID) ir.Value {
	return ir.C(ir.R(namespace, ir.Field("name")), ir.Str(namespaceHost))
}

func resolveQueue(ctx *resolve.Context, node ir.Node) *resolve.Handle {
	p := resolve.Props[ir.QueueProps](ctx, node)
	namespace := ensureNamespace(ctx)

	args := ir.Attrs{
		ir.A("name", ir.Str(ctx.Named(node.Name))),
		ir.A("namespace_id", ir.R(namespace, ir.Field("id"))),
		ir.A("default_message_ttl", ir.Str(fmt.Sprintf("P%dD", p.RetentionDays))),
		ir.A("dead_lettering_on_message_expiration", ir.Bool(p.DeadLetter)),
	}
	if p.DeadLetter {
		args = append(args, ir.A("max_delivery_count", ir.Num(maxDeliveryCount)))
	}
	if p.FIFO {
		args = append(args, ir.A("requires_session", ir.Bool(true)))
	}
	queue := ctx.Add(ir.Resource{
		Type:        "azurerm_servicebus_queue",
		Name:        ctx.Local(node.Name),
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args:        args,
	})
	queueID := ir.ID{Type: queue.Type, Name: queue.Name}

	return &resolve.Handle{
		Node:    node,
		Primary: queueID,
		Exports: resolve.QueueExports{
			Name:      ir.R(queueID, ir.Field("name")),
			Scope:     ir.R(queueID, ir.Field("id")),
			Namespace: namespaceFQDN(namespace),
			FIFO:      p.FIFO,
		},
	}
}
