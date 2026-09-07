<script lang="ts">
  import { getStore } from '../store.svelte.ts';

  const store = getStore();
  const still =
    typeof window !== 'undefined' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  // The path is drawn by Svelte Flow, so the dots ride the same line the user sees,
  // corners and all.
  const shares = $derived.by(() => {
    const rates = store.rates.edges;
    const busiest = Object.values(rates).reduce((a, b) => Math.max(a, b), 0);
    if (busiest <= 0) {
      return [] as { id: string; share: number }[];
    }
    return store.flowEdges
      .map((edge) => ({ id: edge.id, share: (rates[edge.id] ?? 0) / busiest }))
      .filter((edge) => edge.share > 0);
  });

  function pathFor(id: string): string | null {
    const selector = `.svelte-flow__edge[data-id="${id}"] path.svelte-flow__edge-path`;
    return document.querySelector(selector)?.getAttribute('d') ?? null;
  }

  function count(share: number): number {
    return Math.max(1, Math.round(6 * share));
  }

  function seconds(share: number): number {
    return 4 - 2.8 * share;
  }

  // A weight per edge for the reduced-motion reading, applied by a stroke-width rule
  // in app.css.
  $effect(() => {
    if (!still) {
      return;
    }
    for (const edge of shares) {
      const el = document.querySelector<HTMLElement>(`.svelte-flow__edge[data-id="${edge.id}"]`);
      el?.style.setProperty('--togen-edge-weight', String(1 + 2 * edge.share));
    }
  });
</script>

{#if !still}
  <svg class="pointer-events-none absolute inset-0 h-full w-full" data-particles aria-hidden="true">
    {#each shares as edge (edge.id)}
      {@const d = pathFor(edge.id)}
      {#each Array(count(edge.share)) as _, i (i)}
        <circle r="2.5" fill="var(--togen-selection)" data-particle data-edge={edge.id}>
          {#if d !== null}
            <animateMotion
              dur="{seconds(edge.share)}s"
              begin="{(i * seconds(edge.share)) / count(edge.share)}s"
              repeatCount="indefinite"
              path={d}
            />
          {/if}
        </circle>
      {/each}
    {/each}
  </svg>
{/if}
