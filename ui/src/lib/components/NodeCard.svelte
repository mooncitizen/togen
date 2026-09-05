<script lang="ts">
  import { Handle, Position, type NodeProps } from '@xyflow/svelte';

  let { data, selected }: NodeProps = $props();
  const card = $derived(data as { name: string; type: string; errors: number });
</script>

<div
  class="w-40 rounded border bg-white px-2 py-1.5 shadow-sm {selected
    ? 'border-stone-900 ring-1 ring-stone-900'
    : card.errors > 0
      ? 'border-red-500'
      : 'border-stone-300'}"
>
  <div class="flex items-baseline gap-1">
    <div class="text-[10px] tracking-wide text-stone-500 uppercase">{card.type}</div>
    {#if card.errors > 0}
      <span
        class="ml-auto rounded-full bg-red-600 px-1.5 text-[10px] font-medium text-white"
        aria-label="{card.errors} problems">{card.errors}</span
      >
    {/if}
  </div>
  <div class="truncate text-sm font-medium text-stone-900">{card.name}</div>
</div>
<Handle type="target" position={Position.Left} />
<Handle type="source" position={Position.Right} />
