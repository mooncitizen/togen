<script lang="ts">
  import { errorLines, getStore } from '../store.svelte.ts';

  const store = getStore();
  const project = $derived(store.project);
  const lines = $derived(store.error === null ? [] : errorLines(store.error));
</script>

<header class="flex items-baseline gap-3 border-b border-stone-200 bg-white px-4 py-2">
  <h1 class="text-sm font-semibold">Togen studio</h1>
  <span class="text-sm text-stone-500">{project?.name ?? 'not loaded'}</span>
  {#if project}
    <span class="rounded bg-stone-100 px-1.5 py-0.5 text-xs tracking-wide text-stone-600 uppercase">
      {project.provider}
    </span>
  {/if}
  {#if lines.length > 0}
    <ul class="ml-auto min-w-0 text-right text-xs text-red-700">
      {#each lines as line, index (index)}
        <li class="truncate">{line}</li>
      {/each}
    </ul>
  {/if}
</header>
