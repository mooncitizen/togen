package ir

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestRelationRulesHandsOutFreshSlices(t *testing.T) {
	first := RelationRules()[RelReads]
	first.To[0] = NodeQueue
	if got := RelationRules()[RelReads].To[0]; got != NodeDatabase {
		t.Fatalf("the table was shared: reads.to[0] = %q", got)
	}
}

func TestLegalRelations(t *testing.T) {
	cases := []struct {
		name     string
		from, to NodeType
		want     []Relation
	}{
		{"gateway routes to a function", NodeGateway, NodeFunction, []Relation{RelRoutes}},
		{"compute reads and writes a database", NodeFunction, NodeDatabase, []Relation{RelReads, RelWrites}},
		{"compute publishes and consumes a queue", NodeService, NodeQueue, []Relation{RelPublishes, RelConsumes}},
		{"a database talks to nothing", NodeDatabase, NodeFunction, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if d := cmp.Diff(c.want, LegalRelations(c.from, c.to)); d != "" {
				t.Fatal(d)
			}
		})
	}
}

// The table is derived from the roles a catalogue entry declares. This pins what it was
// when it was written by hand, so the derivation is provably a refactor.
func TestRelationRulesArePinned(t *testing.T) {
	compute := []NodeType{NodeService, NodeFunction}
	stores := []NodeType{NodeDatabase, NodeBucket, NodeCache}
	want := map[Relation]RelationRule{
		RelRoutes:    {From: []NodeType{NodeGateway}, To: compute},
		RelCalls:     {From: compute, To: compute},
		RelReads:     {From: compute, To: stores},
		RelWrites:    {From: compute, To: stores},
		RelPublishes: {From: compute, To: []NodeType{NodeQueue}},
		RelConsumes:  {From: compute, To: []NodeType{NodeQueue}},
	}
	if diff := cmp.Diff(want, RelationRules()); diff != "" {
		t.Errorf("relation rules (-want +got):\n%s", diff)
	}
}
