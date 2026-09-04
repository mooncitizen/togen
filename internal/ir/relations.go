package ir

import "slices"

type RelationRule struct{ From, To []NodeType }

func compute() []NodeType    { return []NodeType{NodeService, NodeFunction} }
func dataStores() []NodeType { return []NodeType{NodeDatabase, NodeBucket, NodeCache} }

func RelationRules() map[Relation]RelationRule {
	return map[Relation]RelationRule{
		RelRoutes:    {From: []NodeType{NodeGateway}, To: compute()},
		RelCalls:     {From: compute(), To: compute()},
		RelReads:     {From: compute(), To: dataStores()},
		RelWrites:    {From: compute(), To: dataStores()},
		RelPublishes: {From: compute(), To: []NodeType{NodeQueue}},
		RelConsumes:  {From: compute(), To: []NodeType{NodeQueue}},
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
