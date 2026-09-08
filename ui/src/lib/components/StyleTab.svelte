<script lang="ts">
  import { entryFor } from '../catalogue.ts';
  import { kindsSnippet, nodesSnippet } from '../snippet.ts';
  import { getStore } from '../store.svelte.ts';
  import { resolveKind, resolveStyle, type Source } from '../style.ts';
  import type { Node } from '../types.ts';
  import CopyButton from './CopyButton.svelte';
  import Icon from './Icon.svelte';

  let { node }: { node: Node } = $props();

  const store = getStore();
  const provider = $derived(store.project?.provider ?? 'aws');
  const style = $derived(resolveStyle(node, store.config, provider));
  // What every node of the type gets, without this node's own overrides.
  const kind = $derived(resolveKind(node.type, store.config, provider));
  const forNode = $derived(nodesSnippet(node, style));
  const forKind = $derived(kindsSnippet(node.type, kind));
  const entry = $derived(entryFor(provider, node.type));

  const sources: Record<Source, string> = {
    node: 'node override',
    kind: 'kind override',
    scheme: 'provider scheme',
  };
</script>

<div class="flex flex-col gap-4 p-4">
  <div class="flex items-center gap-3">
    <Icon icon={style.icon} color={style.color} shape={style.shape} size={44} />
    <span class="flex min-w-0 flex-col leading-tight">
      <span class="truncate font-medium">{node.name}</span>
      <span class="text-[11px] text-muted" data-node-resource>
        {node.type}{entry === undefined ? '' : ` · ${entry.resource}`}
      </span>
      {#if entry?.tier === 'draws'}
        <span class="text-[11px] text-faint" data-draws-note>
          Drawn only. Generate writes nothing for this node.
        </span>
      {/if}
    </span>
  </div>

  <section class="flex flex-col">
    <h2 class="pb-1 text-[11px] font-semibold tracking-[.06em] text-muted uppercase">Applied</h2>
    <dl class="flex flex-col">
      <div class="flex h-9 items-center gap-2.5">
        <dt class="w-12 shrink-0 text-xs text-muted">Colour</dt>
        <dd class="flex min-w-0 items-center gap-2 font-mono text-xs">
          <span
            class="h-[18px] w-[18px] shrink-0 rounded-[5px]"
            style="background: {style.color}"
            aria-hidden="true"
          ></span>
          <span class="truncate">{style.color}</span>
        </dd>
        <dd class="ml-auto shrink-0 text-[11px] text-faint">{sources[style.source.color]}</dd>
      </div>
      <div class="flex h-9 items-center gap-2.5">
        <dt class="w-12 shrink-0 text-xs text-muted">Icon</dt>
        <dd class="flex min-w-0 items-center gap-2 font-mono text-xs">
          <Icon icon={style.icon} color={style.color} shape="card" size={22} />
          <span class="truncate" title={style.icon}>{style.icon}</span>
        </dd>
        <dd class="ml-auto shrink-0 text-[11px] text-faint">{sources[style.source.icon]}</dd>
      </div>
      <div class="flex h-9 items-center gap-2.5">
        <dt class="w-12 shrink-0 text-xs text-muted">Shape</dt>
        <dd class="flex min-w-0 items-center font-mono text-xs">{style.shape}</dd>
        <dd class="ml-auto shrink-0 text-[11px] text-faint">{sources[style.source.shape]}</dd>
      </div>
    </dl>
  </section>

  <section class="flex flex-col gap-2 border-t border-border pt-3">
    <div class="flex items-center justify-between">
      <h2 class="text-[11px] font-semibold tracking-[.06em] text-muted uppercase">
        Override in togen.yml
      </h2>
      <CopyButton text={forNode} label="Copy the style.nodes snippet" />
    </div>
    <pre
      class="overflow-x-auto rounded-lg border border-border bg-code p-3 font-mono text-xs leading-[1.55]"
      data-snippet="nodes">{forNode}</pre>
  </section>

  <section class="flex flex-col gap-2 border-t border-border pt-3">
    <div class="flex items-center justify-between">
      <h2 class="text-[11px] font-semibold tracking-[.06em] text-muted uppercase">
        Or for every {node.type}
      </h2>
      <CopyButton text={forKind} label="Copy the style.kinds snippet" />
    </div>
    <pre
      class="overflow-x-auto rounded-lg border border-border bg-code p-3 font-mono text-xs leading-[1.55]"
      data-snippet="kinds">{forKind}</pre>
    <p class="text-xs text-muted">
      Paste this into <code class="font-mono">togen.yml</code>, next to
      <code class="font-mono">togen/</code>, and change what you want. The studio reads that file
      and never writes it; this panel follows as soon as you save.
    </p>
  </section>
</div>
