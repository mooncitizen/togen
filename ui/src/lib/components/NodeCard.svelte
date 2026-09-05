<script lang="ts">
  import { Handle, Position, type NodeProps } from '@xyflow/svelte';

  let { data, selected }: NodeProps = $props();
  const card = $derived(data as { name: string; type: string; errors: number });
</script>

<div
  class="w-40 rounded-md border bg-panel px-2 py-1.5 {selected
    ? 'border-selection ring-1 ring-selection'
    : card.errors > 0
      ? 'border-err'
      : 'border-border-strong'}"
>
  <div class="flex items-baseline gap-1">
    <div class="text-[10px] font-semibold tracking-[.06em] text-muted uppercase">{card.type}</div>
    {#if card.errors > 0}
      <span
        class="ml-auto rounded-full bg-err px-1.5 text-[10px] font-semibold text-white"
        aria-label="{card.errors} problems">{card.errors}</span
      >
    {/if}
  </div>
  <div class="truncate text-sm font-medium">{card.name}</div>
</div>
<Handle type="target" position={Position.Left} />
<Handle type="source" position={Position.Right} />
