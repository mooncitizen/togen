import schema from '../../../schema/project.schema.json';

import type { NodeType } from './types.ts';

export type CatalogueEntry = {
  type: NodeType;
  label: string;
  description: string;
};

const descriptions: Record<NodeType, string> = {
  service: 'A container that keeps running behind a load balancer.',
  function: 'Code that runs on demand, per request or per message.',
  database: 'A managed relational database, Postgres or MySQL.',
  gateway: 'The public entry point that routes to services and functions.',
  queue: 'Messages handed from a publisher to a consumer.',
  bucket: 'Object storage for files.',
  cache: 'An in-memory store for hot data.',
};

// The palette follows the schema, so a node type added there shows up here.
export const catalogue: CatalogueEntry[] = schema.properties.nodes.items.oneOf.map((variant) => {
  const type = variant.properties.type.const as NodeType;
  return { type, label: type[0].toUpperCase() + type.slice(1), description: descriptions[type] };
});

const types = new Set<string>(catalogue.map((entry) => entry.type));

export function isNodeType(value: string): value is NodeType {
  return types.has(value);
}

// A dropped node has to validate straight away, so the one property with no
// default gets a placeholder the user is expected to replace.
export function defaultProperties(type: NodeType): Record<string, unknown> {
  return type === 'service' ? { image: 'nginx:1.27' } : {};
}
