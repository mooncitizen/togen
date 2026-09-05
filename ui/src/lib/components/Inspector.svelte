<script lang="ts">
  import { fly } from 'svelte/transition';

  import { getStore } from '../store.svelte.ts';
  import NodeForm from './NodeForm.svelte';

  const store = getStore();
  const node = $derived(
    store.project?.nodes.find((candidate) => candidate.id === store.selectedNodeId) ?? null,
  );

  function keydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      store.select(null);
    }
  }
</script>

<svelte:window onkeydown={keydown} />

{#if node !== null}
  <aside
    class="flex w-80 shrink-0 flex-col overflow-y-auto border-l border-stone-200 bg-white"
    aria-label="Inspector"
    transition:fly={{ x: 24, duration: 120 }}
  >
    <header class="flex items-center gap-2 border-b border-stone-200 px-3 py-2">
      <span class="rounded bg-stone-100 px-1.5 py-0.5 text-xs tracking-wide text-stone-600 uppercase"
        >{node.type}</span
      >
      <button
        class="ml-auto rounded px-1.5 text-stone-500 hover:bg-stone-100 hover:text-stone-900"
        aria-label="Close inspector"
        onclick={() => store.select(null)}>×</button
      >
    </header>
    {#key node.id}
      <NodeForm {node} />
    {/key}
  </aside>
{/if}
