<script lang="ts">
  import { projectField, reasonFor } from '../form.ts';
  import { defaultRegion, palettes, regionsFor, suggestedName } from '../sketch.ts';
  import { getStore } from '../store.svelte.ts';
  import type { Provider, ValidationError } from '../types.ts';
  import { errorLine } from '../validate.ts';

  let { onback }: { onback: () => void } = $props();

  const store = getStore();
  const nameField = projectField('name');
  const environmentField = projectField('environment');
  const fields = ['name', 'environment', 'provider', 'region'];

  let name = $state(suggestedName(store.workspace?.name ?? ''));
  let environment = $state('dev');
  let provider = $state<Provider>('aws');
  let region = $state(defaultRegion('aws'));
  let reasons = $state<Record<string, string | undefined>>({});
  let refused = $state<string[]>([]);

  const regions = $derived(regionsFor(provider));

  const input = 'h-8 rounded-md border border-border-strong bg-input px-2.5 text-[13px]';

  function pick(next: Provider) {
    provider = next;
    region = defaultRegion(next);
    reasons = { ...reasons, provider: undefined, region: undefined };
  }

  function setName(value: string) {
    name = value;
    reasons = { ...reasons, name: reasonFor(nameField, value) };
  }

  function setEnvironment(value: string) {
    environment = value;
    reasons = { ...reasons, environment: reasonFor(environmentField, value) };
  }

  // The same checks the schema applies, so a bad name is refused before the post.
  function sound(): boolean {
    reasons = {
      name: reasonFor(nameField, name),
      environment: reasonFor(environmentField, environment),
      region: regions.some((candidate) => candidate.id === region) ? undefined : 'pick a region',
    };
    return Object.values(reasons).every((reason) => reason === undefined);
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    refused = [];
    if (!sound()) {
      return;
    }
    place(await store.createProject({ name, environment, provider, region }));
  }

  // A refusal names the field it is about, or it is about the sketch as a whole.
  function place(errors: ValidationError[]) {
    const next = { ...reasons };
    const rest: string[] = [];
    for (const error of errors) {
      if (fields.includes(error.path)) {
        next[error.path] = error.message;
      } else {
        rest.push(errorLine(error));
      }
    }
    reasons = next;
    refused = rest;
  }
</script>

<form
  class="flex w-[640px] max-w-full flex-col gap-5 rounded-xl border border-border bg-panel p-7"
  aria-label="New sketch"
  novalidate
  onsubmit={submit}
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
    <h1 class="mt-1 text-[22px] leading-tight font-semibold tracking-[-.01em]">New sketch</h1>
  </div>

  <div class="grid grid-cols-2 gap-3.5">
    <div class="flex flex-col gap-1">
      <label class="text-xs font-medium" for="sketch-name">Project name</label>
      <input
        id="sketch-name"
        class={input}
        type="text"
        maxlength={nameField.maxLength}
        value={name}
        oninput={(event) => setName(event.currentTarget.value)}
      />
      {#if reasons.name}<p class="text-xs text-err">{reasons.name}</p>{/if}
      <p class="text-[11px] text-faint">
        Lowercase letters, digits and hyphens, up to {nameField.maxLength} characters. Used in resource
        names.
      </p>
    </div>
    <div class="flex flex-col gap-1">
      <label class="text-xs font-medium" for="sketch-environment">Environment</label>
      <input
        id="sketch-environment"
        class={input}
        type="text"
        maxlength={environmentField.maxLength}
        value={environment}
        oninput={(event) => setEnvironment(event.currentTarget.value)}
      />
      {#if reasons.environment}<p class="text-xs text-err">{reasons.environment}</p>{/if}
      <p class="text-[11px] text-faint">
        Lowercase letters, digits and hyphens, up to {environmentField.maxLength} characters.
      </p>
    </div>
  </div>

  <div class="flex flex-col gap-1.5">
    <span id="sketch-provider" class="text-xs font-medium">Provider</span>
    <div class="grid grid-cols-3 gap-2.5" role="radiogroup" aria-labelledby="sketch-provider">
      {#each palettes as entry (entry.provider)}
        <button
          type="button"
          class="flex flex-col gap-2.5 rounded-[10px] border p-3.5 text-left {provider ===
          entry.provider
            ? 'border-accent bg-raised ring-1 ring-accent'
            : 'border-border bg-panel hover:border-border-strong'}"
          role="radio"
          aria-checked={provider === entry.provider}
          aria-labelledby="sketch-provider-{entry.provider}"
          data-provider={entry.provider}
          onclick={() => pick(entry.provider)}
        >
          <span class="flex items-center justify-between">
            <span class="inline-flex items-center gap-2 font-semibold">
              <span class="h-2.5 w-2.5 rounded-full" style="background: {entry.accent}"></span>
              <span id="sketch-provider-{entry.provider}">{entry.label}</span>
            </span>
            {#if provider === entry.provider}
              <span
                class="inline-flex h-4 w-4 items-center justify-center rounded-full bg-accent text-accent-text"
              >
                <svg
                  width="12"
                  height="12"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="3"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  aria-hidden="true"
                >
                  <path d="M5 12l5 5L20 7" />
                </svg>
              </span>
            {/if}
          </span>
          <span class="flex gap-1">
            {#each entry.swatches as swatch (swatch.type)}
              <span
                class="h-5 w-5 rounded-[5px]"
                style="background: {swatch.color}"
                title="{swatch.type}: {swatch.resource}"
                data-kind={swatch.type}
              ></span>
            {/each}
          </span>
        </button>
      {/each}
    </div>
    {#if reasons.provider}<p class="text-xs text-err">{reasons.provider}</p>{/if}
  </div>

  <div class="flex flex-col gap-1">
    <label class="text-xs font-medium" for="sketch-region">Region</label>
    <select id="sketch-region" class={input} bind:value={region}>
      {#each regions as entry (entry.id)}
        <option value={entry.id}>{entry.id} · {entry.name}</option>
      {/each}
    </select>
    {#if reasons.region}<p class="text-xs text-err">{reasons.region}</p>{/if}
    <p class="text-[11px] text-faint">
      The provider's own region ids. The list follows the provider you picked.
    </p>
  </div>

  {#if refused.length > 0}
    <ul class="text-xs text-err" aria-label="Refused">
      {#each refused as line, index (index)}
        <li>{line}</li>
      {/each}
    </ul>
  {/if}

  <div class="flex items-center justify-between border-t border-border pt-3">
    <span class="text-xs text-faint">
      Creates <span class="font-mono">togen/</span> and <span class="font-mono">togen.yml</span>,
      then opens the canvas.
    </span>
    <button
      type="submit"
      class="inline-flex h-[30px] shrink-0 items-center rounded-md border border-accent bg-accent px-3 font-medium text-accent-text hover:opacity-90 disabled:opacity-40 disabled:hover:opacity-40"
      disabled={store.creating}>{store.creating ? 'Creating' : 'Create sketch'}</button
    >
  </div>
</form>
