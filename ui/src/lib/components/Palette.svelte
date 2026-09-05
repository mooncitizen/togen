<script lang="ts">
  import { catalogue } from '../catalogue.ts';
  import { getStore } from '../store.svelte.ts';
  import { resolveKind } from '../style.ts';
  import type { NodeType } from '../types.ts';
  import Icon from './Icon.svelte';

  const store = getStore();
  const provider = $derived(store.project?.provider ?? 'aws');

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
      {@const style = resolveKind(entry.type, store.config, provider)}
      <li
        class="flex cursor-grab flex-col items-center gap-1.5 rounded-lg border border-border bg-raised px-1.5 pt-2.5 pb-2 hover:border-border-strong active:cursor-grabbing"
        draggable="true"
        data-node-type={entry.type}
        title={entry.description}
        ondragstart={(event) => start(event, entry.type)}
      >
        <Icon icon={style.icon} color={style.color} shape={style.shape} size={32} />
        <span class="text-xs">{entry.label}</span>
      </li>
    {/each}
  </ul>
  <p class="mx-1.5 mt-1 text-[11px] text-faint">Drag a type onto the canvas.</p>
</section>
