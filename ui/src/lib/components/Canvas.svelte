<script lang="ts">
  import {
    Background,
    Controls,
    MiniMap,
    SvelteFlow,
    useSvelteFlow,
    type Edge,
    type Node,
  } from '@xyflow/svelte';

  import { isNodeType } from '../catalogue.ts';
  import { getStore } from '../store.svelte.ts';
  import type { Position } from '../types.ts';
  import NodeCard from './NodeCard.svelte';

  const store = getStore();
  const flow = useSvelteFlow();
  const nodeTypes = { togen: NodeCard };

  let nodes = $state.raw<Node[]>([]);
  let edges = $state.raw<Edge[]>([]);

  // Svelte Flow owns selection and in-flight drag positions, so the store
  // pushes into these rather than the canvas reading from it directly.
  $effect(() => {
    nodes = store.flowNodes;
  });
  $effect(() => {
    edges = store.flowEdges;
  });

  function allowDrop(event: DragEvent) {
    event.preventDefault();
    if (event.dataTransfer !== null) {
      event.dataTransfer.dropEffect = 'copy';
    }
  }

  function drop(event: DragEvent) {
    event.preventDefault();
    const type = event.dataTransfer?.getData('application/togen-node') ?? '';
    if (!isNodeType(type)) {
      return;
    }
    void store.addNode(type, flow.screenToFlowPosition({ x: event.clientX, y: event.clientY }));
  }

  // A box-selected group drags together, so every dragged node needs saving,
  // not just the one Svelte Flow happens to call the target.
  function dragStop({ nodes: dragged }: { nodes: { id: string; position: Position }[] }) {
    if (dragged.length === 0) {
      return;
    }
    const positions: Record<string, Position> = {};
    for (const node of dragged) {
      positions[node.id] = node.position;
    }
    store.moveNodes(positions);
  }
</script>

<div class="min-w-0 flex-1">
  {#if store.ready}
    <SvelteFlow
      bind:nodes
      bind:edges
      {nodeTypes}
      nodesConnectable={false}
      initialViewport={store.viewport}
      ondragover={allowDrop}
      ondrop={drop}
      onnodedragstop={dragStop}
      onnodeclick={({ node }) => store.select(node.id)}
      onpaneclick={() => store.select(null)}
      onmoveend={(_, viewport) => store.setViewport(viewport)}
    >
      <Background />
      <Controls />
      <MiniMap />
    </SvelteFlow>
  {/if}
</div>
