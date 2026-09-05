<script lang="ts">
  import { tick } from 'svelte';

  import { getStore, overviewId } from '../store.svelte.ts';
  import About from './About.svelte';
  import Palette from './Palette.svelte';
  import ViewsIcon from './ViewsIcon.svelte';

  const store = getStore();

  let naming = $state(false);
  let name = $state('');
  let field = $state<HTMLInputElement | undefined>();

  async function startNaming() {
    naming = true;
    name = '';
    await tick();
    field?.focus();
  }

  function create() {
    if (store.createView(name) !== null) {
      naming = false;
    }
  }

  function keydown(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      event.preventDefault();
      create();
    }
    if (event.key === 'Escape') {
      naming = false;
    }
  }

  function blur() {
    if (name.trim() === '') {
      naming = false;
    }
  }
</script>

<aside
  class="flex w-60 shrink-0 flex-col overflow-y-auto border-r border-border bg-panel"
  aria-label="Rail"
>
  <section class="flex flex-col gap-1 px-2.5 pt-3 pb-2">
    <div class="flex items-center justify-between px-1.5 pb-1">
      <h2 class="text-[11px] font-semibold tracking-[.06em] text-muted uppercase">Views</h2>
      <button
        class="inline-flex h-6 w-6 items-center justify-center rounded-md text-muted hover:bg-raised hover:text-text"
        aria-label="New view"
        title="New view"
        onclick={startNaming}
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
          <path d="M12 5v14M5 12h14" />
        </svg>
      </button>
    </div>
    <ul class="flex flex-col gap-0.5">
      {#each store.views as view (view.id)}
        {@const active = view.id === store.activeView}
        <li
          class="group flex h-8 items-center gap-1 rounded-md pr-1 pl-2.5 {active
            ? 'bg-raised font-medium'
            : 'text-muted hover:bg-raised/60'}"
        >
          <button
            class="flex min-w-0 flex-1 items-center gap-2 text-left"
            aria-current={active ? 'true' : undefined}
            onclick={() => store.openView(view.id)}
          >
            <span class={active ? 'text-accent' : 'text-faint'}><ViewsIcon /></span>
            <span class="flex-1 truncate">{view.name}</span>
            <span class="font-mono text-[11px] text-faint">{store.members(view).size}</span>
          </button>
          <button
            class="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-muted opacity-0 group-hover:opacity-100 hover:bg-panel hover:text-text focus-visible:opacity-100 {active
              ? 'opacity-100'
              : ''}"
            aria-label="Edit {view.name}"
            title="Edit view"
            onclick={() => store.openEditor(view.id)}
          >
            <svg
              width="13"
              height="13"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <path d="M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z" />
            </svg>
          </button>
          {#if view.id !== overviewId}
            <button
              class="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-muted opacity-0 group-hover:opacity-100 hover:bg-panel hover:text-err focus-visible:opacity-100 {active
                ? 'opacity-100'
                : ''}"
              aria-label="Delete {view.name}"
              title="Delete view"
              onclick={() => store.deleteView(view.id)}
            >
              <svg
                width="13"
                height="13"
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
          {/if}
        </li>
      {/each}
      {#if naming}
        <li class="flex h-8 items-center px-1">
          <input
            bind:this={field}
            class="h-7 w-full rounded-md border border-border-strong bg-input px-2 text-[13px]"
            type="text"
            aria-label="View name"
            placeholder="View name"
            bind:value={name}
            onkeydown={keydown}
            onblur={blur}
          />
        </li>
      {/if}
    </ul>
  </section>
  <Palette />
  <div
    class="mt-auto flex items-center justify-between border-t border-border py-2 pr-2.5 pl-4 text-[11px] text-faint"
  >
    <span class="font-mono">togen/project.json</span>
    <About />
  </div>
</aside>
