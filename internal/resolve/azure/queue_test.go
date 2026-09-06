package azure

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/emit/hcl"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func queueNode(t *testing.T, id, name string, p ir.QueueProps) ir.Node {
	t.Helper()
	return ir.Node{ID: id, Type: ir.NodeQueue, Name: name, Properties: props(t, p)}
}

func setupQueue(t *testing.T, p ir.QueueProps) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx := newContext(t, []ir.Node{queueNode(t, "n5", "jobs", p)})
	return ctx, resolveQueue(ctx, ctx.Project.Nodes[0])
}

var defaultQueue = ir.QueueProps{DeadLetter: true, RetentionDays: 4}

var (
	namespaceID = ir.ID{Type: "azurerm_servicebus_namespace", Name: "main"}
	queueID     = ir.ID{Type: "azurerm_servicebus_queue", Name: "jobs"}
	busFQDN     = ir.C(ir.R(namespaceID, ir.Field("name")), ir.Str(".servicebus.windows.net"))
)

func queueArgs(args ...ir.Attr) ir.Attrs {
	return append(ir.Attrs{
		ir.A("name", ir.Str("shop-dev-jobs")),
		ir.A("namespace_id", ir.R(namespaceID, ir.Field("id"))),
	}, args...)
}

func TestQueueEmitsAQueueInABasicNamespace(t *testing.T) {
	ctx, handle := setupQueue(t, defaultQueue)
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}

	namespace := firstOfType(t, ctx, "azurerm_servicebus_namespace")
	if diff := cmp.Diff(located("shop-dev-bus", ir.A("sku", ir.Str("Basic"))), namespace.Args); diff != "" {
		t.Errorf("namespace args (-want +got):\n%s", diff)
	}
	if namespace.SourceNode != "" || namespace.SourceLabel != "service bus namespace" {
		t.Errorf("namespace source = %q/%q", namespace.SourceNode, namespace.SourceLabel)
	}

	queue := firstOfType(t, ctx, "azurerm_servicebus_queue")
	want := queueArgs(
		ir.A("default_message_ttl", ir.Str("P4D")),
		ir.A("dead_lettering_on_message_expiration", ir.Bool(true)),
		ir.A("max_delivery_count", ir.Num(5)),
	)
	if diff := cmp.Diff(want, queue.Args); diff != "" {
		t.Errorf("queue args (-want +got):\n%s", diff)
	}
	if queue.SourceNode != "n5" || queue.SourceLabel != "jobs" {
		t.Errorf("queue source = %q/%q", queue.SourceNode, queue.SourceLabel)
	}
	// The group, the namespace and the queue.
	if got := len(ctx.Resources()); got != 3 {
		t.Errorf("resources = %d", got)
	}
	if got := countOfType(ctx, "azurerm_virtual_network"); got != 0 {
		t.Error("a queue pulled in the network")
	}

	wantExports := resolve.QueueExports{
		Name:      ir.R(queueID, ir.Field("name")),
		Scope:     ir.R(queueID, ir.Field("id")),
		Namespace: busFQDN,
	}
	if diff := cmp.Diff(resolve.Exports(wantExports), handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if handle.Primary != queueID {
		t.Errorf("primary = %s", handle.Primary)
	}
	wantOutputs := []ir.Output{{
		Name:        "servicebus_namespace",
		Description: "Fully qualified Service Bus namespace the queues live in",
		Value:       busFQDN,
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
}

func TestQueueWithoutDeadLetterKeepsExpiredMessagesOutOfTheDeadLetterQueue(t *testing.T) {
	// deadLetter defaults to true, so it has to be turned off in the raw properties rather
	// than through QueueProps, where omitempty would drop the false.
	ctx := newContext(t, []ir.Node{{
		ID:         "n5",
		Type:       ir.NodeQueue,
		Name:       "jobs",
		Properties: json.RawMessage(`{"deadLetter":false}`),
	}})
	resolveQueue(ctx, ctx.Project.Nodes[0])

	want := queueArgs(
		ir.A("default_message_ttl", ir.Str("P4D")),
		ir.A("dead_lettering_on_message_expiration", ir.Bool(false)),
	)
	if diff := cmp.Diff(want, firstOfType(t, ctx, "azurerm_servicebus_queue").Args); diff != "" {
		t.Errorf("queue args (-want +got):\n%s", diff)
	}
}

func TestQueueFIFORequiresSessionsOnAStandardNamespace(t *testing.T) {
	p := defaultQueue
	p.FIFO = true
	ctx, handle := setupQueue(t, p)

	want := queueArgs(
		ir.A("default_message_ttl", ir.Str("P4D")),
		ir.A("dead_lettering_on_message_expiration", ir.Bool(true)),
		ir.A("max_delivery_count", ir.Num(5)),
		ir.A("requires_session", ir.Bool(true)),
	)
	if diff := cmp.Diff(want, firstOfType(t, ctx, "azurerm_servicebus_queue").Args); diff != "" {
		t.Errorf("queue args (-want +got):\n%s", diff)
	}
	sku, _ := firstOfType(t, ctx, "azurerm_servicebus_namespace").Args.Get("sku")
	if diff := cmp.Diff(ir.Value(ir.Str("Standard")), sku); diff != "" {
		t.Errorf("namespace sku (-want +got):\n%s", diff)
	}
	if !handle.Exports.(resolve.QueueExports).FIFO {
		t.Error("the exports do not say the queue is fifo")
	}
}

func TestQueueRetentionIsAnISODurationInDays(t *testing.T) {
	for _, tc := range []struct {
		days int
		want string
	}{
		{1, "P1D"},
		{14, "P14D"},
	} {
		p := defaultQueue
		p.RetentionDays = tc.days
		ctx, _ := setupQueue(t, p)
		ttl, _ := firstOfType(t, ctx, "azurerm_servicebus_queue").Args.Get("default_message_ttl")
		if diff := cmp.Diff(ir.Value(ir.Str(tc.want)), ttl); diff != "" {
			t.Errorf("%d days ttl (-want +got):\n%s", tc.days, diff)
		}
	}
}

func TestTwoQueuesShareOneNamespaceWhichAFIFOQueueMakesStandard(t *testing.T) {
	fifo := defaultQueue
	fifo.FIFO = true
	p := newProject(t, []ir.Node{
		queueNode(t, "n5", "jobs", defaultQueue),
		queueNode(t, "n6", "events", fifo),
	})
	g, err := resolve.Run(p, New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if errs := ir.ValidateGraph(g); len(errs) > 0 {
		t.Errorf("graph is not valid:\n%s", errs.Error())
	}
	var types []string
	for _, r := range g.Resources {
		types = append(types, r.Type)
	}
	want := []string{
		"azurerm_resource_group",
		"azurerm_servicebus_namespace",
		"azurerm_servicebus_queue",
		"azurerm_servicebus_queue",
	}
	if diff := cmp.Diff(want, types); diff != "" {
		t.Errorf("resources (-want +got):\n%s", diff)
	}
	if len(g.Outputs) != 1 || g.Outputs[0].Name != "servicebus_namespace" {
		t.Errorf("outputs = %+v", g.Outputs)
	}

	files, err := hcl.Emit(g)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	main := unaligned(files["main.tf"])
	for _, want := range []string{
		`resource "azurerm_servicebus_namespace" "main" { name = "shop-dev-bus"`,
		`sku = "Standard"`,
		`resource "azurerm_servicebus_queue" "events" { name = "shop-dev-events" namespace_id = azurerm_servicebus_namespace.main.id default_message_ttl = "P4D" dead_lettering_on_message_expiration = true max_delivery_count = 5 requires_session = true }`,
	} {
		if !strings.Contains(main, want) {
			t.Errorf("main.tf lacks %s:\n%s", want, files["main.tf"])
		}
	}
	wantOutput := `"${azurerm_servicebus_namespace.main.name}.servicebus.windows.net"`
	if !strings.Contains(string(files["outputs.tf"]), wantOutput) {
		t.Errorf("outputs.tf:\n%s", files["outputs.tf"])
	}
}

func TestQueuesAloneStayOnTheBasicNamespace(t *testing.T) {
	p := newProject(t, []ir.Node{
		queueNode(t, "n5", "jobs", defaultQueue),
		queueNode(t, "n6", "events", defaultQueue),
	})
	g, err := resolve.Run(p, New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, r := range g.Resources {
		if r.Type != "azurerm_servicebus_namespace" {
			continue
		}
		sku, _ := r.Args.Get("sku")
		if diff := cmp.Diff(ir.Value(ir.Str("Basic")), sku); diff != "" {
			t.Errorf("namespace sku (-want +got):\n%s", diff)
		}
	}
}
