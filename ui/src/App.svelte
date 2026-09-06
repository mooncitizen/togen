<script lang="ts">
  import { onMount } from 'svelte';
  import { SvelteFlowProvider } from '@xyflow/svelte';

  import { events } from './lib/api.ts';
  import Canvas from './lib/components/Canvas.svelte';
  import Inspector from './lib/components/Inspector.svelte';
  import Rail from './lib/components/Rail.svelte';
  import TopBar from './lib/components/TopBar.svelte';
  import ViewEditor from './lib/components/ViewEditor.svelte';
  import { Store, setStore } from './lib/store.svelte.ts';
  import { Theme, setTheme } from './lib/theme.svelte.ts';

  const store = new Store();
  setStore(store);
  const theme = new Theme(() => store.config?.style?.theme ?? 'dark');
  setTheme(theme);

  onMount(() => {
    void store.load();
    const detach = theme.attach();
    const stop = events((name) => {
      if (name === 'project-changed' || name === 'layout-changed' || name === 'views-changed') {
        store.reload();
      }
      if (name === 'config-changed') {
        void store.loadConfig();
      }
    });
    return () => {
      stop();
      detach();
    };
  });
</script>

<SvelteFlowProvider>
  <div class="flex h-full flex-col bg-canvas text-text">
    <TopBar />
    <main class="flex min-h-0 flex-1">
      <Rail />
      <Canvas />
      {#if store.editing}
        <ViewEditor />
      {:else}
        <Inspector />
      {/if}
    </main>
  </div>
</SvelteFlowProvider>
