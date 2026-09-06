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

<section class="flex flex-col gap-2 border-t border-border px-2.5 py-3">
  <h2 class="px-1.5 text-[11px] font-semibold tracking-[.06em] text-muted uppercase">Palette</h2>
  <ul class="grid grid-cols-2 gap-1.5">
    {#each catalogue as entry (entry.type)}
      <li
        class="flex cursor-grab flex-col items-center gap-1.5 rounded-lg border border-border bg-raised px-1.5 pt-2.5 pb-2 hover:border-border-strong active:cursor-grabbing"
        draggable="true"
        data-node-type={entry.type}
        title={entry.description}
        ondragstart={(event) => start(event, entry.type)}
      >
        <span class="h-8 w-8 rounded-lg bg-border-strong" aria-hidden="true"></span>
        <span class="text-xs">{entry.label}</span>
      </li>
    {/each}
  </ul>
  <p class="mx-1.5 mt-1 text-[11px] text-faint">Drag a type onto the canvas.</p>
</section>
