<script lang="ts">
  import { fly } from 'svelte/transition';

  import { getStore } from '../store.svelte.ts';
  import EdgeForm from './EdgeForm.svelte';
  import NodeForm from './NodeForm.svelte';

  const store = getStore();
  const node = $derived(
    store.project?.nodes.find((candidate) => candidate.id === store.selectedNodeId) ?? null,
  );
  const edge = $derived(
    store.project?.edges.find((candidate) => candidate.id === store.selectedEdgeId) ?? null,
  );

  function keydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      store.clearSelection();
    }
  }

  function remove() {
    if (node !== null) {
      void store.deleteNode(node.id);
      return;
    }
    if (edge !== null) {
      void store.deleteEdge(edge.id);
    }
  }
</script>

<svelte:window onkeydown={keydown} />

{#if node !== null || edge !== null}
  <aside
    class="flex w-80 shrink-0 flex-col overflow-y-auto border-l border-stone-200 bg-white"
    aria-label="Inspector"
    transition:fly={{ x: 24, duration: 120 }}
  >
    <header class="flex items-center gap-2 border-b border-stone-200 px-3 py-2">
      <span class="rounded bg-stone-100 px-1.5 py-0.5 text-xs tracking-wide text-stone-600 uppercase"
        >{node?.type ?? edge?.relation}</span
      >
      <button
        class="ml-auto rounded px-1.5 text-stone-500 hover:bg-stone-100 hover:text-stone-900"
        aria-label="Close inspector"
        onclick={() => store.clearSelection()}>×</button
      >
    </header>
    {#if node !== null}
      {#key node.id}
        <NodeForm {node} />
      {/key}
    {:else if edge !== null}
      {#key edge.id}
        <EdgeForm {edge} />
      {/key}
    {/if}
    <div class="mt-auto border-t border-stone-200 p-3">
      <button
        class="rounded border border-red-300 px-2 py-1 text-xs text-red-700 hover:bg-red-50"
        onclick={remove}>Delete {node === null ? 'edge' : 'node'}</button
      >
    </div>
  </aside>
{/if}
