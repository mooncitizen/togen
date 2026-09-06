import { expect, test } from 'vitest';

import schemes from '../../../schema/styles.json';

import { bundled } from './icons.ts';
import { accent, resolveIcon, resolveKind, resolveStyle } from './style.ts';
import type { Config, NodeType, Provider } from './types.ts';

const base: Config = { version: 1, targets: ['hcl'], outDir: 'infra' };
const orders = { name: 'orders-db', type: 'database' as const };

test('with nothing configured every property comes from the provider scheme', () => {
  expect(resolveStyle(orders, null, 'aws')).toEqual({
    color: '#C925D1',
    icon: 'aws/rds',
    shape: 'cylinder',
    source: { color: 'scheme', icon: 'scheme', shape: 'scheme' },
  });
  expect(resolveStyle(orders, base, 'gcp').icon).toBe('gcp/cloud-sql');
  expect(resolveKind('queue', base, 'azure').color).toBe('#FFB900');
});

test('a kind overrides the scheme and a node overrides the kind, property by property', () => {
  const config: Config = {
    ...base,
    style: {
      kinds: { database: { color: '#112233', shape: 'hexagon' } },
      nodes: { 'orders-db': { color: '#DD344C', icon: './icons/orders.svg' } },
    },
  };

  expect(resolveStyle(orders, config, 'aws')).toEqual({
    color: '#DD344C',
    icon: './icons/orders.svg',
    shape: 'hexagon',
    source: { color: 'node', icon: 'node', shape: 'kind' },
  });
  expect(resolveStyle({ name: 'reports-db', type: 'database' }, config, 'aws')).toEqual({
    color: '#112233',
    icon: 'aws/rds',
    shape: 'hexagon',
    source: { color: 'kind', icon: 'scheme', shape: 'kind' },
  });
  expect(resolveKind('database', config, 'aws')).toEqual({
    color: '#112233',
    icon: 'aws/rds',
    shape: 'hexagon',
    source: { color: 'kind', icon: 'scheme', shape: 'kind' },
  });
});

test('a bundled id is inlined and a relative path is fetched from the studio', () => {
  expect(resolveIcon('aws/lambda')).toEqual({
    kind: 'inline',
    svg: bundled['aws/lambda'],
    ground: 'solid',
  });
  expect(resolveIcon('gcp/cloud-run')).toMatchObject({ kind: 'inline', ground: 'tint' });
  expect(resolveIcon('azure/functions')).toMatchObject({ kind: 'inline', ground: 'tint' });
  expect(resolveIcon('./icons/orders.svg')).toEqual({
    kind: 'file',
    url: '/api/icon?path=.%2Ficons%2Forders.svg',
    ground: 'solid',
  });
  expect(resolveIcon('../shared/db.png')).toMatchObject({
    kind: 'file',
    url: '/api/icon?path=..%2Fshared%2Fdb.png',
  });
  expect(resolveIcon('aws/nothing')).toEqual({ kind: 'none', ground: 'solid' });
});

test('every icon the schemes name is bundled as svg with its own ids', () => {
  const ids = new Set<string>();
  for (const provider of Object.keys(schemes) as Provider[]) {
    for (const type of Object.keys(schemes[provider].kinds) as NodeType[]) {
      const icon = schemes[provider].kinds[type].icon;
      const svg = bundled[icon];
      expect(svg, icon).toMatch(/^<svg /);
      expect(svg, icon).not.toMatch(/<(style|title)|class=/);
      for (const id of svg.matchAll(/\sid="([^"]+)"/g)) {
        expect(ids.has(id[1]), `${icon} reuses ${id[1]}`).toBe(false);
        ids.add(id[1]);
      }
      for (const ref of svg.matchAll(/url\(#([^)]+)\)/g)) {
        expect(svg, `${icon} refers to ${ref[1]}`).toContain(`id="${ref[1]}"`);
      }
    }
  }
});

test('the provider accent comes from its scheme', () => {
  expect(accent('aws')).toBe('#ED7100');
  expect(accent('gcp')).toBe('#4285F4');
  expect(accent('azure')).toBe('#0078D4');
});
