<script lang="ts">
  import { ViewportPortal } from '@xyflow/svelte';

  import type { Boundary } from '../boundary.ts';

  let { boundaries }: { boundaries: Boundary[] } = $props();
</script>

<ViewportPortal target="back">
  {#each boundaries as boundary (boundary.id)}
    <div
      class="pointer-events-none absolute font-mono"
      data-boundary={boundary.id}
      data-nodes={boundary.nodeIds.join(' ')}
      style="left: {boundary.rect.x}px; top: {boundary.rect.y}px; width: {boundary.rect.width}px; height: {boundary.rect.height}px; --boundary: {boundary.color}"
    >
      <svg class="frame" aria-hidden="true">
        <rect width="100%" height="100%" rx="14" />
      </svg>
      <span
        class="label absolute top-2.5 left-3.5 flex items-center gap-1.5 text-[10.5px] leading-4 whitespace-nowrap"
      >
        <span class="kind font-medium tracking-[.08em] uppercase">{boundary.kind}</span>
        <span class="text-muted">{boundary.label}</span>
        <span class="rounded-full border border-border px-1.5 text-[10px] leading-[14px] text-muted"
          >implicit</span
        >
      </span>
    </div>
  {/each}
</ViewportPortal>

<style>
  .frame {
    position: absolute;
    top: 0.75px;
    left: 0.75px;
    width: calc(100% - 1.5px);
    height: calc(100% - 1.5px);
    overflow: visible;
  }

  .frame rect {
    fill: color-mix(in srgb, var(--boundary) 4%, transparent);
    stroke: color-mix(in srgb, var(--boundary) 60%, transparent);
    stroke-width: 1.5;
    stroke-dasharray: 6 5;
  }

  .kind {
    color: var(--boundary);
  }
</style>
