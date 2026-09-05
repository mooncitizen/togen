<script lang="ts">
  import { iconSets } from '../icons.ts';

  let open = $state(false);

  function keydown(event: KeyboardEvent) {
    if (open && event.key === 'Escape') {
      open = false;
    }
  }
</script>

<svelte:window onkeydown={keydown} />

<button
  class="rounded-md px-1.5 py-0.5 text-[11px] text-faint hover:bg-raised hover:text-text"
  onclick={() => (open = true)}>About</button
>

{#if open}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-[rgba(0,0,0,.45)] p-6"
    onclick={(event) => {
      if (event.target === event.currentTarget) {
        open = false;
      }
    }}
    role="presentation"
  >
    <div
      class="flex max-h-full w-[36rem] max-w-full flex-col rounded-lg border border-border bg-panel shadow-panel"
      role="dialog"
      aria-modal="true"
      aria-label="About"
    >
      <header class="flex h-12 shrink-0 items-center gap-2 border-b border-border pr-2 pl-4">
        <span class="text-sm font-semibold">Togen</span>
        <span class="text-xs text-muted">studio</span>
        <button
          class="ml-auto inline-flex h-[30px] w-[30px] items-center justify-center rounded-md text-muted hover:bg-raised hover:text-text"
          aria-label="Close about"
          onclick={() => (open = false)}
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
      <div class="min-h-0 flex-1 overflow-y-auto px-4 py-3 text-xs">
        <p>
          Togen turns a sketch of an application into infrastructure code. The studio is its
          canvas: it reads and writes the project files under <code class="font-mono">togen/</code>
          and reads <code class="font-mono">togen.yml</code> for the look.
        </p>
        <h2 class="mt-4 text-[11px] font-semibold tracking-[.06em] text-muted uppercase">
          Icons
        </h2>
        <p class="mt-1">
          The node icons are the providers' own architecture icon sets, bundled under each
          vendor's terms.
        </p>
        <ul class="mt-2 flex flex-col gap-3" aria-label="Icon licences">
          {#each iconSets as set (set.name)}
            <li class="flex flex-col gap-2 rounded-md border border-border bg-raised p-3">
              <h3 class="font-medium">{set.name}</h3>
              {#each set.notice as paragraph, index (index)}
                {#if paragraph.quote}
                  <blockquote class="border-l-2 border-border-strong pl-2.5 text-muted italic">
                    {paragraph.text}
                  </blockquote>
                {:else}
                  <p class="text-muted">{paragraph.text}</p>
                {/if}
              {/each}
            </li>
          {/each}
        </ul>
      </div>
    </div>
  </div>
{/if}
