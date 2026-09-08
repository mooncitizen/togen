package ir

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestApplyDefaultsFillsNodeAndEdgeProperties(t *testing.T) {
	p := &Project{Version: 1, Name: "shop", Provider: ProviderAWS, Region: "eu-west-2", Environment: "dev",
		Nodes: []Node{
			{ID: "n1", Type: NodeGateway, Name: "api"},
			{ID: "n2", Type: NodeFunction, Name: "handler", Properties: json.RawMessage(`{"runtime":"python"}`)},
			{ID: "n3", Type: NodeDatabase, Name: "main-db"},
		},
		Edges: []Edge{
			{ID: "e1", From: "n1", To: "n2", Relation: RelRoutes, Properties: EdgeProperties{Path: "/users"}},
			{ID: "e2", From: "n2", To: "n3", Relation: RelReads},
		}}
	out, err := ApplyDefaults(p)
	if err != nil {
		t.Fatal(err)
	}
	fn, err := NodeProps[FunctionProps](out.Nodes[1])
	if err != nil {
		t.Fatal(err)
	}
	want := FunctionProps{Runtime: "python", Handler: "index.handler", Size: SizeSmall, TimeoutSeconds: 30, Env: map[string]string{}}
	if d := cmp.Diff(want, fn); d != "" {
		t.Fatal(d)
	}
	db, err := NodeProps[DatabaseProps](out.Nodes[2])
	if err != nil {
		t.Fatal(err)
	}
	if db.Engine != EnginePostgres || db.Version != "" || db.StorageGB != 20 {
		t.Fatalf("db defaults wrong: %+v", db)
	}
	if d := cmp.Diff(EdgeProperties{Path: "/users", Methods: []Method{MethodAny}}, out.Edges[0].Properties); d != "" {
		t.Fatal(d)
	}
	if d := cmp.Diff(EdgeProperties{}, out.Edges[1].Properties); d != "" {
		t.Fatal(d)
	}
	if string(p.Nodes[1].Properties) != `{"runtime":"python"}` {
		t.Fatal("input mutated")
	}
}

func TestApplyDefaultsKeepsAFalseThatOverridesATrueDefault(t *testing.T) {
	p := &Project{Nodes: []Node{
		{ID: "b1", Type: NodeBucket, Name: "uploads", Properties: json.RawMessage(`{"versioning":false}`)},
		{ID: "q1", Type: NodeQueue, Name: "jobs", Properties: json.RawMessage(`{"deadLetter":false}`)},
	}}
	// The CLI applies defaults on the way out of validation and the resolver applies them
	// again, so a second pass has to give the same answer as the first.
	out, err := ApplyDefaults(p)
	if err != nil {
		t.Fatal(err)
	}
	if out, err = ApplyDefaults(out); err != nil {
		t.Fatal(err)
	}
	bucket, err := NodeProps[BucketProps](out.Nodes[0])
	if err != nil {
		t.Fatal(err)
	}
	if bucket.Versioning {
		t.Error("versioning came back on")
	}
	queue, err := NodeProps[QueueProps](out.Nodes[1])
	if err != nil {
		t.Fatal(err)
	}
	if queue.DeadLetter {
		t.Error("the dead letter queue came back on")
	}
	if queue.RetentionDays != 4 {
		t.Errorf("retentionDays = %d", queue.RetentionDays)
	}
}

// A type with no props struct is a draw-only catalogue entry, whose properties are the
// user's to write and the schema's to check. Defaults leave them alone.
func TestApplyDefaultsLeavesATypeWithNoPropsStructAlone(t *testing.T) {
	raw := json.RawMessage(`{"billing":"on-demand"}`)
	p := &Project{Nodes: []Node{{ID: "n1", Type: "aws/table", Name: "big", Properties: raw}}}
	out, err := ApplyDefaults(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(out.Nodes[0].Properties) != string(raw) {
		t.Errorf("properties = %s", out.Nodes[0].Properties)
	}
}

func TestApplyDefaultsOnlyFillsRouteProperties(t *testing.T) {
	p := &Project{Version: 1, Name: "shop", Provider: ProviderAWS, Region: "eu-west-2", Environment: "dev",
		Edges: []Edge{
			{ID: "e1", From: "n1", To: "n2", Relation: RelRoutes},
			{ID: "e2", From: "n2", To: "n3", Relation: RelWrites},
			{ID: "e3", From: "n2", To: "n4", Relation: RelPublishes},
		}}
	out, err := ApplyDefaults(p)
	if err != nil {
		t.Fatal(err)
	}
	if d := cmp.Diff(EdgeProperties{Path: "/", Methods: []Method{MethodAny}}, out.Edges[0].Properties); d != "" {
		t.Fatal(d)
	}
	for _, e := range out.Edges[1:] {
		if d := cmp.Diff(EdgeProperties{}, e.Properties); d != "" {
			t.Errorf("%s got route defaults:\n%s", e.ID, d)
		}
	}
}

func TestPropsForEveryNodeType(t *testing.T) {
	for _, nt := range NodeTypes {
		if _, ok := PropsFor(nt); !ok {
			t.Errorf("no props struct for %q", nt)
		}
	}
}

func TestEveryPropsDefaultParses(t *testing.T) {
	for _, nt := range NodeTypes {
		props, ok := PropsFor(nt)
		if !ok {
			t.Fatalf("no props struct for %q", nt)
		}
		if err := setTagDefaults(props); err != nil {
			t.Errorf("%s: %v", nt, err)
		}
	}
}

func TestApplyDefaultsKeepsAZeroMinReplicas(t *testing.T) {
	p := &Project{Nodes: []Node{
		{ID: "s1", Type: NodeService, Name: "web", Properties: json.RawMessage(`{"image":"nginx","minReplicas":0}`)},
		{ID: "s2", Type: NodeService, Name: "api", Properties: json.RawMessage(`{"image":"nginx"}`)},
	}}
	out, err := ApplyDefaults(p)
	if err != nil {
		t.Fatal(err)
	}
	if out, err = ApplyDefaults(out); err != nil {
		t.Fatal(err)
	}
	web, err := NodeProps[ServiceProps](out.Nodes[0])
	if err != nil {
		t.Fatal(err)
	}
	if web.MinReplicas == nil || *web.MinReplicas != 0 {
		t.Errorf("minReplicas = %v, want 0", web.MinReplicas)
	}
	api, err := NodeProps[ServiceProps](out.Nodes[1])
	if err != nil {
		t.Fatal(err)
	}
	if api.MinReplicas == nil || *api.MinReplicas != 1 {
		t.Errorf("minReplicas = %v, want the default of 1", api.MinReplicas)
	}
}

func TestServicePropsWriteAZeroAndOmitAnUnsetMinReplicas(t *testing.T) {
	for _, c := range []struct {
		props ServiceProps
		want  string
	}{
		{ServiceProps{Image: "nginx"}, `{"image":"nginx"}`},
		{ServiceProps{Image: "nginx", MinReplicas: Ptr(0)}, `{"image":"nginx","minReplicas":0}`},
	} {
		raw, err := json.Marshal(c.props)
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != c.want {
			t.Errorf("got %s, want %s", raw, c.want)
		}
	}
}
