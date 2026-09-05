<script lang="ts">
  import { fly } from 'svelte/transition';

  import { belongs, money, quantity, unitPrice } from '../cost.ts';
  import { getStore } from '../store.svelte.ts';
  import { errorLine } from '../validate.ts';

  const store = getStore();
  const cost = $derived(store.cost);
  const lines = $derived(store.costErrors.map(errorLine));
  const selected = $derived(
    store.project?.nodes.find((node) => node.id === store.selectedNodeId) ?? null,
  );

  let list: HTMLDivElement | undefined;

  function highlighted(entry: { name: string; kind?: string }): boolean {
    return selected !== null && belongs(entry, selected);
  }

  $effect(() => {
    if (selected !== null) {
      list?.querySelector('[data-selected]')?.scrollIntoView({ block: 'nearest' });
    }
  });

  function keydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      store.toggleCost(false);
    }
  }
</script>

<svelte:window onkeydown={keydown} />

<aside
  class="flex w-80 shrink-0 flex-col border-l border-border bg-panel"
  aria-label="Cost"
  transition:fly={{ x: 24, duration: 120 }}
>
  <header class="flex h-12 shrink-0 items-center gap-2 border-b border-border pr-2 pl-4">
    <span class="text-[11px] font-semibold tracking-[.06em] text-muted uppercase">Cost</span>
    {#if cost !== null}
      <span class="font-mono text-xs text-muted">{cost.currency}/month</span>
    {/if}
    <button
      class="ml-auto inline-flex h-[30px] w-[30px] items-center justify-center rounded-md text-muted hover:bg-raised hover:text-text"
      aria-label="Close cost"
      onclick={() => store.toggleCost(false)}
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
  <div class="min-h-0 flex-1 overflow-y-auto" bind:this={list}>
    {#if lines.length > 0}
      <ul class="p-4 text-xs text-err" aria-label="Cost problems">
        {#each lines as line, index (index)}
          <li class="py-0.5">{line}</li>
        {/each}
      </ul>
    {:else if cost !== null}
      {#each cost.items as item (`${item.kind}/${item.name}`)}
        <section
          class="border-b border-border px-4 py-3 {highlighted(item) ? 'bg-raised' : ''}"
          data-selected={highlighted(item) || undefined}
        >
          <div class="flex items-baseline gap-1.5">
            <span class="truncate font-medium">{item.name}</span>
            <span class="shrink-0 text-[11px] text-faint">{item.kind}</span>
          </div>
          {#if item.summary}
            <p class="text-[11px] text-muted">{item.summary}</p>
          {/if}
          <div class="mt-2 flex flex-col gap-1">
            {#each item.lines as line (line.label)}
              <div
                class="grid grid-cols-[1fr_auto_auto] items-baseline gap-x-3 text-xs"
                title={line.sku}
              >
                <span class="truncate">{line.label}</span>
                <span class="font-mono text-[11px] whitespace-nowrap text-muted"
                  >{quantity(line.quantity)} {line.unit} × {unitPrice(line.unitPrice)}</span
                >
                <span class="text-right font-mono">{money(line.amount)}</span>
              </div>
            {/each}
            <div class="grid grid-cols-[1fr_auto] items-baseline gap-x-3 text-xs">
              <span class="text-right text-[11px] text-muted">subtotal</span>
              <span class="font-mono font-medium">{money(item.subtotal)}</span>
            </div>
          </div>
        </section>
      {/each}
      {#if cost.notPriced.length > 0}
        <section class="px-4 py-3">
          <h3 class="text-[11px] font-semibold tracking-[.06em] text-muted uppercase">
            Not priced
          </h3>
          <ul class="mt-2 flex flex-col gap-1">
            {#each cost.notPriced as omission, index (index)}
              <li
                class="-mx-2 rounded-md px-2 py-1 {highlighted(omission) ? 'bg-raised' : ''}"
                data-selected={highlighted(omission) || undefined}
              >
                <div class="flex items-baseline gap-1.5 text-xs">
                  <span class="truncate font-medium">{omission.name}</span>
                  <span class="shrink-0 text-[11px] text-faint">{omission.kind}</span>
                </div>
                <p class="text-[11px] text-muted">{omission.reason}</p>
              </li>
            {/each}
          </ul>
        </section>
      {/if}
    {:else}
      <p class="p-4 text-xs text-muted">No estimate yet.</p>
    {/if}
  </div>
  {#if cost !== null}
    <footer
      class="flex shrink-0 flex-col gap-1 border-t border-border px-4 py-2.5 text-[11px] text-faint"
    >
      <div class="flex items-baseline justify-between text-xs text-text">
        <span class="font-medium">Total</span>
        <span class="font-mono">{money(cost.total)} {cost.currency}/month</span>
      </div>
      <p>{cost.region}, {cost.note}</p>
      {#if cost.warning}
        <p class="text-warn">{cost.warning}</p>
      {/if}
    </footer>
  {/if}
</aside>
