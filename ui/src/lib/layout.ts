import dagre from '@dagrejs/dagre';

import type { Position, Project } from './types.ts';

export const nodeWidth = 180;
export const nodeHeight = 60;

export function autoLayout(project: Project): Record<string, Position> {
  const graph = new dagre.graphlib.Graph();
  graph.setGraph({ rankdir: 'LR', ranksep: 140, nodesep: 48, marginx: 48, marginy: 48 });
  graph.setDefaultEdgeLabel(() => ({}));

  const known = new Set(project.nodes.map((node) => node.id));
  for (const node of project.nodes) {
    graph.setNode(node.id, { width: nodeWidth, height: nodeHeight });
  }
  for (const edge of project.edges) {
    if (known.has(edge.from) && known.has(edge.to)) {
      graph.setEdge(edge.from, edge.to);
    }
  }
  dagre.layout(graph);

  const positions: Record<string, Position> = {};
  for (const node of project.nodes) {
    const laid = graph.node(node.id);
    positions[node.id] = {
      x: Math.round(laid.x - nodeWidth / 2),
      y: Math.round(laid.y - nodeHeight / 2),
    };
  }
  return positions;
}
