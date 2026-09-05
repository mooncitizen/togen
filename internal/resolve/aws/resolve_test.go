package aws

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func example() *ir.Project {
	return &ir.Project{
		Version:     1,
		Name:        "shop",
		Provider:    ir.ProviderAWS,
		Region:      "eu-west-2",
		Environment: "dev",
		Nodes: []ir.Node{
			{ID: "n1", Type: ir.NodeGateway, Name: "api"},
			{ID: "n2", Type: ir.NodeFunction, Name: "handler"},
			{ID: "n3", Type: ir.NodeDatabase, Name: "main-db"},
			{
				ID:         "n4",
				Type:       ir.NodeService,
				Name:       "web",
				Properties: json.RawMessage(`{"image":"nginx:1.27","port":80,"public":true}`),
			},
			{ID: "n5", Type: ir.NodeQueue, Name: "jobs"},
			{ID: "n6", Type: ir.NodeFunction, Name: "worker"},
			{ID: "n7", Type: ir.NodeBucket, Name: "uploads"},
			{ID: "n8", Type: ir.NodeCache, Name: "sessions"},
		},
		Edges: []ir.Edge{
			{ID: "e1", From: "n1", To: "n2", Relation: ir.RelRoutes},
			{ID: "e2", From: "n2", To: "n3", Relation: ir.RelReads},
			{ID: "e3", From: "n4", To: "n3", Relation: ir.RelReads},
			{ID: "e4", From: "n2", To: "n5", Relation: ir.RelPublishes},
			{ID: "e5", From: "n6", To: "n5", Relation: ir.RelConsumes},
			{ID: "e6", From: "n2", To: "n7", Relation: ir.RelWrites},
			{ID: "e7", From: "n6", To: "n7", Relation: ir.RelReads},
			{ID: "e8", From: "n4", To: "n8", Relation: ir.RelReads},
			{ID: "e9", From: "n4", To: "n2", Relation: ir.RelCalls},
			{ID: "e10", From: "n6", To: "n4", Relation: ir.RelCalls},
			{
				ID: "e11", From: "n1", To: "n4", Relation: ir.RelRoutes,
				Properties: ir.EdgeProperties{Path: "/web", Methods: []ir.Method{ir.MethodGet}},
			},
		},
	}
}

func runErrors(t *testing.T, p *ir.Project) ir.Errors {
	t.Helper()
	_, err := resolve.Run(p, New())
	if err == nil {
		t.Fatal("want a ResolveError, got nil")
	}
	re, ok := err.(*resolve.ResolveError)
	if !ok {
		t.Fatalf("want a *resolve.ResolveError, got %T: %v", err, err)
	}
	return re.Errors
}

