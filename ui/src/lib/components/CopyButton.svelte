<script lang="ts">
  import { onDestroy } from 'svelte';
  import { fade } from 'svelte/transition';

  let { text, label }: { text: string; label: string } = $props();

  const shownFor = 1500;

  let said = $state<string | null>(null);
  let timer: ReturnType<typeof setTimeout> | undefined;

  async function copy() {
    said = (await write(text)) ? 'Copied' : 'Clipboard unavailable';
    clearTimeout(timer);
    timer = setTimeout(() => {
      said = null;
    }, shownFor);
  }

  // The clipboard is only there on a secure origin with permission, and the
  // studio is served over plain http on a machine the user may not own.
  async function write(value: string): Promise<boolean> {
    const clipboard = navigator.clipboard;
    if (clipboard === undefined) {
      return false;
    }
    try {
      await clipboard.writeText(value);
      return true;
    } catch {
      return false;
    }
  }

  onDestroy(() => clearTimeout(timer));
</script>

<span class="inline-flex items-center gap-2">
  {#if said !== null}
    <span class="text-[11px] text-muted" role="status" out:fade={{ duration: 300 }}>{said}</span>
  {/if}
  <button
    class="inline-flex h-[26px] items-center gap-1.5 rounded-md border border-transparent px-2 text-xs font-medium text-muted hover:bg-raised hover:text-text"
    aria-label={label}
    title={label}
    onclick={() => void copy()}
  >
    <svg
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.75"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      <rect x="9" y="9" width="11" height="11" rx="2" />
      <path d="M5 15V6a2 2 0 0 1 2-2h9" />
    </svg>
    Copy
  </button>
</span>
