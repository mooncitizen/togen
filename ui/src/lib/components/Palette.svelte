<script lang="ts">
  import { catalogue } from '../catalogue.ts';
  import type { NodeType } from '../types.ts';

  function start(event: DragEvent, type: NodeType) {
    if (event.dataTransfer === null) {
      return;
    }
    event.dataTransfer.setData('application/togen-node', type);
    event.dataTransfer.effectAllowed = 'copy';
  }
</script>

<aside class="w-56 shrink-0 overflow-y-auto border-r border-stone-200 bg-white p-3">
  <h2 class="mb-2 text-xs font-semibold tracking-wide text-stone-500 uppercase">Palette</h2>
  <ul class="flex flex-col gap-1">
    {#each catalogue as entry (entry.type)}
      <li
        class="cursor-grab rounded border border-stone-200 px-2 py-1.5 hover:border-stone-400 hover:bg-stone-50 active:cursor-grabbing"
        draggable="true"
        data-node-type={entry.type}
        ondragstart={(event) => start(event, entry.type)}
      >
        <div class="text-sm font-medium">{entry.label}</div>
        <div class="text-xs text-stone-500">{entry.description}</div>
      </li>
    {/each}
  </ul>
</aside>
