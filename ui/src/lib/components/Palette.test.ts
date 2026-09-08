import { userEvent } from 'vitest/browser';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';

import { overviewLayout, reset, serve, show } from '../../harness.ts';
import type { Project, Provider } from '../types.ts';

const shop: Project = {
  version: 1,
  name: 'shop',
  provider: 'aws',
  region: 'eu-west-2',
  environment: 'dev',
  nodes: [{ id: 'gateway-1', type: 'gateway', name: 'api' }],
  edges: [],
};

const placed = overviewLayout({ 'gateway-1': { x: 20, y: 20 } });

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

test('the search box filters the palette by label, service name and alias', async () => {
  serve(shop, placed);
  const screen = await show();
  const search = screen.container.querySelector<HTMLInputElement>('[data-palette-search]')!;

  const shown = () =>
    [...screen.container.querySelectorAll('[data-node-type]')].map((tile) =>
      tile.getAttribute('data-node-type'),
    );

  await userEvent.fill(search, 'lambda');
  expect(shown()).toEqual(['function']);

  await userEvent.fill(search, 'Object storage');
  expect(shown()).toEqual(['bucket']);

  await userEvent.fill(search, 'redis');
  expect(shown()).toEqual(['cache']);

  await userEvent.fill(search, 'mainframe');
  expect(shown()).toEqual([]);
  await expect
    .element(screen.getByText('Nothing on aws matches "mainframe".'))
    .toBeInTheDocument();

  await userEvent.fill(search, '');
  expect(shown()).toHaveLength(7);
});

test('a group heading collapses its tiles and a search opens them again', async () => {
  serve(shop, placed);
  const screen = await show();

  const compute = screen.container.querySelector<HTMLButtonElement>(
    '[data-palette-group="Compute"]',
  )!;
  expect(compute.getAttribute('aria-expanded')).toBe('true');

  await userEvent.click(compute);
  expect(screen.container.querySelector('[data-node-type="service"]')).toBeNull();

  await userEvent.fill(
    screen.container.querySelector<HTMLInputElement>('[data-palette-search]')!,
    'container',
  );
  expect(screen.container.querySelector('[data-node-type="service"]')).not.toBeNull();
});
