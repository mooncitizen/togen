<script lang="ts">
  import { errorLines, getStore } from '../store.svelte.ts';
  import FromExample from './FromExample.svelte';
  import NewSketch from './NewSketch.svelte';
  import ThemeToggle from './ThemeToggle.svelte';
  import Wordmark from './Wordmark.svelte';

  type Screen = 'choose' | 'sketch' | 'example';

  const store = getStore();
  const name = $derived(store.workspace?.name ?? '');

  let screen = $state<Screen>('choose');

  const card =
    'flex w-[300px] flex-col items-start gap-3 rounded-xl border border-border bg-panel p-[22px] text-left';
</script>

<div class="flex h-full flex-col bg-canvas text-text" data-screen="first-run">
  <header class="flex h-12 shrink-0 items-center gap-3 border-b border-border bg-panel px-4">
    <Wordmark />
    <span class="h-5 w-px bg-border"></span>
    <span class="inline-flex min-w-0 items-center gap-1.5 text-muted" title={store.workspace?.dir}>
      <svg
        width="16"
        height="16"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="1.75"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
      </svg>
      <span class="truncate font-mono text-xs">{name}</span>
    </span>
    <div class="ml-auto"><ThemeToggle /></div>
  </header>
  <main
    class="flex min-h-0 flex-1 flex-col items-center justify-center gap-7 overflow-y-auto p-6 [background-image:radial-gradient(var(--togen-dot)_1px,transparent_1px)] [background-size:20px_20px]"
  >
    {#if store.conflict !== null}
      <div
        class="flex w-[640px] max-w-full items-center gap-4 rounded-xl border border-warn bg-panel px-4 py-3"
        role="alert"
      >
        <div class="flex min-w-0 flex-1 flex-col gap-0.5">
          <p class="font-medium">A project now exists in this directory.</p>
          <p class="truncate font-mono text-xs text-muted">{store.conflict}</p>
        </div>
        <button
          class="inline-flex h-[30px] shrink-0 items-center rounded-md border border-accent bg-accent px-3 font-medium text-accent-text hover:opacity-90"
          onclick={() => void store.load()}>Open it</button
        >
      </div>
    {:else if store.error !== null}
      <p class="w-[640px] max-w-full text-xs text-err" role="alert">{errorLines(store.error)[0]}</p>
    {/if}
    {#if screen === 'sketch'}
      <NewSketch onback={() => (screen = 'choose')} />
    {:else if screen === 'example'}
      <FromExample onback={() => (screen = 'choose')} />
    {:else}
      <div class="flex flex-col items-center gap-1.5 text-center">
        <p class="text-muted">No project in <span class="font-mono">{name}</span> yet.</p>
        <h1 class="text-[26px] leading-tight font-semibold tracking-[-.01em]">
          What do you want to do?
        </h1>
      </div>
      <div class="flex flex-wrap justify-center gap-4">
        <button
          type="button"
          class="{card} hover:border-border-strong"
          aria-labelledby="choice-sketch"
          aria-describedby="choice-sketch-text"
          onclick={() => (screen = 'sketch')}
        >
          <svg
            class="text-accent"
            width="28"
            height="28"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <rect x="3" y="5" width="7" height="5" rx="1.5" />
            <rect x="14" y="14" width="7" height="5" rx="1.5" />
            <path d="M10 7.5h3.5a2 2 0 0 1 2 2V14" />
          </svg>
          <span id="choice-sketch" class="text-[15px] font-semibold">New sketch</span>
          <span id="choice-sketch-text" class="text-muted"
            >Pick a provider, name the project and start with an empty canvas.</span
          >
          <span
            class="inline-flex h-[30px] items-center rounded-md border border-accent bg-accent px-3 font-medium text-accent-text"
            >Start</span
          >
        </button>
        <button
          type="button"
          class="{card} hover:border-border-strong"
          aria-labelledby="choice-example"
          aria-describedby="choice-example-text"
          onclick={() => (screen = 'example')}
        >
          <svg
            class="text-accent"
            width="28"
            height="28"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <path d="M4 6h16v12H4z" />
            <path d="M8 10h8M8 14h5" />
          </svg>
          <span id="choice-example" class="text-[15px] font-semibold">Start from an example</span>
          <span id="choice-example-text" class="text-muted"
            >Copy one of the bundled projects in and change it.</span
          >
          <span
            class="inline-flex h-[30px] items-center rounded-md border border-border-strong bg-raised px-3 font-medium"
            >Choose one</span
          >
        </button>
        <button
          type="button"
          class="{card} opacity-70"
          aria-labelledby="choice-import"
          aria-describedby="choice-import-text"
          disabled
        >
          <svg
            class="text-faint"
            width="28"
            height="28"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <path d="M12 3v12m0 0-4-4m4 4 4-4M4 17v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2" />
          </svg>
          <span id="choice-import" class="text-[15px] font-semibold">Import existing Terraform</span>
          <span id="choice-import-text" class="text-muted"
            >Read an infra directory into a sketch you can edit.</span
          >
          <span class="rounded-full border border-border px-2 py-0.5 text-[11px] text-muted"
            >Coming later</span
          >
        </button>
      </div>
      <p class="text-center text-xs text-faint">
        Togen writes <span class="font-mono">togen/project.json</span>,
        <span class="font-mono">togen/layout.json</span> and
        <span class="font-mono">togen.yml</span> into this directory. Nothing leaves your machine.
      </p>
    {/if}
  </main>
</div>
