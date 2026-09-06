<script lang="ts">
  import { getTheme } from '../theme.svelte.ts';

  const theme = getTheme();
  const label = $derived(theme.scheme === 'dark' ? 'Switch to light' : 'Switch to dark');
</script>

<button
  class="inline-flex h-[30px] w-[30px] shrink-0 items-center justify-center rounded-md border border-transparent text-muted hover:bg-raised hover:text-text"
  aria-label={label}
  title={label}
  onclick={() => theme.toggle()}
>
  {#if theme.scheme === 'dark'}
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.75"
      stroke-linecap="round"
    >
      <circle cx="12" cy="12" r="4" />
      <path
        d="M12 2v2M12 20v2M2 12h2M20 12h2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"
      />
    </svg>
  {:else}
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.75"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <path d="M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5z" />
    </svg>
  {/if}
</button>
