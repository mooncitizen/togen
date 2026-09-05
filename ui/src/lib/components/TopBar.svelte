<script lang="ts">
  import { dash, money } from '../cost.ts';
  import { errorLines, getStore } from '../store.svelte.ts';
  import { accent } from '../style.ts';
  import { getTheme } from '../theme.svelte.ts';
  import { errorLine } from '../validate.ts';

  type Status = { tone: 'ok' | 'warn' | 'err'; text: string; lines: string[] };

  const tones = { ok: 'bg-ok', warn: 'bg-warn', err: 'bg-err' };

  const store = getStore();
  const theme = getTheme();
  const project = $derived(store.project);
  const lines = $derived(store.error === null ? [] : errorLines(store.error));
  const problems = $derived(store.problems.map(errorLine));
  const status = $derived(describe());
  const blocked = $derived(reason());
  const estimate = $derived(
    store.cost === null ? dash : `${money(store.cost.total)} ${store.cost.currency}/mo`,
  );

  let open = $state(false);

  // One pill, and the most recent thing wins: a notice answers what the user
  // just did, a refused save is newer than the checks, and the checks matter
  // more than what togen.yml has to say.
  function describe(): Status {
    if (store.notice !== null) {
      return { tone: 'warn', text: store.notice, lines: [] };
    }
    if (lines.length > 0) {
      return { tone: 'err', text: lines[0], lines: lines.slice(1) };
    }
    if (problems.length > 0) {
      const count = `${problems.length} ${problems.length === 1 ? 'problem' : 'problems'}`;
      return { tone: 'err', text: count, lines: problems };
    }
    if (store.note !== null) {
      return { tone: 'warn', text: store.note, lines: [] };
    }
    return { tone: 'ok', text: 'No problems', lines: [] };
  }

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

<header
  class="relative flex h-12 shrink-0 items-center gap-3 border-b border-border bg-panel pr-3 pl-4"
