<script lang="ts">
  import { onMount } from 'svelte';
  import { SvelteFlowProvider } from '@xyflow/svelte';

  import { events } from './lib/api.ts';
  import Canvas from './lib/components/Canvas.svelte';
  import Palette from './lib/components/Palette.svelte';
  import TopBar from './lib/components/TopBar.svelte';
  import { Store, setStore } from './lib/store.svelte.ts';

  const store = new Store();
  setStore(store);

  onMount(() => {
    void store.load();
    return events((name) => {
      if (name === 'project-changed' || name === 'layout-changed') {
        store.reload();
      }
    });
  });
</script>

<SvelteFlowProvider>
  <div class="flex h-full flex-col bg-stone-50 text-stone-900">
    <TopBar />
    <main class="flex min-h-0 flex-1">
      <Palette />
      <Canvas />
    </main>
  </div>
</SvelteFlowProvider>
