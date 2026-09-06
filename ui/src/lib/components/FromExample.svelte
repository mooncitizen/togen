<script lang="ts">
  import { onMount } from 'svelte';

  import { ApiError, getExamples } from '../api.ts';
  import { errorLines, getStore } from '../store.svelte.ts';
  import type { Example } from '../types.ts';
  import { errorLine } from '../validate.ts';

  let { onback }: { onback: () => void } = $props();

  const store = getStore();

  let examples = $state.raw<Example[] | null>(null);
  let unlisted = $state<string | null>(null);
  let refused = $state<string[]>([]);

  onMount(async () => {
    try {
      examples = await getExamples();
    } catch (failure) {
      unlisted = failure instanceof ApiError ? errorLines(failure)[0] : String(failure);
    }
  });

  async function use(id: string) {
    refused = [];
    const errors = await store.createFromExample(id);
    refused = errors.map(errorLine);
  }
</script>

<section
  class="flex w-[640px] max-w-full flex-col gap-5 rounded-xl border border-border bg-panel p-7"
  aria-label="Start from an example"
>
  <div class="flex flex-col gap-1">
    <button
      type="button"
      class="inline-flex items-center gap-1.5 self-start text-xs text-muted hover:text-text"
      onclick={onback}
    >
      <svg
        width="14"
        height="14"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <path d="M19 12H5m6-6-6 6 6 6" />
      </svg>
      Back
    </button>
    <h1 class="mt-1 text-[22px] leading-tight font-semibold tracking-[-.01em]">
      Start from an example
    </h1>
    <p class="text-muted">Copy one of the bundled projects in and change it.</p>
  </div>

  {#if unlisted !== null}
    <p class="text-xs text-err" role="alert">{unlisted}</p>
  {:else if examples === null}
    <p class="text-xs text-faint">Reading the examples.</p>
  {:else if examples.length === 0}
    <p class="text-xs text-faint">No examples are bundled with this build.</p>
  {:else}
    <ul class="flex flex-col gap-1.5" aria-label="Examples">
      {#each examples as example (example.id)}
        <li class="flex items-center gap-3 rounded-md border border-border bg-raised px-3 py-2.5">
          <span class="flex min-w-0 flex-1 flex-col gap-0.5">
            <span class="font-mono text-[13px]">{example.id}</span>
            <span class="text-xs text-muted">{example.description}</span>
          </span>
          <button
            type="button"
            class="inline-flex h-[30px] shrink-0 items-center rounded-md border border-accent bg-accent px-3 font-medium text-accent-text hover:opacity-90 disabled:opacity-40 disabled:hover:opacity-40"
            aria-label="Use {example.id}"
            disabled={store.creating}
            onclick={() => void use(example.id)}>Use</button
          >
        </li>
      {/each}
    </ul>
  {/if}

  {#if refused.length > 0}
    <ul class="text-xs text-err" aria-label="Refused">
      {#each refused as line, index (index)}
        <li>{line}</li>
      {/each}
    </ul>
  {/if}

  <p class="border-t border-border pt-3 text-xs text-faint">
    Copies <span class="font-mono">togen/</span> and <span class="font-mono">togen.yml</span> in,
    then opens the canvas.
  </p>
</section>
