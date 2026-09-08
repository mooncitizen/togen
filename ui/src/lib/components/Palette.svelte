<script lang="ts">
  import { groupsFor, matches, type CatalogueEntry } from '../catalogue.ts';
  import { getStore } from '../store.svelte.ts';
  import { resolveKind } from '../style.ts';
  import type { NodeType } from '../types.ts';
  import Icon from './Icon.svelte';

  const store = getStore();
  const provider = $derived(store.project?.provider ?? 'aws');

  let query = $state('');
  let toggled = $state(new Set<string>());

  const groups = $derived(
    groupsFor(provider)
      .map((group) => ({ ...group, entries: group.entries.filter((e) => matches(e, query)) }))
      .filter((group) => group.entries.length > 0),
  );
  const searching = $derived(query.trim() !== '');
  const found = $derived(groups.reduce((total, group) => total + group.entries.length, 0));

  // Groups start open and a search opens them all; toggling flips one from its default.
  function open(group: string): boolean {
    return searching || !toggled.has(group);
  }

  function toggle(group: string) {
    const next = new Set(toggled);
    if (next.has(group)) {
      next.delete(group);
    } else {
      next.add(group);
    }
    toggled = next;
  }

  function start(event: DragEvent, type: NodeType) {
    if (event.dataTransfer === null) {
      return;
    }
    event.dataTransfer.setData('application/togen-node', type);
    event.dataTransfer.effectAllowed = 'copy';
  }

  function title(entry: CatalogueEntry): string {
    const drawn = entry.tier === 'draws' ? ' Draws only: it generates nothing.' : '';
    return `${entry.resource}. ${entry.description}${drawn}`;
  }
</script>

<section class="flex flex-col gap-2 border-t border-border px-2.5 py-3">
  <h2 class="px-1.5 text-[11px] font-semibold tracking-[.06em] text-muted uppercase">Palette</h2>
  <input
    type="search"
    bind:value={query}
    placeholder="Search services"
    aria-label="Search the palette"
    data-palette-search
    class="mx-0.5 rounded-md border border-border bg-raised px-2 py-1 text-xs placeholder:text-faint focus:border-border-strong focus:outline-none"
  />
  {#each groups as group (group.group)}
    {@const shown = open(group.group)}
    <div class="flex flex-col gap-1.5">
      <button
        type="button"
        class="flex items-center justify-between px-1.5 text-[11px] text-muted hover:text-fg"
        aria-expanded={shown}
        data-palette-group={group.group}
        onclick={() => toggle(group.group)}
      >
        <span>{group.group}</span>
        <span class="text-faint">{group.entries.length}</span>
      </button>
      {#if shown}
        <ul class="grid grid-cols-2 gap-1.5">
          {#each group.entries as entry (entry.type)}
            {@const style = resolveKind(entry.type, store.config, provider)}
            <li
              class="relative flex cursor-grab flex-col items-center gap-1.5 rounded-lg border border-border bg-raised px-1.5 pt-2.5 pb-2 hover:border-border-strong active:cursor-grabbing"
              draggable="true"
              data-node-type={entry.type}
              data-tier={entry.tier}
              title={title(entry)}
              ondragstart={(event) => start(event, entry.type)}
            >
              <Icon icon={style.icon} color={style.color} shape={style.shape} size={32} />
              <span class="text-xs">{entry.label}</span>
              {#if entry.tier === 'draws'}
                <span
                  class="absolute top-1 right-1 rounded bg-sunken px-1 text-[9px] text-faint"
                  data-draws-badge>draws</span
                >
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/each}
  {#if found === 0}
    <p class="mx-1.5 text-[11px] text-faint">Nothing on {provider} matches "{query.trim()}".</p>
  {:else}
    <p class="mx-1.5 mt-1 text-[11px] text-faint">Drag a type onto the canvas.</p>
  {/if}
</section>
