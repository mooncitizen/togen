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