>
  <div class="flex items-baseline gap-1.5">
    <span class="text-sm font-semibold">Togen</span>
    <span class="text-xs text-muted">studio</span>
  </div>
  <span class="h-5 w-px bg-border"></span>
  {#if project}
    <div class="flex min-w-0 items-center gap-2">
      <span class="truncate font-medium">{project.name}</span>
      <span
        class="inline-flex h-[22px] shrink-0 items-center gap-1.5 rounded-full border border-border bg-raised px-2 text-[11px] font-semibold tracking-[.04em] uppercase"
      >
        <span class="h-2 w-2 rounded-full" style="background: {accent(project.provider)}"></span>
        {project.provider}
      </span>
      <span class="font-mono text-xs text-muted">{project.region}</span>
      <span class="font-mono text-xs text-muted">{project.environment}</span>
    </div>
  {:else}
    <span class="text-muted">not loaded</span>
  {/if}
  <div class="ml-auto flex min-w-0 items-center gap-2">
    <button
      class="inline-flex h-[30px] w-[30px] items-center justify-center rounded-md border border-transparent text-muted hover:bg-raised hover:text-text disabled:opacity-40 disabled:hover:bg-transparent disabled:hover:text-muted"
      aria-label="Undo"
      title="Undo"
      disabled={!store.canUndo}
      onclick={() => void store.undo()}
    >
      <svg
        width="16"
        height="16"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="1.75"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M9 14 4 9l5-5" /><path d="M4 9h10.5a5.5 5.5 0 0 1 0 11H11" />
      </svg>
    </button>
    <button
      class="inline-flex h-[30px] w-[30px] items-center justify-center rounded-md border border-transparent text-muted hover:bg-raised hover:text-text disabled:opacity-40 disabled:hover:bg-transparent disabled:hover:text-muted"
      aria-label="Redo"
      title="Redo"
      disabled={!store.canRedo}
      onclick={() => void store.redo()}
    >
      <svg
        width="16"
        height="16"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="1.75"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="m15 14 5-5-5-5" /><path d="M20 9H9.5a5.5 5.5 0 0 0 0 11H13" />
      </svg>
    </button>
    <button
      class="inline-flex h-[26px] shrink-0 items-center rounded-full border bg-raised px-2.5 font-mono text-xs hover:border-border-strong {store.costOpen
        ? 'border-border-strong'
        : 'border-border'} {store.cost === null ? 'text-muted' : ''}"
      aria-pressed={store.costOpen}
      title="Monthly estimate"
      onclick={() => store.toggleCost()}>{estimate}</button
    >
    {#if status.lines.length > 0}
      <button
        class="inline-flex h-[26px] min-w-0 items-center gap-1.5 rounded-full border border-border bg-raised px-2.5 text-xs hover:border-border-strong"
        aria-expanded={open}
        title={status.text}
        onclick={() => (open = !open)}
      >
        <span class="h-[7px] w-[7px] shrink-0 rounded-full {tones[status.tone]}"></span>
        <span class="truncate">{status.text}</span>
      </button>
    {:else}
      <span
        class="inline-flex h-[26px] min-w-0 items-center gap-1.5 rounded-full border border-border bg-raised px-2.5 text-xs"
        title={status.text}
      >
        <span class="h-[7px] w-[7px] shrink-0 rounded-full {tones[status.tone]}"></span>
        <span class="truncate">{status.text}</span>
      </span>
    {/if}
    <button
      class="inline-flex h-[30px] w-[30px] shrink-0 items-center justify-center rounded-md border border-transparent text-muted hover:bg-raised hover:text-text"
      aria-label={theme.scheme === 'dark' ? 'Switch to light' : 'Switch to dark'}
      title={theme.scheme === 'dark' ? 'Switch to light' : 'Switch to dark'}
      onclick={() => theme.toggle()}
    >
      {#if theme.scheme === 'dark'}
        <svg
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.75"
          stroke-linecap="round"
        >
          <circle cx="12" cy="12" r="4" />
          <path
            d="M12 2v2M12 20v2M2 12h2M20 12h2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"
          />
        </svg>
      {:else}
        <svg
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.75"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5z" />
        </svg>
      {/if}
    </button>
    <button
      class="inline-flex h-[30px] shrink-0 items-center rounded-md border border-border-strong bg-raised px-3 font-medium enabled:hover:bg-panel disabled:opacity-40"
      disabled={project === null}
      title={project === null ? 'no project loaded' : 'Export the current view'}
      onclick={() => (store.exporting = true)}>Export</button
    >
    <button
      class="inline-flex h-[30px] shrink-0 items-center rounded-md border border-accent bg-accent px-3 font-medium text-accent-text hover:opacity-90 disabled:opacity-40 disabled:hover:opacity-40"
      disabled={blocked !== ''}
      title={blocked === '' ? 'Write the target files' : blocked}
      onclick={() => void store.generate()}
      >{store.generating ? 'Generating' : 'Generate'}</button
    >
  </div>
  {#if open && status.lines.length > 0}
    <ul
      class="absolute top-full right-0 z-30 max-h-64 w-[46rem] max-w-full overflow-y-auto rounded-b-md border border-border bg-panel p-2 text-xs text-err shadow-panel"
      aria-label="Problems"
    >
      {#each status.lines as line, index (index)}
        <li class="py-0.5">{line}</li>
      {/each}
    </ul>
  {/if}
  {#if store.generated !== null}
    <section
      class="absolute top-full right-0 z-40 max-h-64 w-[28rem] max-w-full overflow-y-auto rounded-b-md border border-border bg-panel p-3 text-xs shadow-panel"
      aria-label="Generated"
    >
      <div class="flex items-center gap-2">
        <h2 class="font-medium">Generated</h2>
        <button
          class="ml-auto inline-flex h-6 w-6 items-center justify-center rounded-md text-muted hover:bg-raised hover:text-text"
          aria-label="Dismiss"
          onclick={() => store.dismissGenerated()}>×</button
        >
      </div>
      {#each store.generated ?? [] as written (written.dir)}
        <p class="mt-2 font-mono">{written.dir}</p>
        <ul class="text-muted">
          {#each written.files as file (file)}
            <li class="font-mono">{file}</li>
          {/each}
        </ul>
      {/each}
    </section>
  {/if}
</header>
