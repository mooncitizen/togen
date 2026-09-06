<script lang="ts">
  import { boundariesFor, cardHeight, cardWidth, frameFor } from '../boundary.ts';
  import {
    defaults,
    describeSize,
    fileName,
    margin,
    pageFor,
    renderView,
    save,
    settled,
    sizeFor,
    titleAt,
    type Background,
    type Format,
    type Options,
    type Scale,
  } from '../export.ts';
  import { getStore } from '../store.svelte.ts';
  import type { Resolved } from '../style.ts';

  type Choice = { value: string; label: string; disabled?: boolean; note?: string };

  let { canvas, onclose }: { canvas: HTMLElement | undefined; onclose: () => void } = $props();

  const store = getStore();

  const formats: Choice[] = [
    { value: 'png', label: 'PNG' },
    { value: 'jpeg', label: 'JPEG' },
  ];
  const scales: Choice[] = [
    { value: '1', label: '1x' },
    { value: '2', label: '2x' },
    { value: '3', label: '3x' },
  ];
  const box = { width: 440, height: 150, pad: 10 };
  const checker =
    'background-image: repeating-conic-gradient(var(--togen-raised) 0 25%, var(--togen-panel) 0 50%); background-size: 12px 12px';

  let options = $state<Options>({ ...defaults });
  let busy = $state(false);
  let failure = $state<string | null>(null);
  let dialog = $state<HTMLDivElement | null>(null);

  const project = $derived(store.project);
  const view = $derived(store.view);
  const visible = $derived(store.visibleNodeIds);
  const frame = $derived(
    project === null ? null : frameFor(project, visible, store.positions, project.provider),
  );
  const size = $derived(frame === null ? null : sizeFor(frame, options));
  const label = $derived(options.format === 'png' ? 'PNG' : 'JPEG');
  const grounds: Choice[] = $derived([
    { value: 'theme', label: 'Theme' },
    {
      value: 'transparent',
      label: 'Transparent',
      disabled: options.format === 'jpeg',
      note: options.format === 'jpeg' ? 'PNG only' : undefined,
    },
    { value: 'white', label: 'White' },
  ]);

  // The page at 1x shrunk to fit the box, with the cards as tinted rectangles
  // and the boundaries as outlines, from the layout rather than a render.
  const preview = $derived.by(() => {
    if (project === null || frame === null) {
      return null;
    }
    const page = pageFor(frame, options);
    const fit = Math.min(
      (box.width - box.pad * 2) / page.width,
      (box.height - box.pad * 2) / page.height,
    );
    const left = (box.width - page.width * fit) / 2;
    const top = (box.height - page.height * fit) / 2;
    const at = (x: number, y: number) => ({
      x: left + (margin + x - frame.x) * fit,
      y: top + (page.top + y - frame.y) * fit,
    });
    return {
      fit,
      title: { x: left + titleAt.x * fit, y: top + titleAt.y * fit },
      boundaries: boundariesFor(project, visible, store.positions, project.provider).map(
        (boundary) => ({
          id: boundary.id,
          color: boundary.color,
          ...at(boundary.rect.x, boundary.rect.y),
          width: boundary.rect.width * fit,
          height: boundary.rect.height * fit,
        }),
      ),
      cards: store.flowNodes
        .filter((node) => visible.has(node.id))
        .map((node) => ({
          id: node.id,
          color: (node.data as { style: Resolved }).style.color,
          ...at(node.position.x, node.position.y),
          width: cardWidth * fit,
          height: cardHeight * fit,
        })),
    };
  });

  $effect(() => {
    const opener = document.activeElement;
    dialog?.focus();
    return () => {
      if (opener instanceof HTMLElement) {
        opener.focus();
      }
    };
  });

  function pickFormat(value: string) {
    options = settled({ ...options, format: value as Format });
  }

  function pickScale(value: string) {
    options.scale = Number(value) as Scale;
  }

  function pickGround(value: string) {
    options.background = value as Background;
  }

  // The inspector closes on Escape from the window, and the dialog is the
  // nearer thing to dismiss while it is open.
  function keydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      event.stopPropagation();
      onclose();
    }
  }

  function backdrop(event: MouseEvent) {
    if (event.target === event.currentTarget) {
      onclose();
    }
  }

  async function run() {
    if (project === null || frame === null) {
      return;
    }
    busy = true;
    failure = null;
    try {
      const blob = await renderView(canvas, frame, { name: view.name, nodes: visible }, options);
      save(blob, fileName(project.name, view.id, options.format));
      onclose();
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error);
    } finally {
      busy = false;
    }
  }
</script>

