<script module lang="ts">
  export function particleCount(share: number): number {
    return Math.max(1, Math.round(6 * share));
  }
</script>

<script lang="ts">
  import { ViewportPortal } from '@xyflow/svelte';

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

  let paths = $state.raw<Record<string, string>>({});

  // Held outside the reactive graph so the effect below never reads what it writes.
  let last: Record<string, string> = {};

  function read(): void {
    const next: Record<string, string> = {};
    for (const edge of document.querySelectorAll('.svelte-flow__edges .svelte-flow__edge')) {
      const id = edge.getAttribute('data-id');
      const d = edge.querySelector('path.svelte-flow__edge-path')?.getAttribute('d') ?? null;
      if (id !== null && d !== null) {
        next[id] = d;
      }
    }
    const ids = Object.keys(next);
    if (
      ids.length === Object.keys(last).length &&
      ids.every((id) => last[id] === next[id])
    ) {
      return;
    }
    last = next;
    paths = next;
  }

  // Svelte Flow paints the edges after this overlay mounts, and redraws them on every
  // node drag, so the paths are read from the DOM rather than during render.
  $effect(() => {
    const container = document.querySelector('.svelte-flow__edges');
    if (container === null) {
      return;
    }
    let queued = 0;
    const schedule = () => {
      if (queued !== 0) {
        return;
      }
      queued = requestAnimationFrame(() => {
        queued = 0;
        read();
      });
    };
    // The overlay is portalled into the viewport, not into this container, so its own
    // circles can never feed back through the observer.
    const observer = new MutationObserver(schedule);
    observer.observe(container, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ['d'],
    });
    schedule();
    return () => {
      observer.disconnect();
      if (queued !== 0) {
        cancelAnimationFrame(queued);
      }
    };
  });

  function seconds(share: number): number {
    return 4 - 2.8 * share;
  }

</script>

{#if !still}
  <!-- Inside the portal the dots share the viewport transform, so pan and zoom carry them. -->
  <ViewportPortal target="front">
    <svg
      class="pointer-events-none absolute top-0 left-0 h-full w-full overflow-visible"
      data-particles
      aria-hidden="true"
    >
      {#each shares as edge (edge.id)}
        {@const d = paths[edge.id] ?? null}
        {#if d !== null}
          {#each Array(particleCount(edge.share)) as _, i (i)}
            <circle r="2.5" fill="var(--togen-selection)" data-particle data-edge={edge.id}>
              <animateMotion
                dur="{seconds(edge.share)}s"
                begin="{(i * seconds(edge.share)) / particleCount(edge.share)}s"
                repeatCount="indefinite"
                path={d}
              />
            </circle>
          {/each}
        {/if}
      {/each}
    </svg>
  </ViewportPortal>
{/if}
