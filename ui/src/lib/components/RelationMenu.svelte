<script lang="ts">
  import type { Position, Relation } from '../types.ts';

  let {
    relations,
    at,
    onchoose,
    oncancel,
  }: {
    relations: Relation[];
    at: Position;
    onchoose: (relation: Relation) => void;
    oncancel: () => void;
  } = $props();

  let menu = $state<HTMLDivElement | null>(null);

  $effect(() => {
    menu?.focus();
  });

  // The inspector closes on Escape from the window, and this menu is the nearer
  // thing to dismiss while it is open.
  function keydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      event.stopPropagation();
      oncancel();
    }
  }
</script>

<button class="absolute inset-0 z-10 cursor-default" aria-label="Cancel" onclick={oncancel}></button>
<div
  bind:this={menu}
  class="absolute z-20 flex min-w-28 flex-col rounded-md border border-border-strong bg-panel py-1 shadow-panel"
  style="left: {at.x}px; top: {at.y}px"
  role="menu"
  aria-label="Relations"
  tabindex="-1"
  onkeydown={keydown}
>
  {#each relations as relation (relation)}
    <button
      class="px-2.5 py-1 text-left font-mono text-xs hover:bg-raised"
      role="menuitem"
      onclick={() => onchoose(relation)}>{relation}</button
    >
  {/each}
</div>
