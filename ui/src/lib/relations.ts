import table from '../../../schema/relations.json';

import type { Node, NodeType, Relation } from './types.ts';

const rules = table.relations as Record<string, { from: string[]; to: string[] }>;
const order = Object.keys(rules) as Relation[];

export function legalRelations(from: NodeType, to: NodeType): Relation[] {
  return order.filter((relation) => {
    const rule = rules[relation];
    return rule.from.includes(from) && rule.to.includes(to);
  });
}

// The canvas refuses a connection the CLI would refuse on save, in the same words
// where there is a relation to name and plainly where there is not.
export function connectionRefusal(from: Node, to: Node): string | undefined {
  if (from.id === to.id) {
    return 'an edge cannot connect a node to itself';
  }
  if (legalRelations(from.type, to.type).length === 0) {
    return `no relation is allowed from a ${from.type} to a ${to.type}`;
  }
  return undefined;
}
