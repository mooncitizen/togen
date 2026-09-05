import schemes from '../../../schema/styles.json';

import { bundled } from './icons.ts';
import type { Config, NodeStyle, NodeType, Provider, Shape } from './types.ts';

export type Source = 'node' | 'kind' | 'scheme';

export type Resolved = {
  color: string;
  icon: string;
  shape: Shape;
  source: { color: Source; icon: Source; shape: Source };
};

// AWS draws a white glyph for a square of the category colour; Google and
// Microsoft draw coloured glyphs for a light ground, so their square is a tint.
export type Ground = 'solid' | 'tint';

export type Artwork =
  | { kind: 'inline'; svg: string; ground: Ground }
  | { kind: 'file'; url: string; ground: Ground }
  | { kind: 'none'; ground: Ground };

// Node, then kind, then the provider's scheme, property by property (ADR 0007).
export function resolveStyle(
  node: { name: string; type: NodeType },
  config: Config | null,
  provider: Provider,
): Resolved {
  return resolve(node.type, config?.style?.nodes?.[node.name], config, provider);
}

export function resolveKind(type: NodeType, config: Config | null, provider: Provider): Resolved {
  return resolve(type, undefined, config, provider);
}

export function accent(provider: Provider): string {
  return schemes[provider].accent;
}

export function resolveIcon(icon: string): Artwork {
  const svg = bundled[icon];
  if (svg !== undefined) {
    return { kind: 'inline', svg, ground: icon.startsWith('aws/') ? 'solid' : 'tint' };
  }
  if (isRelative(icon)) {
    return { kind: 'file', url: `/api/icon?path=${encodeURIComponent(icon)}`, ground: 'solid' };
  }
  return { kind: 'none', ground: 'solid' };
}

function resolve(
  type: NodeType,
  own: NodeStyle | undefined,
  config: Config | null,
  provider: Provider,
): Resolved {
  const kind = config?.style?.kinds?.[type];
  const scheme = schemes[provider].kinds[type];
  const color = pick(own?.color, kind?.color, scheme.color);
  const icon = pick(own?.icon, kind?.icon, scheme.icon);
  const shape = pick(own?.shape, kind?.shape, scheme.shape as Shape);
  return {
    color: color.value,
    icon: icon.value,
    shape: shape.value,
    source: { color: color.source, icon: icon.source, shape: shape.source },
  };
}

function pick<T>(own: T | undefined, kind: T | undefined, scheme: T): { value: T; source: Source } {
  if (own !== undefined) {
    return { value: own, source: 'node' };
  }
  if (kind !== undefined) {
    return { value: kind, source: 'kind' };
  }
  return { value: scheme, source: 'scheme' };
}

function isRelative(path: string): boolean {
  return path.startsWith('./') || path.startsWith('../');
}
