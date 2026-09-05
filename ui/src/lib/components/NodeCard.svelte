<script lang="ts">
  import { Handle, Position, type NodeProps } from '@xyflow/svelte';

  let { data, selected }: NodeProps = $props();
  const card = $derived(data as { name: string; type: string });
</script>

<div
  class="w-40 rounded border bg-white px-2 py-1.5 shadow-sm {selected
    ? 'border-stone-900 ring-1 ring-stone-900'
    : 'border-stone-300'}"
>
  <div class="text-[10px] tracking-wide text-stone-500 uppercase">{card.type}</div>
  <div class="truncate text-sm font-medium text-stone-900">{card.name}</div>
</div>
<Handle type="target" position={Position.Left} />
<Handle type="source" position={Position.Right} />
