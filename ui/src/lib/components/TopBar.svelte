<script lang="ts">
  import { errorLines, getStore } from '../store.svelte.ts';
  import { errorLine } from '../validate.ts';

  const store = getStore();
  const project = $derived(store.project);
  const lines = $derived(store.error === null ? [] : errorLines(store.error));
  const problems = $derived(store.problems.map(errorLine));
  const blocked = $derived(reason());

  let open = $state(false);

  function reason(): string {
    if (store.project === null) {
      return 'no project loaded';
    }
    if (store.errors.length > 0) {
      return 'fix the problems first';
    }
    if (store.generating) {
      return 'generating';
    }
    return store.saving ? 'saving' : '';
  }

  // The browser's own undo owns a field the user is typing in.
  function keydown(event: KeyboardEvent) {
    if (!(event.metaKey || event.ctrlKey) || event.altKey) {
      return;
    }
    const key = event.key.toLowerCase();
    if (key !== 'z' && key !== 'y') {
      return;
    }
    const target = event.target;
    if (
      target instanceof HTMLElement &&
      (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName))
    ) {
      return;
    }
    event.preventDefault();
    if (key === 'y' || event.shiftKey) {
      void store.redo();
      return;
    }
    void store.undo();
  }
</script>

<svelte:window onkeydown={keydown} />

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
    <div class="flex shrink-0 items-baseline gap-1.5">
      <button
        class="rounded border border-stone-300 px-1.5 py-0.5 text-xs text-stone-700 hover:bg-stone-100 disabled:opacity-40 disabled:hover:bg-transparent"
        disabled={!store.canUndo}
        onclick={() => void store.undo()}>Undo</button
      >
      <button
        class="rounded border border-stone-300 px-1.5 py-0.5 text-xs text-stone-700 hover:bg-stone-100 disabled:opacity-40 disabled:hover:bg-transparent"
        disabled={!store.canRedo}
        onclick={() => void store.redo()}>Redo</button
      >
      <button
        class="rounded bg-stone-900 px-2 py-0.5 text-xs text-white hover:bg-stone-700 disabled:opacity-40 disabled:hover:bg-stone-900"
        disabled={blocked !== ''}
        title={blocked === '' ? 'Write the target files' : blocked}
        onclick={() => void store.generate()}
        >{store.generating ? 'Generating' : 'Generate'}</button
      >
    </div>
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
  {#if store.generated !== null}
    <section
      class="absolute top-full right-0 z-40 max-h-64 w-[28rem] max-w-full overflow-y-auto rounded-b border border-stone-200 bg-white p-3 text-xs shadow-lg"
      aria-label="Generated"
    >
      <div class="flex items-baseline gap-2">
        <h2 class="font-medium text-stone-900">Generated</h2>
        <button
          class="ml-auto rounded px-1.5 text-stone-500 hover:bg-stone-100 hover:text-stone-900"
          aria-label="Dismiss"
          onclick={() => store.dismissGenerated()}>×</button
        >
      </div>
      {#each store.generated ?? [] as written (written.dir)}
        <p class="mt-2 font-mono text-stone-900">{written.dir}</p>
        <ul class="text-stone-600">
          {#each written.files as file (file)}
            <li class="font-mono">{file}</li>
          {/each}
        </ul>
      {/each}
    </section>
  {/if}
</header>
