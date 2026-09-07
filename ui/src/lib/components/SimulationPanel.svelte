<script lang="ts">
  import { fly } from 'svelte/transition';

  import { rateLabel } from '../simulate.ts';
  import { getStore } from '../store.svelte.ts';

  const store = getStore();
  const entries = $derived(
    (store.project?.nodes ?? []).filter((node) =>
      ['gateway', 'service', 'function'].includes(node.type),
    ),
  );
  const sources = $derived(store.simulation.sources);
  const bursts = $derived(store.simulation.bursts ?? []);

  const field = 'h-8 rounded-md border border-border-strong bg-input px-2.5 text-[13px]';

  function keydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      store.toggleSimulation(false);
    }
  }
</script>

<svelte:window onkeydown={keydown} />

<aside
  class="flex w-80 shrink-0 flex-col overflow-y-auto border-l border-border bg-panel"
  aria-label="Simulation"
  transition:fly={{ x: 24, duration: 120 }}
>
  <header class="flex h-12 shrink-0 items-center gap-2 border-b border-border pr-2 pl-4">
    <span class="text-[11px] font-semibold tracking-[.06em] text-muted uppercase">Simulation</span>
    <button
      class="ml-auto inline-flex h-[30px] w-[30px] items-center justify-center rounded-md text-muted hover:bg-raised hover:text-text"
      aria-label="Close simulation"
      onclick={() => store.toggleSimulation(false)}
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

  <section class="flex flex-col gap-3 border-b border-border p-4">
    <h3 class="text-xs font-medium">Load</h3>
    {#if sources.length === 0}
      <p class="text-[11px] text-faint">No load defined yet.</p>
    {/if}
    {#each sources as source (source.id)}
      <div class="flex flex-col gap-1.5 rounded-md border border-border p-2.5">
        <div class="flex items-center gap-1.5">
          <input
            class="{field} flex-1"
            aria-label="Source name"
            value={source.name}
            oninput={(e) => store.updateSource(source.id, { name: e.currentTarget.value })}
          />
          <button
            class="h-8 w-8 shrink-0 rounded-md text-muted hover:bg-raised hover:text-err"
            aria-label="Remove {source.name}"
            onclick={() => store.removeSource(source.id)}>×</button
          >
        </div>
        <select
          class={field}
          aria-label="Entry node"
          value={source.target}
          onchange={(e) => store.updateSource(source.id, { target: e.currentTarget.value })}
        >
          {#each entries as node (node.id)}
            <option value={node.id}>{node.name} ({node.type})</option>
          {/each}
        </select>
        <input
          class={field}
          aria-label="Rate"
          pattern={'[0-9]+(\\.[0-9]+)?[kM]?/(min|hour|day|month)'}
          value={source.rate}
          oninput={(e) => store.updateSource(source.id, { rate: e.currentTarget.value })}
        />
        <p class="text-[11px] text-faint">A number per period, such as 800/min or 2M/month.</p>
        <input
          class={field}
          type="number"
          min="0"
          aria-label="Bytes per request"
          value={source.bytesPerRequest ?? 0}
          oninput={(e) =>
            store.updateSource(source.id, { bytesPerRequest: Number(e.currentTarget.value) })}
        />
        <p class="text-[11px] text-faint">
          Average response size. The only thing egress and NAT are derived from.
        </p>
      </div>
    {/each}
    <button
      class="h-8 rounded-md border border-border-strong text-[13px] hover:bg-raised"
      onclick={() => store.addSource()}>Add a source</button
    >
  </section>

  <section class="flex flex-col gap-3 p-4">
    <h3 class="text-xs font-medium">Bursts</h3>
    {#each bursts as burst (burst.id)}
      <div class="flex flex-col gap-1.5 rounded-md border border-border p-2.5">
        <div class="flex items-center gap-1.5">
          <input
            class="{field} flex-1"
            aria-label="Burst name"
            value={burst.name}
            oninput={(e) => store.updateBurst(burst.id, { name: e.currentTarget.value })}
          />
          <button
            class="h-8 w-8 shrink-0 rounded-md text-muted hover:bg-raised hover:text-err"
            aria-label="Remove {burst.name}"
            onclick={() => store.removeBurst(burst.id)}>×</button
          >
        </div>
        <select
          class={field}
          aria-label="Burst source"
          value={burst.source}
          onchange={(e) => store.updateBurst(burst.id, { source: e.currentTarget.value })}
        >
          {#each sources as source (source.id)}
            <option value={source.id}>{source.name}</option>
          {/each}
        </select>
        <div class="grid grid-cols-3 gap-1.5">
          <input
            class={field}
            type="number"
            min="1"
            step="0.5"
            aria-label="Multiplier"
            value={burst.multiplier}
            oninput={(e) =>
              store.updateBurst(burst.id, { multiplier: Number(e.currentTarget.value) })}
          />
          <input
            class={field}
            type="number"
            min="1"
            aria-label="Minutes"
            value={burst.minutes}
            oninput={(e) => store.updateBurst(burst.id, { minutes: Number(e.currentTarget.value) })}
          />
          <input
            class={field}
            type="number"
            min="1"
            aria-label="Times a month"
            value={burst.timesPerMonth}
            oninput={(e) =>
              store.updateBurst(burst.id, { timesPerMonth: Number(e.currentTarget.value) })}
          />
        </div>
        <p class="text-[11px] text-faint">
          {burst.multiplier}x for {burst.minutes} minutes, {burst.timesPerMonth} times a month.
          Every burst is priced; the selector only chooses what the canvas shows.
        </p>
      </div>
    {/each}
    <button
      class="h-8 rounded-md border border-border-strong text-[13px] hover:bg-raised disabled:opacity-40"
      disabled={sources.length === 0}
      onclick={() => store.addBurst()}>Add a burst</button
    >
  </section>

  {#if store.simulating}
    <p class="mt-auto border-t border-border px-4 py-2 font-mono text-[11px] text-faint">
      togen/simulation.json · {rateLabel(
        Object.values(store.rates.nodes).reduce((a, b) => Math.max(a, b), 0),
      )} peak
    </p>
  {/if}
</aside>
