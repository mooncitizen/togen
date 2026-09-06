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
    class="flex w-80 shrink-0 flex-col border-l border-border bg-panel"
    aria-label="Inspector"
    transition:fly={{ x: 24, duration: 120 }}
  >
    <header class="flex h-12 shrink-0 items-center gap-2 border-b border-border pr-2 pl-4">
      <span class="text-[11px] font-semibold tracking-[.06em] text-muted uppercase"
        >{node?.type ?? edge?.relation}</span
      >
      <button
        class="ml-auto inline-flex h-[30px] w-[30px] items-center justify-center rounded-md text-muted hover:bg-raised hover:text-text"
        aria-label="Close inspector"
        onclick={() => store.clearSelection()}
      >
        <svg
          width="14"
          height="14"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          aria-hidden="true"
        >
          <path d="M6 6l12 12M18 6 6 18" />
        </svg>
      </button>
    </header>
    <div class="min-h-0 flex-1 overflow-y-auto">
      {#if node !== null}
        {#key node.id}
          <NodeForm {node} />
        {/key}
      {:else if edge !== null}
        {#key edge.id}
          <EdgeForm {edge} />
        {/key}
      {/if}
    </div>
    <footer class="flex shrink-0 items-center justify-between border-t border-border px-3 py-2.5">
      <button
        class="inline-flex h-[30px] items-center rounded-md border border-transparent px-3 font-medium text-err hover:bg-raised"
        onclick={remove}>Delete {node === null ? 'edge' : 'node'}</button
      >
      <span class="text-[11px] text-faint">{store.saving ? 'Saving' : 'Saved'}</span>
    </footer>
  </aside>
{/if}
