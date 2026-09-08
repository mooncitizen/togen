import published from '../../../schema/catalogue.json';

import type { NodeType, Provider, Shape } from './types.ts';

export type Tier = 'generates' | 'draws';

export type CatalogueEntry = {
  type: NodeType;
  label: string;
  group: string;
  description: string;
  resource: string;
  tier: Tier;
  aliases: string[];
  style: { color: string; icon: string; shape: Shape };
  properties: Record<string, unknown>;
};

type PublishedEntry = {
  id: string;
  label: string;
  group: string;
  description: string;
  resource: string;
  tier: string;
  aliases?: string[];
  style: { color: string; icon: string; shape: string };
  properties: Record<string, unknown>;
};

const providers = published.providers as Record<string, { entries: PublishedEntry[] }>;

function read(entry: PublishedEntry): CatalogueEntry {
  return {
    type: entry.id as NodeType,
    label: entry.label,
    group: entry.group,
    description: entry.description,
    resource: entry.resource,
    tier: entry.tier as Tier,
    aliases: entry.aliases ?? [],
    style: entry.style as CatalogueEntry['style'],
    properties: entry.properties ?? {},
  };
}

// What the provider offers, in catalogue order, which is the order the palette shows.
export function catalogueFor(provider: Provider): CatalogueEntry[] {
  return (providers[provider]?.entries ?? []).map(read);
}

export function entryFor(provider: Provider, type: NodeType): CatalogueEntry | undefined {
  return catalogueFor(provider).find((entry) => entry.type === type);
}

const known = new Set<string>(
  Object.values(providers).flatMap((p) => p.entries.map((entry) => entry.id)),
);

export function isNodeType(value: string): value is NodeType {
  return known.has(value);
}

// A dropped node has to validate straight away, so any property with no default takes the
// placeholder its schema gives, which the user is expected to replace.
export function defaultProperties(
  provider: Provider,
  type: NodeType,
): Record<string, unknown> {
  const schema = entryFor(provider, type)?.properties as
    | { properties?: Record<string, { default?: unknown; examples?: unknown[] }>; required?: string[] }
    | undefined;
  const out: Record<string, unknown> = {};
  for (const name of schema?.required ?? []) {
    const field = schema?.properties?.[name];
    const placeholder = field?.default ?? field?.examples?.[0];
    if (placeholder !== undefined) {
      out[name] = placeholder;
    }
  }
  return out;
}

// Groups in the order the catalogue first mentions them, so the data decides the rail.
export function groupsFor(provider: Provider): { group: string; entries: CatalogueEntry[] }[] {
  const groups: { group: string; entries: CatalogueEntry[] }[] = [];
  for (const entry of catalogueFor(provider)) {
    const existing = groups.find((g) => g.group === entry.group);
    if (existing === undefined) {
      groups.push({ group: entry.group, entries: [entry] });
    } else {
      existing.entries.push(entry);
    }
  }
  return groups;
}

export function matches(entry: CatalogueEntry, query: string): boolean {
  const needle = query.trim().toLowerCase();
  if (needle === '') {
    return true;
  }
  return [entry.label, entry.description, entry.resource, entry.type, ...entry.aliases].some(
    (field) => field.toLowerCase().includes(needle),
  );
}
