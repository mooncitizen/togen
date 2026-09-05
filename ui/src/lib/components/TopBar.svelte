<script lang="ts">
  import { errorLines, getStore } from '../store.svelte.ts';
  import { errorLine } from '../validate.ts';

  const store = getStore();
  const project = $derived(store.project);
  const lines = $derived(store.error === null ? [] : errorLines(store.error));
  const problems = $derived(store.errors.map(errorLine));

  let open = $state(false);
</script>

<header class="relative flex items-baseline gap-3 border-b border-stone-200 bg-white px-4 py-2">
  <h1 class="text-sm font-semibold">Togen studio</h1>
  <span class="text-sm text-stone-500">{project?.name ?? 'not loaded'}</span>
  {#if project}
    <span class="rounded bg-stone-100 px-1.5 py-0.5 text-xs tracking-wide text-stone-600 uppercase">
      {project.provider}
    </span>
  {/if}
  <div class="ml-auto flex min-w-0 items-baseline gap-3">
    {#if store.notice !== null}
      <p class="truncate text-xs text-amber-700">{store.notice}</p>
    {/if}
    {#if problems.length > 0}
      <button
        class="shrink-0 rounded border border-red-300 px-1.5 py-0.5 text-xs text-red-700 hover:bg-red-50"
        aria-expanded={open}
        onclick={() => (open = !open)}
        >{problems.length}
        {problems.length === 1 ? 'problem' : 'problems'}</button
      >
    {/if}
    {#if lines.length > 0}
      <ul class="min-w-0 text-right text-xs text-red-700">
        {#each lines as line, index (index)}
          <li class="truncate">{line}</li>
        {/each}
      </ul>
    {/if}
  </div>
  {#if open && problems.length > 0}
    <ul
      class="absolute top-full right-0 z-30 max-h-64 w-[46rem] max-w-full overflow-y-auto rounded-b border border-stone-200 bg-white p-2 text-xs text-red-700 shadow-lg"
      aria-label="Problems"
    >
      {#each problems as line, index (index)}
        <li class="py-0.5">{line}</li>
      {/each}
    </ul>
  {/if}
</header>
