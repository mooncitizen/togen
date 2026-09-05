<script lang="ts">
  import { Handle, Position, type NodeProps } from '@xyflow/svelte';

  import type { Resolved } from '../style.ts';
  import Icon from './Icon.svelte';

  let { data, selected }: NodeProps = $props();
  const card = $derived(
    data as { name: string; type: string; errors: number; style: Resolved; dimmed: boolean },
  );
</script>

<div
  class="card relative flex items-center gap-2.5 px-3"
  class:selected
  class:dimmed={card.dimmed}
  style="--node: {card.style.color}"
>
  <Icon icon={card.style.icon} color={card.style.color} shape={card.style.shape} size={28} />
  <span class="flex min-w-0 flex-col">
    <span class="text-[10px] font-semibold tracking-[.06em] text-muted uppercase">{card.type}</span>
    <span class="truncate text-[13px] font-medium">{card.name}</span>
  </span>
  {#if card.errors > 0}
    <span
      class="absolute -top-2 -right-2 inline-flex h-[18px] min-w-[18px] items-center justify-center rounded-full bg-err px-1.5 text-[11px] font-semibold text-white"
      aria-label="{card.errors} {card.errors === 1 ? 'problem' : 'problems'}">{card.errors}</span
    >
  {/if}
</div>
<Handle type="target" position={Position.Left} />
<Handle type="source" position={Position.Right} />

<style>
  .card {
    width: 160px;
    height: 56px;
    border-radius: 10px;
    background: color-mix(in srgb, var(--node) var(--togen-tint-alpha), transparent);
    border: 1px solid color-mix(in srgb, var(--node) var(--togen-border-alpha), transparent);
  }

  .card.selected {
    border-color: var(--togen-selection);
    box-shadow: 0 0 0 1px var(--togen-selection);
  }

  .card.dimmed {
    opacity: 0.32;
  }
</style>
