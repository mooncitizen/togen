package ir

import (
	"slices"

	"github.com/mooncitizen/togen/internal/catalogue"
)

type RelationRule struct{ From, To []NodeType }

// The rules are written in terms of the roles a catalogue entry declares, so a new entry
// joins the table by saying what it is rather than by being added here.
func withRole(r catalogue.Role) []NodeType {
	ids := catalogue.Embedded().WithRole(r)
	out := make([]NodeType, len(ids))
	for i, id := range ids {
		out[i] = NodeType(id)
	}
	return out
}

func entryPoints() []NodeType { return withRole(catalogue.RoleEntry) }
func compute() []NodeType     { return withRole(catalogue.RoleCompute) }
func dataStores() []NodeType  { return withRole(catalogue.RoleStore) }
func messaging() []NodeType   { return withRole(catalogue.RoleMessaging) }

func RelationRules() map[Relation]RelationRule {
	return map[Relation]RelationRule{
		RelRoutes:    {From: entryPoints(), To: compute()},
		RelCalls:     {From: compute(), To: compute()},
		RelReads:     {From: compute(), To: dataStores()},
		RelWrites:    {From: compute(), To: dataStores()},
		RelPublishes: {From: compute(), To: messaging()},
		RelConsumes:  {From: compute(), To: messaging()},
	}
}

func LegalRelations(from, to NodeType) []Relation {
	rules := RelationRules()
	var out []Relation
	for _, r := range Relations {
		rule := rules[r]
		if slices.Contains(rule.From, from) && slices.Contains(rule.To, to) {
			out = append(out, r)
		}
	}
	return out
}