func TestResolveProducesAValidGraphForTheExampleProject(t *testing.T) {
	g, err := resolve.Run(example(), New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if errs := ir.ValidateGraph(g); len(errs) > 0 {
		t.Errorf("graph is not valid:\n%s", errs.Error())
	}
	want := ir.Provider{
		Name:    "aws",
		Source:  "hashicorp/aws",
		Version: "~> 6.0",
		Config:  ir.Attrs{ir.A("region", ir.Str("eu-west-2"))},
	}
	if diff := cmp.Diff(want, g.Provider); diff != "" {
		t.Errorf("provider (-want +got):\n%s", diff)
	}

	var types []string
	for _, r := range g.Resources {
		types = append(types, r.Type)
	}
	for _, want := range []string{
		"aws_vpc",
		"aws_apigatewayv2_api",
		"aws_lambda_function",
		"aws_db_instance",
		"aws_vpc_security_group_ingress_rule",
		"aws_iam_role_policy",
		"aws_ecs_cluster",
		"aws_ecs_service",
		"aws_ecs_task_definition",
		"aws_lb",
		"aws_lb_listener",
		"aws_lb_target_group",
		"aws_sqs_queue",
		"aws_lambda_event_source_mapping",
		"aws_s3_bucket",
		"aws_s3_bucket_versioning",
		"aws_s3_bucket_server_side_encryption_configuration",
		"aws_s3_bucket_public_access_block",
		"aws_elasticache_subnet_group",
		"aws_elasticache_replication_group",
		"aws_service_discovery_private_dns_namespace",
		"aws_service_discovery_service",
		"aws_apigatewayv2_vpc_link",
	} {
		if !slices.Contains(types, want) {
			t.Errorf("missing %s", want)
		}
	}

	i := slices.IndexFunc(g.Resources, func(r ir.Resource) bool { return r.Type == "aws_lambda_function" })
	fn := g.Resources[i]
	if _, ok := fn.Args.Get("vpc_config"); !ok {
		t.Error("the lambda has no vpc_config")
	}
	if _, ok := fn.Args.Get("environment"); !ok {
		t.Error("the lambda has no environment")
	}

	// worker calls web over the private network, so it joins the vpc.
	j := slices.IndexFunc(g.Resources, func(r ir.Resource) bool {
		return r.Type == "aws_lambda_function" && r.Name == "worker"
	})
	if _, ok := g.Resources[j].Args.Get("vpc_config"); !ok {
		t.Error("the worker lambda did not join the vpc to call web")
	}
	k := slices.IndexFunc(g.Resources, func(r ir.Resource) bool { return r.Type == "aws_ecs_service" })
	if _, ok := g.Resources[k].Args.Get("service_registries"); !ok {
		t.Error("the called service was not registered in cloud map")
	}
	for _, r := range g.Resources {
		if r.Type == "aws_vpc_security_group_ingress_rule" && strings.Contains(r.Name, "uploads") {
			t.Errorf("the bucket was given an ingress rule: %s", r.Name)
		}
	}

	var outputs []string
	for _, o := range g.Outputs {
		outputs = append(outputs, o.Name)
	}
	slices.Sort(outputs)
	wantOutputs := []string{
		"api_url", "jobs_url", "main_db_endpoint", "sessions_endpoint", "uploads_bucket", "web_url",
	}
	if diff := cmp.Diff(wantOutputs, outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
	var variables []string
	for _, v := range g.Variables {
		variables = append(variables, v.Name)
	}
	if diff := cmp.Diff([]string{"handler_package", "worker_package"}, variables); diff != "" {
		t.Errorf("variables (-want +got):\n%s", diff)
	}
	if len(g.Data) != 1 || g.Data[0].Type != "aws_availability_zones" {
		t.Errorf("data sources = %+v", g.Data)
	}
}

func TestResolveDoesNotCreateANetworkWhenNothingNeedsOne(t *testing.T) {
	p := example()
	p.Nodes = p.Nodes[:2]
	p.Edges = p.Edges[:1]
	g, err := resolve.Run(p, New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, r := range g.Resources {
		if r.Type == "aws_vpc" {
			t.Fatal("a vpc was created")
		}
	}
	if len(g.Data) != 0 {
		t.Errorf("data sources = %+v", g.Data)
	}
}

func TestResolveRejectsOtherProviders(t *testing.T) {
	p := example()
	p.Provider = ir.ProviderGCP
	errs := runErrors(t, p)
	want := ir.Errors{{Path: "provider", Message: "the aws resolver cannot resolve a 'gcp' project"}}
	if diff := cmp.Diff(want, errs); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

// Every node type in the IR now resolves, so an unsupported one cannot be written into a
// project any more: ApplyDefaults rejects an unknown type before the resolver sees it. The
// report is still reachable through ResolveNode itself, which is where a new type will land.
func TestResolveReportsANodeTypeItDoesNotSupport(t *testing.T) {
	ctx, _ := newContext(t, nil, nil)
	handle, ok := New().ResolveNode(ctx, ir.Node{ID: "n9", Type: "cdn", Name: "edge"})
	if ok || handle != nil {
		t.Fatalf("ResolveNode = %v, %v", handle, ok)
	}
	want := ir.Errors{{
		NodeID:  "n9",
		Message: "node type 'cdn' is not supported by the aws resolver yet",
	}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

// Every relation in the IR now resolves, so the report is reached with one that does not
// exist, which is where a new relation will land before it has a resolver.
func TestResolveReportsEveryUnsupportedEdgeInFileOrder(t *testing.T) {
	p := example()
	p.Nodes = append(p.Nodes, ir.Node{ID: "n10", Type: ir.NodeFunction, Name: "mailer"})
	p.Edges = append(p.Edges,
		ir.Edge{ID: "e11", From: "n2", To: "n10", Relation: ir.Relation("mirrors")},
		ir.Edge{ID: "e12", From: "n6", To: "n10", Relation: ir.Relation("mirrors")},
	)
	errs := runErrors(t, p)
	if len(errs) != 2 {
		t.Fatalf("errors = %v", errs)
	}
	for i, want := range []string{"e11", "e12"} {
		if errs[i].NodeID != "" || errs[i].EdgeID != want {
			t.Errorf("errors[%d] = %+v, want edge %q", i, errs[i], want)
		}
		if !strings.Contains(errs[i].Message, "'mirrors' edges") {
			t.Errorf("errors[%d].Message = %q", i, errs[i].Message)
		}
	}
}

func TestResolveRejectsNamesThatExceedAWSLimits(t *testing.T) {
	p := example()
	p.Name = "a-project-name-that-is-long-too"
	p.Environment = "production-like"
	p.Nodes = []ir.Node{{ID: "n2", Type: ir.NodeFunction, Name: "a-very-long-function-name-that-g"}}
	p.Edges = nil
	errs := runErrors(t, p)
	if len(errs) != 1 {
		t.Fatalf("errors = %v", errs)
	}
	if errs[0].NodeID != "n2" || !strings.Contains(errs[0].Message, "64") {
		t.Errorf("error = %+v", errs[0])
	}
}

func TestResolveRejectsQueueNamesThatExceedTheSQSLimit(t *testing.T) {
	p := example()
	p.Nodes = []ir.Node{{ID: "n5", Type: ir.NodeQueue, Name: strings.Repeat("j", 72)}}
	p.Edges = nil
	errs := runErrors(t, p)
	if len(errs) != 2 {
		t.Fatalf("errors = %v", errs)
	}
	for i, err := range errs {
		if err.NodeID != "n5" || !strings.Contains(err.Message, "80") {
			t.Errorf("errors[%d] = %+v", i, err)
		}
	}
}

func TestResolveRejectsServiceNamesThatExceedTheLoadBalancerLimit(t *testing.T) {
	p := example()
	p.Nodes = []ir.Node{{
		ID:         "n4",
		Type:       ir.NodeService,
		Name:       "a-very-long-public-service-name",
		Properties: json.RawMessage(`{"image":"nginx:1.27","port":80,"public":true}`),
	}}
	p.Edges = nil
	errs := runErrors(t, p)
	if len(errs) != 2 {
		t.Fatalf("errors = %v", errs)
	}
	for i, want := range []string{"aws_lb ", "aws_lb_target_group "} {
		if errs[i].NodeID != "n4" || !strings.Contains(errs[i].Message, "32") {
			t.Errorf("errors[%d] = %+v", i, errs[i])
		}
		if !strings.HasPrefix(errs[i].Message, want) {
			t.Errorf("errors[%d].Message = %q, want it to start with %q", i, errs[i].Message, want)
		}
	}
}
