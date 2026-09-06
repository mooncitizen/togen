<script lang="ts">
  import { resolveIcon } from '../style.ts';
  import type { Shape } from '../types.ts';

  let { icon, color, shape, size }: { icon: string; color: string; shape: Shape; size: number } =
    $props();
  const art = $derived(resolveIcon(icon));
</script>

<span
  class="square"
  data-icon={icon}
  data-shape={shape}
  data-ground={art.ground}
  style="--node: {color}; --size: {size}px"
  aria-hidden="true"
>
  {#if art.kind === 'inline'}
    {@html art.svg}
  {:else if art.kind === 'file'}
    <img src={art.url} alt="" />
  {:else}
    <svg viewBox="0 0 24 24" fill="none" stroke="#ffffff" stroke-width="1.75">
      <rect x="5" y="5" width="14" height="14" rx="3" />
    </svg>
  {/if}
</span>

<style>
  .square {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    width: var(--size);
    height: var(--size);
    border-radius: 8px;
    background: var(--node);
    overflow: hidden;
  }

  .square[data-ground='tint'] {
    background: color-mix(in srgb, var(--node) 35%, var(--togen-panel));
  }

  .square :global(svg),
  .square img {
    display: block;
    width: 100%;
    height: 100%;
  }

  .square[data-ground='tint'] :global(svg) {
    width: 70%;
    height: 70%;
  }

  /* The shape varies the silhouette of the square, not the card. */
  .square[data-shape='circle'] {
    border-radius: 50%;
  }

  .square[data-shape='cylinder'] {
    border-radius: 50% / 24%;
  }

  .square[data-shape='hexagon'] {
    border-radius: 0;
    clip-path: polygon(25% 0, 75% 0, 100% 50%, 75% 100%, 25% 100%, 0 50%);
  }
</style>
