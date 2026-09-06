import { afterEach, beforeEach, expect, test, vi } from 'vitest';

import { defaults, overviewLayout, reset, serve, show, socket } from '../../harness.ts';
import type { Config, Project } from '../types.ts';

const shop: Project = {
  version: 1,
  name: 'shop',
  provider: 'aws',
  region: 'eu-west-2',
  environment: 'dev',
  nodes: [
    { id: 'gateway-1', type: 'gateway', name: 'api' },
    { id: 'function-1', type: 'function', name: 'orders' },
    { id: 'database-1', type: 'database', name: 'orders-db' },
    { id: 'database-2', type: 'database', name: 'reports-db' },
  ],
  edges: [{ id: 'edge-1', from: 'gateway-1', to: 'function-1', relation: 'routes' }],
};

const placed = overviewLayout({
    'gateway-1': { x: 20, y: 20 },
    'function-1': { x: 260, y: 20 },
    'database-1': { x: 500, y: 20 },
    'database-2': { x: 500, y: 140 },
  });

const styled: Config = {
  ...defaults,
  style: {
    theme: 'dark',
    kinds: { database: { color: '#112233' } },
    nodes: { 'orders-db': { color: '#DD344C', shape: 'hexagon', icon: './icons/orders.svg' } },
  },
};

type Screen = Awaited<ReturnType<typeof show>>;

async function card(screen: Screen, id: string): Promise<HTMLElement> {
  return vi.waitFor(() => {
    const found = screen.container.querySelector<HTMLElement>(
      `.svelte-flow__node[data-id="${id}"] .card`,
    );
    expect(found).not.toBeNull();
    return found!;
  });
}

function square(element: Element): HTMLElement {
  const found = element.querySelector<HTMLElement>('[data-icon]');
  expect(found).not.toBeNull();
  return found!;
}

// Chromium reports a colour-mixed background as color(srgb r g b / a) and a
// plain one as rgb(r, g, b); both come back as 0 to 255 channels and an alpha.
function rgba(value: string): number[] {
  const srgb = /^color\(srgb ([\d.]+) ([\d.]+) ([\d.]+)(?: \/ ([\d.]+))?\)$/.exec(value);
  if (srgb !== null) {
    return [
      Math.round(Number(srgb[1]) * 255),
      Math.round(Number(srgb[2]) * 255),
      Math.round(Number(srgb[3]) * 255),
      srgb[4] === undefined ? 1 : Math.round(Number(srgb[4]) * 1000) / 1000,
    ];
  }
  const rgb = /^rgba?\(([\d.]+), ([\d.]+), ([\d.]+)(?:, ([\d.]+))?\)$/.exec(value);
  expect(rgb, value).not.toBeNull();
  return [
    Number(rgb![1]),
    Number(rgb![2]),
    Number(rgb![3]),
    rgb![4] === undefined ? 1 : Math.round(Number(rgb![4]) * 1000) / 1000,
  ];
}

function background(element: Element): number[] {
  return rgba(getComputedStyle(element).backgroundColor);
}

beforeEach(() => {
  reset();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test('a card is the design: tinted in the scheme colour, the provider icon in a square of it', async () => {
  serve(shop, placed);
  const screen = await show();

  const api = await card(screen, 'gateway-1');
  expect(api.getBoundingClientRect().width).toBe(160);
  expect(api.getBoundingClientRect().height).toBe(56);
  expect(api.textContent).toContain('gateway');
  expect(api.textContent).toContain('api');

  const tint = background(api);
  expect(tint.slice(0, 3)).toEqual([140, 79, 255]);
  expect(tint[3]).toBeCloseTo(0.133, 2);

  const icon = square(api);
  expect(icon.dataset.icon).toBe('aws/api-gateway');
  expect(icon.dataset.shape).toBe('card');
  expect(icon.querySelector('svg')).not.toBeNull();
  expect(background(icon)).toEqual([140, 79, 255, 1]);
  expect(icon.getBoundingClientRect().width).toBe(28);

  const db = await card(screen, 'database-1');
  expect(square(db).dataset.icon).toBe('aws/rds');
  expect(square(db).dataset.shape).toBe('cylinder');
  expect(background(square(db))).toEqual([201, 37, 209, 1]);
});

test('style.nodes restyles one node and style.kinds the rest of its type', async () => {
  serve(shop, placed, styled);
  const screen = await show();

  const orders = square(await card(screen, 'database-1'));
  expect(background(orders)).toEqual([221, 52, 76, 1]);
  expect(orders.dataset.shape).toBe('hexagon');
  expect(orders.querySelector('img')?.getAttribute('src')).toBe(
    '/api/icon?path=.%2Ficons%2Forders.svg',
  );

  const reports = square(await card(screen, 'database-2'));
  expect(background(reports)).toEqual([17, 34, 51, 1]);
  expect(reports.dataset.shape).toBe('cylinder');
  expect(reports.dataset.icon).toBe('aws/rds');

  expect(background(square(await card(screen, 'gateway-1')))).toEqual([140, 79, 255, 1]);
});

test('the badge counts the problems on the node and the selected card gets the ring', async () => {
  serve(
    { ...shop, nodes: [...shop.nodes, { id: 'gateway-2', type: 'gateway', name: 'admin' }] },
    overviewLayout({ ...placed.views.overview.nodes, 'gateway-2': { x: 20, y: 200 } }),
  );
  const screen = await show();

  const admin = await card(screen, 'gateway-2');
  await vi.waitFor(() =>
    expect(admin.querySelector('[aria-label="1 problem"]')?.textContent).toBe('1'),
  );
  const api = await card(screen, 'gateway-1');
  expect(api.querySelector('.bg-err')).toBeNull();
  expect(getComputedStyle(api).boxShadow).toBe('none');

  api.dispatchEvent(new MouseEvent('click', { bubbles: true }));

  await vi.waitFor(() => {
    const ring = getComputedStyle(api);
    expect(ring.boxShadow).toContain('rgb(230, 233, 238)');
    expect(ring.borderColor).toBe('rgb(230, 233, 238)');
  });
  expect(getComputedStyle(admin).boxShadow).toBe('none');
});

test('cards and tiles take the new colours when togen.yml changes', async () => {
  serve(shop, placed);
  const screen = await show();
  const api = square(await card(screen, 'gateway-1'));
  const tile = square(screen.container.querySelector('[data-node-type="gateway"]')!);
  expect(background(api)).toEqual([140, 79, 255, 1]);
  expect(background(tile)).toEqual([140, 79, 255, 1]);

  serve(shop, placed, { ...defaults, style: { theme: 'dark', kinds: { gateway: { color: '#112233' } } } });
  socket()?.onmessage?.({ data: JSON.stringify({ event: 'config-changed' }) });

  await vi.waitFor(async () =>
    expect(background(square(await card(screen, 'gateway-1')))).toEqual([17, 34, 51, 1]),
  );
  expect(background(square(screen.container.querySelector('[data-node-type="gateway"]')!))).toEqual([
    17, 34, 51, 1,
  ]);
});
