import schemes from '../../../schema/styles.json';

import type { Node, NodeType, Position, Project, Provider } from './types.ts';

export type Rect = { x: number; y: number; width: number; height: number };

export type Boundary = {
  id: 'network' | 'resource-group';
  kind: string;
  label: string;
  color: string;
  nodeIds: string[];
  rect: Rect;
};

type Sketch = Pick<Project, 'name' | 'environment' | 'nodes' | 'edges'>;

const cardWidth = 160;
const cardHeight = 56;
const padding = 24;
const labelRoom = 12;

const hosted = new Set<NodeType>(['service', 'database', 'cache']);

// The AWS resolver's rule, per node: a service, database or cache always sits
// in the network, and a function joins it once it calls a service or reads or
// writes a database or cache. GCP and Azure have no resolver yet and are drawn
// by the same rule. The outer boundary comes first so it is painted underneath.
export function boundariesFor(
  project: Sketch,
  visible: Set<string>,
  positions: Record<string, Position>,
  provider: Provider,
): Boundary[] {
  const placed = project.nodes.filter((node) => visible.has(node.id) && node.id in positions);
  const members = placed.filter((node) => needsNetwork(node, project));
  const scheme = schemes[provider];
  const name = `${project.name}-${project.environment}`;
  const network: Boundary | undefined =
    members.length === 0
      ? undefined
      : {
          id: 'network',
          kind: scheme.network.kind,
          label: name,
          color: scheme.network.color,
          nodeIds: members.map((node) => node.id),
          rect: pad(cards(members, positions)),
        };

  const boundaries: Boundary[] = [];
  if ('resourceGroup' in scheme && placed.length > 0) {
    const inside = cards(placed, positions);
    boundaries.push({
      id: 'resource-group',
      kind: scheme.resourceGroup.kind,
      label: `${name}-rg`,
      color: scheme.resourceGroup.color,
      nodeIds: placed.map((node) => node.id),
      rect: pad(network === undefined ? inside : union(inside, network.rect)),
    });
  }
  if (network !== undefined) {
    boundaries.push(network);
  }
  return boundaries;
}

function needsNetwork(node: Node, project: Sketch): boolean {
  if (hosted.has(node.type)) {
    return true;
  }
  if (node.type !== 'function') {
    return false;
  }
  const types = new Map(project.nodes.map((candidate) => [candidate.id, candidate.type]));
  return project.edges.some((edge) => {
    const to = types.get(edge.to);
    return edge.from === node.id && to !== undefined && hosted.has(to);
  });
}

function cards(nodes: Node[], positions: Record<string, Position>): Rect {
  return nodes
    .map((node) => ({ ...positions[node.id], width: cardWidth, height: cardHeight }))
    .reduce(union);
}

function union(a: Rect, b: Rect): Rect {
  const x = Math.min(a.x, b.x);
  const y = Math.min(a.y, b.y);
  return {
    x,
    y,
    width: Math.max(a.x + a.width, b.x + b.width) - x,
    height: Math.max(a.y + a.height, b.y + b.height) - y,
  };
}

// The label needs a band of its own above the first row of cards.
function pad(rect: Rect): Rect {
  return {
    x: rect.x - padding,
    y: rect.y - padding - labelRoom,
    width: rect.width + padding * 2,
    height: rect.height + padding * 2 + labelRoom,
  };
}
