<script lang="ts">
  import { fly } from 'svelte/transition';

  import { catalogueFor } from '../catalogue.ts';
  import { getStore, overviewId } from '../store.svelte.ts';
  import type { Node } from '../types.ts';
  import ViewsIcon from './ViewsIcon.svelte';

  const store = getStore();
  const view = $derived(store.view);
  const everything = $derived(view.id === overviewId);
  const members = $derived(store.visibleNodeIds);
  const total = $derived(store.project?.nodes.length ?? 0);
  const groups = $derived(
    catalogueFor(store.project?.provider ?? 'aws')
      .map((entry) => ({
        type: entry.type,
        label: entry.label,
        nodes: (store.project?.nodes ?? []).filter((node) => node.type === entry.type),
      }))
      .filter((group) => group.nodes.length > 0),
  );

  let blank = $state(false);

  function rename(value: string) {
    blank = value.trim() === '';
    if (!blank) {
      store.renameView(view.id, value);
    }
  }

  function toggle(node: Node) {
    store.toggleViewNode(view.id, node.id);
  }

  function keydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      store.closeEditor();
    }
  }
</script>

<svelte:window onkeydown={keydown} />

<aside
  class="flex w-80 shrink-0 flex-col border-l border-border bg-panel"
  aria-label="View editor"
  transition:fly={{ x: 24, duration: 120 }}
>
  <header class="flex h-12 shrink-0 items-center gap-2 border-b border-border pr-2 pl-4">
    <span class="text-accent"><ViewsIcon /></span>
    <span class="flex flex-col leading-tight">
      <span class="text-[13px] font-medium">Edit view</span>
      <span class="font-mono text-[11px] text-muted">togen/views.json</span>
    </span>
    <button
      class="ml-auto inline-flex h-[30px] w-[30px] items-center justify-center rounded-md text-muted hover:bg-raised hover:text-text"
      aria-label="Close view editor"
      onclick={() => store.closeEditor()}
    >
      <svg
        width="14"
        height="14"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        aria-hidden="true"
      >
        <path d="M6 6l12 12M18 6 6 18" />
      </svg>
    </button>
  </header>
  <div class="flex min-h-0 flex-1 flex-col gap-3.5 overflow-y-auto p-4">
    {#key view.id}
      <div class="flex flex-col gap-1">
        <label class="text-xs font-medium" for="view-name">Name</label>
        <input
          id="view-name"
          class="h-8 rounded-md border border-border-strong bg-input px-2.5 text-[13px]"
          type="text"
          value={view.name}
          oninput={(event) => rename(event.currentTarget.value)}
        />
        {#if blank}<p class="text-xs text-err">a view needs a name</p>{/if}
      </div>
    {/key}
    <div class="flex flex-col gap-1">
      <div class="flex items-center justify-between">
        <span class="text-xs font-medium">Nodes in this view</span>
        <span class="text-[11px] text-faint">{members.size} of {total}</span>
      </div>
      {#if everything}
        <p class="text-xs text-muted">Show every node. The overview always shows the whole project.</p>
      {/if}
      {#each groups as group (group.type)}
        <h3 class="mt-2 text-[10px] font-semibold tracking-[.06em] text-muted uppercase">
          {group.label}
        </h3>
        <ul class="flex flex-col">
          {#each group.nodes as node (node.id)}
            <li
              class="flex h-8 items-center rounded-md px-1 {members.has(node.id) ? '' : 'opacity-60'}"
              data-node-id={node.id}
            >
              <label class="flex flex-1 items-center gap-2.5 text-[13px]">
                <input
                  class="h-4 w-4 accent-accent"
                  type="checkbox"
                  checked={members.has(node.id)}
                  disabled={everything}
                  onchange={() => toggle(node)}
                />
                <span class="flex-1 truncate">{node.name}</span>
                <span class="font-mono text-[11px] text-faint">{node.id}</span>
              </label>
            </li>
          {/each}
        </ul>
      {/each}
      <p class="mt-1.5 text-[11px] text-faint">
        Edges follow: one is drawn when both ends are in the view. Unticked nodes are dimmed on
        the canvas while you edit.
      </p>
    </div>
  </div>
  <footer class="flex shrink-0 items-center justify-between border-t border-border px-3 py-2.5">
    {#if everything}
      <span class="text-[11px] text-faint">{store.saving ? 'Saving' : 'Saved'}</span>
    {:else}
      <button
        class="inline-flex h-[30px] items-center rounded-md border border-transparent px-3 font-medium text-err hover:bg-raised"
        onclick={() => store.deleteView(view.id)}>Delete view</button
      >
    {/if}
    <button
      class="inline-flex h-[30px] items-center rounded-md border border-accent bg-accent px-3 font-medium text-accent-text hover:opacity-90"
      onclick={() => store.closeEditor()}>Done</button
    >
  </footer>
</aside>
