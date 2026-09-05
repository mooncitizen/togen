<script lang="ts">
  import { untrack } from 'svelte';

  import type { Field } from '../form.ts';

  type Row = { key: string; value: string };

  let {
    field,
    values,
    onchange,
  }: {
    field: Field;
    values: Record<string, string>;
    onchange: (next: Record<string, string> | undefined) => void;
  } = $props();

  const reason = 'capitals, digits and underscores, starting with a letter';
  const allowed = $derived(field.pattern === undefined ? undefined : new RegExp(field.pattern));

  // A half-typed row is the editor's own state, so the rows are seeded once and
  // the node's env follows them, not the other way round.
  let rows = $state<Row[]>(
    untrack(() => Object.entries(values).map(([key, value]) => ({ key, value: String(value) }))),
  );

  // No pattern means nothing to check against, not a pattern that matches everything.
  function bad(row: Row): boolean {
    return row.key !== '' && allowed !== undefined && !allowed.test(row.key);
  }

  function emit() {
    const next: Record<string, string> = {};
    for (const row of rows) {
      if (row.key !== '' && !bad(row)) {
        next[row.key] = row.value;
      }
    }
    onchange(Object.keys(next).length === 0 ? undefined : next);
  }

  function edit(index: number, part: keyof Row, value: string) {
    rows[index][part] = value;
    emit();
  }
</script>

<div class="flex flex-col gap-1.5">
  <span class="text-xs font-medium text-stone-700">{field.label}</span>
  {#each rows as row, index (index)}
    <div class="flex items-center gap-1">
      <input
        class="w-1/2 min-w-0 rounded border px-1.5 py-1 font-mono text-xs {bad(row)
          ? 'border-red-500'
          : 'border-stone-300'}"
        aria-label="Env key {index + 1}"
        placeholder="NAME"
        value={row.key}
        oninput={(event) => edit(index, 'key', event.currentTarget.value)}
      />
      <input
        class="w-1/2 min-w-0 rounded border border-stone-300 px-1.5 py-1 text-xs"
        aria-label="Env value {index + 1}"
        placeholder="value"
        value={row.value}
        oninput={(event) => edit(index, 'value', event.currentTarget.value)}
      />
      <button
        class="shrink-0 rounded px-1 text-stone-500 hover:bg-stone-100 hover:text-stone-900"
        aria-label="Remove env row {index + 1}"
        onclick={() => {
          rows.splice(index, 1);
          emit();
        }}>×</button
      >
    </div>
    {#if bad(row)}
      <p class="text-xs text-red-700">{row.key} is not a valid name: {reason}</p>
    {/if}
  {/each}
  <button
    class="self-start rounded border border-stone-300 px-1.5 py-0.5 text-xs hover:bg-stone-50"
    onclick={() => rows.push({ key: '', value: '' })}>Add variable</button
  >
  <p class="text-xs text-stone-500">{field.description}</p>
</div>
