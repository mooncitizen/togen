import schema from '../../../schema/project.schema.json';
import regionTable from '../../../schema/regions.json';
import styleTable from '../../../schema/styles.json';

import { catalogueFor } from './catalogue.ts';
import type { NodeType, Provider } from './types.ts';

export type Region = { id: string; name: string };

export type Swatch = { type: NodeType; color: string; resource: string };

export type Palette = { provider: Provider; label: string; accent: string; swatches: Swatch[] };

type Scheme = { accent: string; kinds: Record<string, { color: string; resource: string }> };

const labels: Record<Provider, string> = { aws: 'AWS', gcp: 'GCP', azure: 'Azure' };
const schemes = styleTable as Record<Provider, Scheme>;
const regions = regionTable as Record<Provider, Region[]>;

// The schema's order, which is also the CLI's: aws, gcp, azure.
export const providers = schema.properties.provider.enum as Provider[];

export const palettes: Palette[] = providers.map((provider) => ({
  provider,
  label: labels[provider],
  accent: schemes[provider].accent,
  swatches: catalogueFor(provider).map((entry) => ({
    type: entry.type,
    color: entry.style.color,
    resource: entry.resource,
  })),
}));

export function regionsFor(provider: Provider): Region[] {
  return regions[provider] ?? [];
}

export function defaultRegion(provider: Provider): string {
  return regionsFor(provider)[0]?.id ?? '';
}

// The name togen init would pick: the directory's, made to fit the schema.
export function suggestedName(directory: string): string {
  const name = directory
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^[^a-z]+/, '')
    .slice(0, 32)
    .replace(/-+$/, '');
  return name === '' ? 'project' : name;
}
