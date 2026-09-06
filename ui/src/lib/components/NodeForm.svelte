<script lang="ts">
  import {
    engineVersions,
    fieldsFor,
    nameField,
    reasonFor,
    versionField,
    type Field,
  } from '../form.ts';
  import { getStore } from '../store.svelte.ts';
  import type { Node } from '../types.ts';
  import EnvEditor from './EnvEditor.svelte';

  let { node }: { node: Node } = $props();

  const store = getStore();
  const name = nameField();
  const engineDefault = fieldsFor('database').find((field) => field.key === 'engine')?.default;
  const provider = $derived(store.project?.provider ?? 'aws');
  const fields = $derived(fieldsFor(node.type).map(resolve));

  // A reason keyed by field, shown under the control until the value is fixed;
  // nothing is saved while one is set.
  let reasons = $state<Record<string, string | undefined>>({});

  function resolve(field: Field): Field {
    if (node.type !== 'database' || field.key !== 'version') {
      return field;
    }
    return versionField(field, provider, node.properties?.engine ?? engineDefault);
  }

  function id(field: Field): string {
    return `${node.id}-${field.key}`;
  }

  // A default that is also one of the options would read the same twice, so the
  // empty row says which one leaves the field unset.
  function unset(field: Field): string {
    return field.options?.includes(field.placeholder)
      ? `${field.placeholder} (default)`
      : field.placeholder;
  }

  function held(key: string): unknown {
    return node.properties?.[key];
  }

  function shown(field: Field): string {
    const value = held(field.key);
    return value === undefined || value === null ? '' : String(value);
  }

  function ticked(field: Field): boolean {
    const value = held(field.key);
    return typeof value === 'boolean' ? value : field.default === true;
  }

  function env(field: Field): Record<string, string> {
    const value = held(field.key);
    return typeof value === 'object' && value !== null ? (value as Record<string, string>) : {};
  }

  function set(key: string, value: unknown) {
    store.updateNode(node.id, { properties: { [key]: value } });
  }

  // Half a value is not a value: pausing after the dash in 'orders-db', or on a
  // storage size below the minimum, would otherwise send it, be refused, and
  // take the rest of the typing with it. An invalid value shows why instead.
  function setName(value: string) {
    const reason = reasonFor(name, value);
    reasons = { ...reasons, name: reason };
    if (reason === undefined) {
      store.updateNode(node.id, { name: value });
    }
  }

  function setText(field: Field, value: string) {
    const reason = reasonFor(field, value);
    reasons = { ...reasons, [field.key]: reason };
    if (reason === undefined) {
      set(field.key, value === '' ? undefined : value);
    }
  }

  function setNumber(field: Field, value: string) {
    const reason = reasonFor(field, value);
    reasons = { ...reasons, [field.key]: reason };
    if (reason === undefined) {
      set(field.key, value === '' ? undefined : Number(value));
    }
  }

  // A toggle has no empty state, so a value back at the schema default is dropped
  // rather than written: the file records only what was set (ADR 0004).
  function setToggle(field: Field, value: boolean) {
    set(field.key, value === field.default ? undefined : value);
  }

  function setSelect(field: Field, value: string) {
    const chosen = value === '' ? undefined : value;
    if (node.type === 'database' && field.key === 'engine') {
      store.updateNode(node.id, { properties: { engine: chosen, ...unsupported(chosen) } });
      return;
    }
    set(field.key, chosen);
  }

  // Each engine has its own versions, so one the new engine does not offer goes.
  function unsupported(engine: unknown): Record<string, unknown> {
    const current = node.properties?.version;
    const versions = engineVersions(provider, engine ?? engineDefault);
    if (typeof current !== 'string' || versions === undefined || versions.includes(current)) {
      return {};
    }
    return { version: undefined };
  }
</script>

<div class="flex flex-col gap-3.5 p-4">
  <div class="flex flex-col gap-1">
    <label class="text-xs font-medium" for="{node.id}-name">{name.label}</label>
    <input
      id="{node.id}-name"
      class="h-8 rounded-md border border-border-strong bg-input px-2.5 text-[13px]"
      type="text"
      maxlength={name.maxLength}
      pattern={name.pattern}
      value={node.name}
      oninput={(event) => setName(event.currentTarget.value)}
    />
    {#if reasons.name}<p class="text-xs text-err">{reasons.name}</p>{/if}
    <p class="text-[11px] text-faint">{name.description}</p>
  </div>

  {#each fields as field (field.key)}
    {#if field.kind === 'env'}
      <EnvEditor {field} values={env(field)} onchange={(next) => set(field.key, next)} />
    {:else if field.kind === 'toggle'}
      <div class="flex flex-col gap-1">
        <label class="flex items-center gap-2 text-xs font-medium" for={id(field)}>
          <input
            id={id(field)}
            class="relative h-5 w-[34px] shrink-0 appearance-none rounded-full bg-border-strong transition-colors after:absolute after:top-0.5 after:left-0.5 after:h-4 after:w-4 after:rounded-full after:bg-white after:transition-transform after:content-[''] checked:bg-accent checked:after:translate-x-3.5"
            type="checkbox"
            checked={ticked(field)}
            onchange={(event) => setToggle(field, event.currentTarget.checked)}
          />
          {field.label}
        </label>
        <p class="text-[11px] text-faint">{field.description}</p>
      </div>
    {:else}
      <div class="flex flex-col gap-1">
        <div class="flex items-baseline gap-1.5">
          <label class="text-xs font-medium" for={id(field)}>{field.label}</label>
          {#if field.required}<span class="text-[11px] text-faint">required</span>{/if}
        </div>
        {#if field.kind === 'select'}
          <select
            id={id(field)}
            class="h-8 rounded-md border border-border-strong bg-input px-2.5 text-[13px]"
            value={shown(field)}
            onchange={(event) => setSelect(field, event.currentTarget.value)}
          >
            <option value="">{unset(field)}</option>
            {#each field.options ?? [] as option (option)}
              <option value={option}>{option}</option>
            {/each}
          </select>
        {:else if field.kind === 'number'}
          <input
            id={id(field)}
            class="h-8 rounded-md border border-border-strong bg-input px-2.5 text-[13px]"
            type="number"
            min={field.min}
            max={field.max}
            step={field.step}
            placeholder={field.placeholder}
            required={field.required}
            value={shown(field)}
            oninput={(event) => setNumber(field, event.currentTarget.value)}
          />
        {:else}
          <input
            id={id(field)}
            class="h-8 rounded-md border border-border-strong bg-input px-2.5 text-[13px]"
            type="text"
            minlength={field.minLength}
            maxlength={field.maxLength}
            pattern={field.pattern}
            placeholder={field.placeholder}
            required={field.required}
            value={shown(field)}
            oninput={(event) => setText(field, event.currentTarget.value)}
          />
        {/if}
        {#if reasons[field.key]}<p class="text-xs text-err">{reasons[field.key]}</p>{/if}
        <p class="text-[11px] text-faint">{field.description}</p>
      </div>
    {/if}
  {/each}
</div>
