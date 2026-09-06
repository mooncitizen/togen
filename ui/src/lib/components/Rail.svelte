<script lang="ts">
  import { getStore } from '../store.svelte.ts';
  import About from './About.svelte';
  import Palette from './Palette.svelte';

  const store = getStore();
</script>

<aside
  class="flex w-60 shrink-0 flex-col overflow-y-auto border-r border-border bg-panel"
  aria-label="Rail"
>
  <section class="flex flex-col gap-1 px-2.5 pt-3 pb-2">
    <h2 class="px-1.5 pb-1 text-[11px] font-semibold tracking-[.06em] text-muted uppercase">
      Views
    </h2>
    <ul class="flex flex-col gap-0.5">
      <li class="flex h-8 items-center gap-2 rounded-md bg-raised px-2.5 font-medium">
        <svg
          class="text-accent"
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.75"
          stroke-linecap="round"
          stroke-linejoin="round"
          aria-hidden="true"
        >
          <rect x="3" y="4" width="7" height="6" rx="1.5" />
          <rect x="14" y="14" width="7" height="6" rx="1.5" />
          <path d="M10 7h4v10" />
        </svg>
        <span class="flex-1 truncate">Overview</span>
        <span class="font-mono text-[11px] text-faint">{store.project?.nodes.length ?? 0}</span>
      </li>
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
