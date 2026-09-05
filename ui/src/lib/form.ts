import engineTable from '../../../schema/engines.json';
import schema from '../../../schema/project.schema.json';

import type { NodeType, Provider } from './types.ts';

export type FieldKind = 'toggle' | 'select' | 'number' | 'text' | 'env';

export type Field = {
  key: string;
  label: string;
  description: string;
  kind: FieldKind;
  required: boolean;
  placeholder: string;
  default?: unknown;
  options?: string[];
  min?: number;
  max?: number;
  step?: number;
  minLength?: number;
  maxLength?: number;
  integer?: boolean;
  // On an env field this is what a key has to match, not the value.
  pattern?: string;
};

type Spec = {
  type?: string;
  enum?: string[];
  default?: unknown;
  description?: string;
  minimum?: number;
  maximum?: number;
  minLength?: number;
  maxLength?: number;
  pattern?: string;
  propertyNames?: { pattern?: string };
};

type Variant = {
  properties: {
    type: { const: string };
    name: Spec;
    properties?: { properties?: Record<string, Spec>; required?: string[] };
  };
};

const variants = schema.properties.nodes.items.oneOf as unknown as Variant[];
const engines = engineTable as Record<string, Record<string, { versions: string[] }> | undefined>;

export function fieldsFor(type: NodeType): Field[] {
  const bag = variants.find((variant) => variant.properties.type.const === type)?.properties
    .properties;
  const required = new Set(bag?.required ?? []);
  return Object.entries(bag?.properties ?? {}).map(([key, spec]) =>
    toField(key, spec, required.has(key)),
  );
}

export function nameField(): Field {
  return toField('name', variants[0].properties.name, true);
}

export function engineVersions(provider: Provider, engine: unknown): string[] | undefined {
  return engines[provider]?.[String(engine)]?.versions;
}

// The engine table decides which versions exist, so the select is rebuilt when the
// engine changes and stays a plain input where the provider has no table.
export function versionField(field: Field, provider: Provider, engine: unknown): Field {
  const versions = engineVersions(provider, engine);
  if (versions === undefined) {
    return field;
  }
  return { ...field, kind: 'select', options: versions, placeholder: `newest, ${versions[0]}` };
}

function toField(key: string, spec: Spec, required: boolean): Field {
  const kind = kindOf(spec);
  return {
    key,
    label: label(key),
    description: spec.description ?? '',
    kind,
    required,
    placeholder: kind === 'toggle' || kind === 'env' ? '' : text(spec.default),
    default: spec.default,
    options: spec.enum,
    min: spec.minimum,
    max: spec.maximum,
    step: spec.type === 'integer' ? 1 : undefined,
    minLength: spec.minLength,
    maxLength: spec.maxLength,
    integer: spec.type === 'integer',
    pattern: kind === 'env' ? spec.propertyNames?.pattern : spec.pattern,
  };
}

function kindOf(spec: Spec): FieldKind {
  if (spec.type === 'object') {
    return 'env';
  }
  if (spec.type === 'boolean') {
    return 'toggle';
  }
  if (spec.enum !== undefined) {
    return 'select';
  }
  if (spec.type === 'integer' || spec.type === 'number') {
    return 'number';
  }
  return 'text';
}

function label(key: string): string {
  const words = key
    .replace(/([A-Z])/g, ' $1')
    .trim()
    .toLowerCase();
  return words[0].toUpperCase() + words.slice(1);
}

function text(value: unknown): string {
  return value === undefined ? '' : String(value);
}

const patternReasons: Record<string, string> = {
  '^[a-z][a-z0-9]*(-[a-z0-9]+)*$': 'lowercase letters, digits and hyphens, starting with a letter',
  '^[A-Z][A-Z0-9_]*$': 'capitals, digits and underscores, starting with a letter',
};

export function patternReason(pattern: string): string {
  return patternReasons[pattern] ?? `must match ${pattern}`;
}

// One reason, one wording, whether the check runs here or the studio refuses the
// save: pattern, length, range or required.
export function reasonFor(field: Field, raw: string): string | undefined {
  if (raw === '') {
    return field.required ? 'required' : undefined;
  }
  if (field.kind === 'number') {
    return numberReason(field, raw);
  }
  if (field.pattern !== undefined && !new RegExp(field.pattern).test(raw)) {
    return patternReason(field.pattern);
  }
  if (field.minLength !== undefined && raw.length < field.minLength) {
    return `at least ${field.minLength} characters`;
  }
  if (field.maxLength !== undefined && raw.length > field.maxLength) {
    return `at most ${field.maxLength} characters`;
  }
  return undefined;
}

function numberReason(field: Field, raw: string): string | undefined {
  const parsed = Number(raw);
  if (!Number.isFinite(parsed)) {
    return 'not a number';
  }
  if (field.integer === true && !Number.isInteger(parsed)) {
    return 'a whole number';
  }
  if (field.min !== undefined && field.max !== undefined) {
    return parsed < field.min || parsed > field.max
      ? `between ${field.min} and ${field.max}`
      : undefined;
  }
  if (field.min !== undefined) {
    return parsed < field.min ? `at least ${field.min}` : undefined;
  }
  if (field.max !== undefined) {
    return parsed > field.max ? `at most ${field.max}` : undefined;
  }
  return undefined;
}
