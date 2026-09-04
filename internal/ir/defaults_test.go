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

func TestApplyDefaultsRejectsUnknownNodeType(t *testing.T) {
	p := &Project{Nodes: []Node{{ID: "n1", Type: "mainframe", Name: "big"}}}
	if _, err := ApplyDefaults(p); err == nil {
		t.Fatal("want an error for an unknown node type")
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
