<script lang="ts">
  import { Background, Controls, MiniMap, SvelteFlow, type Edge, type Node } from '@xyflow/svelte';

  let name = $state('not loaded');
  let nodes = $state.raw<Node[]>([]);
  let edges = $state.raw<Edge[]>([]);

  async function loadName() {
    // Anything short of a project with a name leaves the bar reading "not loaded".
    try {
      const response = await fetch('/api/project');
      if (!response.ok) {
        return;
      }
      const project: { name?: unknown } = await response.json();
      if (typeof project.name === 'string') {
        name = project.name;
      }
    } catch {
      return;
    }
  }

  $effect(() => {
    void loadName();
  });
</script>

<div class="flex h-full flex-col bg-stone-50 text-stone-900">
  <header class="flex items-baseline gap-3 border-b border-stone-200 bg-white px-4 py-2">
    <h1 class="text-sm font-semibold">Togen studio</h1>
    <span class="text-sm text-stone-500">{name}</span>
  </header>
  <main class="min-h-0 flex-1">
    <SvelteFlow bind:nodes bind:edges>
      <Background />
      <Controls />
      <MiniMap />
    </SvelteFlow>
  </main>
</div>
