<script lang="ts">
  import { Handle, Position, type NodeProps } from '@xyflow/svelte';

  let { data }: NodeProps = $props();
  const card = $derived(data as { name: string; type: string });
</script>

<div class="w-40 rounded border border-stone-300 bg-white px-2 py-1.5 shadow-sm">
  <div class="text-[10px] tracking-wide text-stone-500 uppercase">{card.type}</div>
  <div class="truncate text-sm font-medium text-stone-900">{card.name}</div>
</div>
<Handle type="target" position={Position.Left} />
<Handle type="source" position={Position.Right} />
