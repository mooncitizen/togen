import { afterEach, beforeEach, expect, test, vi } from 'vitest';

import { reset, serve, show } from '../../harness.ts';
import type { Layout, Project, Provider } from '../types.ts';

const shop: Project = {
  version: 1,
  name: 'shop',
  provider: 'aws',
  region: 'eu-west-2',
  environment: 'dev',
  nodes: [{ id: 'gateway-1', type: 'gateway', name: 'api' }],
  edges: [],
};

const placed: Layout = {
  version: 1,
  nodes: { 'gateway-1': { x: 20, y: 20 } },
  viewport: { x: 0, y: 0, zoom: 1 },
};

const regions: Record<Provider, string> = { aws: 'eu-west-2', gcp: 'europe-west2', azure: 'uksouth' };

beforeEach(() => {
  reset();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test.each([
  ['aws', 'aws/ecs', 'rgb(237, 113, 0)'],
  ['gcp', 'gcp/cloud-run', 'rgb(66, 133, 244)'],
  ['azure', 'azure/container-apps', 'rgb(0, 120, 212)'],
] as [Provider, string, string][])(
  'the palette shows the %s icons and the bar dot takes its accent',
  async (provider, service, accent) => {
    serve({ ...shop, provider, region: regions[provider] }, placed);
    const screen = await show();

    const tiles = screen.container.querySelectorAll('[data-node-type]');
    expect(tiles).toHaveLength(7);
    for (const tile of tiles) {
      const icon = tile.querySelector<HTMLElement>('[data-icon]');
      expect(icon?.dataset.icon).toMatch(new RegExp(`^${provider}/`));
      expect(icon?.querySelector('svg')).not.toBeNull();
      expect(icon?.getBoundingClientRect().width).toBe(32);
    }
    expect(
      screen.container.querySelector(`[data-node-type="service"] [data-icon="${service}"]`),
    ).not.toBeNull();

    const dot = screen.container.querySelector('header .h-2.w-2.rounded-full')!;
    expect(getComputedStyle(dot).backgroundColor).toBe(accent);
  },
);
