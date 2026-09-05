<script lang="ts">
  import { methodOptions, pathField, reasonFor } from '../form.ts';
  import { getStore } from '../store.svelte.ts';
  import type { Edge } from '../types.ts';

  let { edge }: { edge: Edge } = $props();

  const store = getStore();
  const path = pathField();
  const methods = methodOptions();
  const from = $derived(named(edge.from));
  const to = $derived(named(edge.to));
  const chosen = $derived(set());

  let reason = $state<string | undefined>(undefined);

  function named(id: string): string {
    return store.project?.nodes.find((node) => node.id === id)?.name ?? id;
  }

  function set(): string[] {
    const value = edge.properties?.methods;
    return Array.isArray(value) ? value.map(String) : [];
  }

  function setPath(value: string) {
    reason = reasonFor(path, value);
    if (reason === undefined) {
      store.updateEdge(edge.id, { path: value === '' ? undefined : value });
    }
  }

  // ANY already means every method, so it and a named method cannot both be set,
  // and none at all is the schema default and stays out of the file (ADR 0004).
  function setMethod(method: string, on: boolean) {
    let next: string[];
    if (!on) {
      next = chosen.filter((held) => held !== method);
    } else if (method === 'ANY') {
      next = ['ANY'];
    } else {
      next = [...chosen.filter((held) => held !== 'ANY'), method];
    }
    const ordered = methods.filter((candidate) => next.includes(candidate));
    store.updateEdge(edge.id, { methods: ordered.length === 0 ? undefined : ordered });
  }
</script>

<div class="flex flex-col gap-3.5 p-4">
  <div class="flex flex-col gap-1">
    <span class="text-xs font-medium">Endpoints</span>
    <p class="text-[13px]">{from} → {to}</p>
    <p class="text-[11px] text-faint">The {edge.relation} edge between these two.</p>
  </div>

  {#if edge.relation === 'routes'}
    <div class="flex flex-col gap-1">
      <label class="text-xs font-medium" for="{edge.id}-path">{path.label}</label>
      <input
        id="{edge.id}-path"
        class="h-8 rounded-md border border-border-strong bg-input px-2.5 text-[13px]"
        type="text"
        pattern={path.pattern}
        placeholder={path.placeholder}
        value={String(edge.properties?.path ?? '')}
        oninput={(event) => setPath(event.currentTarget.value)}
      />
      {#if reason}<p class="text-xs text-err">{reason}</p>{/if}
      <p class="text-[11px] text-faint">{path.description}</p>
    </div>

    <div class="flex flex-col gap-1.5">
      <span class="text-xs font-medium">Methods</span>
      <div class="flex flex-wrap gap-x-3 gap-y-1">
        {#each methods as method (method)}
          <label class="flex items-center gap-1 text-xs" for="{edge.id}-{method}">
            <input
              id="{edge.id}-{method}"
              type="checkbox"
              checked={chosen.includes(method)}
              onchange={(event) => setMethod(method, event.currentTarget.checked)}
            />
            {method}
          </label>
        {/each}
      </div>
      <p class="text-[11px] text-faint">HTTP methods the route accepts. None means ANY.</p>
    </div>
  {/if}
</div>
