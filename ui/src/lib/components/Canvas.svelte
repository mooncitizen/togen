<script lang="ts">
  import {
    Background,
    Controls,
    MiniMap,
    SvelteFlow,
    useSvelteFlow,
    type Connection,
    type Edge,
    type Node,
    type OnConnectEnd,
  } from '@xyflow/svelte';

  import { boundariesFor, type Boundary } from '../boundary.ts';
  import { isNodeType } from '../catalogue.ts';
  import { connectionRefusal, legalRelations } from '../relations.ts';
  import { getStore } from '../store.svelte.ts';
  import type { Node as Sketched, Position, Relation } from '../types.ts';
  import Boundaries from './Boundaries.svelte';
  import NodeCard from './NodeCard.svelte';
  import RelationMenu from './RelationMenu.svelte';
  import ViewsIcon from './ViewsIcon.svelte';

  const store = getStore();
  const flow = useSvelteFlow();
  const nodeTypes = { togen: NodeCard };

  let nodes = $state.raw<Node[]>([]);
  let edges = $state.raw<Edge[]>([]);
  let menu = $state.raw<{
    from: string;
    to: string;
    relations: Relation[];
    at: Position;
  } | null>(null);
  let pane: HTMLDivElement | undefined;

  // Svelte Flow owns selection and in-flight drag positions, so the store
  // pushes into these rather than the canvas reading from it directly.
  $effect(() => {
    nodes = store.flowNodes;
  });
  $effect(() => {
    edges = store.flowEdges;
  });

  // A card being dragged is where Svelte Flow has it, not where the store
  // does, so the boundary is drawn from the flow's nodes.
  const boundaries: Boundary[] = $derived.by(() => {
    const project = store.project;
    if (project === null) {
      return [];
    }
    const positions: Record<string, Position> = {};
    for (const node of nodes) {
      if (node.id in store.positions) {
        positions[node.id] = node.position;
      }
    }
    return boundariesFor(project, store.visibleNodeIds, positions, project.provider);
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

  function sketched(id: string): Sketched | undefined {
    return store.project?.nodes.find((node) => node.id === id);
  }

  function isValidConnection(connection: Edge | Connection): boolean {
    const from = sketched(connection.source);
    const to = sketched(connection.target);
    return from !== undefined && to !== undefined && connectionRefusal(from, to) === undefined;
  }

  // One legal relation is the answer; several are a question.
  function connect(connection: Connection) {
    const from = sketched(connection.source);
    const to = sketched(connection.target);
    if (from === undefined || to === undefined) {
      return;
    }
    const refusal = connectionRefusal(from, to);
    if (refusal !== undefined) {
      store.notify(refusal);
      edges = store.flowEdges;
      return;
    }
    const relations = legalRelations(from.type, to.type);
    if (relations.length === 1) {
      void store.addEdge(from.id, to.id, relations[0]);
      return;
    }
    menu = { from: from.id, to: to.id, relations, at: anchor(to.id) };
  }

  // A refused connection never reaches connect, so the reason is said here.
  const connectEnd: OnConnectEnd = (_event, state) => {
    if (state.isValid === true || state.fromNode === null || state.toNode === null) {
      return;
    }
    const from = sketched(state.fromNode.id);
    const to = sketched(state.toNode.id);
    if (from === undefined || to === undefined) {
      return;
    }
    const refusal = connectionRefusal(from, to);
    if (refusal !== undefined) {
      store.notify(refusal);
    }
  };

  function choose(relation: Relation) {
    const asked = menu;
    menu = null;
    edges = store.flowEdges;
    if (asked !== null) {
      void store.addEdge(asked.from, asked.to, relation);
    }
  }

  // The menu sits on the node the connection was dropped on, wherever the canvas
  // has been panned to.
  function anchor(id: string): Position {
    const dropped = flow.getNode(id);
    const box = pane?.getBoundingClientRect();
    if (dropped === undefined || box === undefined) {
      return { x: 0, y: 0 };
    }
    const at = flow.flowToScreenPosition(dropped.position);
    return { x: at.x - box.left, y: at.y - box.top };
  }

  // Deleting is the canvas's key, not the page's: a backspace in the inspector
  // is a backspace.
  function keydown(event: KeyboardEvent) {
    if (event.key !== 'Delete' && event.key !== 'Backspace') {
      return;
    }
    const target = event.target;
    if (!(target instanceof Element) || pane?.contains(target) !== true) {
      return;
    }
    if (store.selectedEdgeId !== null) {
      void store.deleteEdge(store.selectedEdgeId);
      return;
    }
    if (store.selectedNodeId !== null) {
      void store.deleteNode(store.selectedNodeId);
    }
  }
</script>

<svelte:window onkeydown={keydown} />

<div class="relative min-w-0 flex-1" bind:this={pane}>
  {#if store.ready}
    <!-- Each view is its own drawing with its own viewport, so switching remounts the flow. -->
    {#key store.activeView}
      <SvelteFlow
        bind:nodes
        bind:edges
        {nodeTypes}
        {isValidConnection}
        deleteKey={null}
        initialViewport={store.viewport}
        ondragover={allowDrop}
        ondrop={drop}
        onnodedragstop={dragStop}
        onnodeclick={({ node }) => store.select(node.id)}
        onedgeclick={({ edge }) => store.selectEdge(edge.id)}
        onpaneclick={() => store.clearSelection()}
        onconnect={connect}
        onconnectend={connectEnd}
        onmoveend={(_, viewport) => store.setViewport(viewport)}
      >
        <Boundaries {boundaries} />
        <Background />
        <Controls />
        <MiniMap />
      </SvelteFlow>
    {/key}
    {#if store.project !== null}
      <div
        class="pointer-events-none absolute top-3.5 left-4 z-10 flex items-center gap-2 text-xs text-muted"
        aria-label="Current view"
      >
        <span class="text-accent"><ViewsIcon /></span>
        <span class="font-medium text-text">{store.view.name}</span>
        <span aria-hidden="true">·</span>
        <span>{store.visibleNodeIds.size} of {store.project.nodes.length} nodes</span>
      </div>
    {/if}
  {/if}
  {#if menu !== null}
    <RelationMenu
      relations={menu.relations}
      at={menu.at}
      onchoose={choose}
      oncancel={() => {
        menu = null;
        edges = store.flowEdges;
      }}
    />
  {/if}
</div>