{#snippet segmented(by: string, choices: Choice[], current: string, pick: (value: string) => void)}
  <div
    class="inline-flex rounded-[7px] border border-border bg-raised p-0.5"
    role="radiogroup"
    aria-labelledby={by}
  >
    {#each choices as choice (choice.value)}
      <button
        type="button"
        class="h-[26px] rounded-[5px] px-3 text-xs font-medium disabled:cursor-not-allowed disabled:opacity-40 {choice.value ===
        current
          ? 'bg-panel text-text shadow-[0_1px_2px_rgba(0,0,0,.25)]'
          : 'text-muted enabled:hover:text-text'}"
        role="radio"
        aria-checked={choice.value === current}
        disabled={choice.disabled === true}
        title={choice.note}
        onclick={() => pick(choice.value)}>{choice.label}</button
      >
    {/each}
  </div>
{/snippet}

<div
  class="fixed inset-0 z-50 flex items-center justify-center bg-[rgba(0,0,0,.45)]"
  role="presentation"
  onclick={backdrop}
  onkeydown={keydown}
>
  <div
    bind:this={dialog}
    class="flex w-[480px] flex-col gap-4 rounded-xl border border-border-strong bg-panel p-5 shadow-panel outline-none"
    role="dialog"
    aria-modal="true"
    aria-labelledby="export-heading"
    tabindex="-1"
  >
    <header class="flex items-start justify-between gap-3">
      <div class="flex min-w-0 flex-col leading-tight">
        <h2 id="export-heading" class="truncate text-[15px] font-semibold">Export {view.name}</h2>
        <p class="text-xs text-muted">The view as drawn, boundary and labels included.</p>
      </div>
      <button
        class="inline-flex h-[30px] w-[30px] shrink-0 items-center justify-center rounded-md text-muted hover:bg-raised hover:text-text"
        aria-label="Close export"
        onclick={onclose}
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
    <div
      class="relative h-[150px] overflow-hidden rounded-lg border border-border {options.background ===
      'white'
        ? 'bg-white'
        : 'bg-canvas'}"
      style={options.background === 'transparent' ? checker : ''}
      data-exporting={options.background}
      role="img"
      aria-label="Preview"
    >
      {#if preview === null}
        <span class="absolute inset-0 flex items-center justify-center text-xs text-muted"
          >No nodes in this view</span
        >
      {:else}
        {#each preview.boundaries as boundary (boundary.id)}
          <span
            class="absolute border border-dashed"
            style="left: {boundary.x}px; top: {boundary.y}px; width: {boundary.width}px; height: {boundary.height}px; border-radius: {14 *
              preview.fit}px; border-color: color-mix(in srgb, {boundary.color} 60%, transparent); background: color-mix(in srgb, {boundary.color} 4%, transparent)"
          ></span>
        {/each}
        {#each preview.cards as card (card.id)}
          <span
            class="absolute rounded-[3px] border"
            data-preview-node={card.id}
            style="left: {card.x}px; top: {card.y}px; width: {card.width}px; height: {card.height}px; border-color: color-mix(in srgb, {card.color} var(--togen-border-alpha), transparent); background: color-mix(in srgb, {card.color} var(--togen-tint-alpha), transparent)"
          ></span>
        {/each}
        {#if options.title}
          <span
            class="absolute font-mono text-[11px] font-medium whitespace-nowrap text-text"
            style="left: {preview.title.x}px; top: {preview.title.y}px">{view.name}</span
          >
        {/if}
      {/if}
    </div>
    <div class="flex flex-col gap-3">
      <div class="flex items-center gap-3">
        <span id="export-format" class="w-24 shrink-0 text-xs text-muted">Format</span>
        {@render segmented('export-format', formats, options.format, pickFormat)}
      </div>
      <div class="flex items-center gap-3">
        <span id="export-scale" class="w-24 shrink-0 text-xs text-muted">Scale</span>
        {@render segmented('export-scale', scales, String(options.scale), pickScale)}
        <output class="text-[11px] text-faint" for="export-scale"
          >{size === null ? 'nothing to draw' : describeSize(size)}</output
        >
      </div>
      <div class="flex items-center gap-3">
        <span id="export-background" class="w-24 shrink-0 text-xs text-muted">Background</span>
        {@render segmented('export-background', grounds, options.background, pickGround)}
        <span class="text-[11px] text-faint">transparent is PNG only</span>
      </div>
      <div class="flex items-center gap-3">
        <span id="export-title" class="w-24 shrink-0 text-xs text-muted">Title</span>
        <button
          type="button"
          class="relative h-5 w-[34px] shrink-0 rounded-full {options.title
            ? 'bg-accent'
            : 'bg-border-strong'}"
          role="switch"
          aria-checked={options.title}
          aria-labelledby="export-title"
          onclick={() => (options.title = !options.title)}
        >
          <span
            class="absolute top-0.5 h-4 w-4 rounded-full bg-white transition-[left] {options.title
              ? 'left-4'
              : 'left-0.5'}"
          ></span>
        </button>
        <span class="text-[11px] text-faint">view name in the top-left corner</span>
      </div>
    </div>
    <footer class="flex items-center gap-2 border-t border-border pt-4">
      {#if failure !== null}
        <p class="min-w-0 flex-1 truncate text-xs text-err" role="alert" title={failure}>
          {failure}
        </p>
      {/if}
      <button
        class="ml-auto inline-flex h-[30px] shrink-0 items-center rounded-md border border-border-strong bg-raised px-3 font-medium hover:bg-panel"
        onclick={onclose}>Cancel</button
      >
      <button
        class="inline-flex h-[30px] shrink-0 items-center rounded-md border border-accent bg-accent px-3 font-medium text-accent-text hover:opacity-90 disabled:opacity-40 disabled:hover:opacity-40"
        disabled={busy || size === null}
        aria-busy={busy}
        onclick={() => void run()}>{busy ? 'Rendering' : `Export ${label}`}</button
      >
    </footer>
  </div>
</div>
