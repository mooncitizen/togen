import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { page } from 'vitest/browser';

import { calls, puts, reset, serve, settle, show } from '../../harness.ts';
import { boundariesFor } from '../boundary.ts';
import type { Layout, Position, Project, Views } from '../types.ts';

const shop: Project = {
  version: 1,
  name: 'shop',
  provider: 'aws',
  region: 'eu-west-2',
  environment: 'dev',
  nodes: [
    { id: 'gateway-1', type: 'gateway', name: 'api' },
    { id: 'function-1', type: 'function', name: 'orders' },
    { id: 'function-2', type: 'function', name: 'cron' },
    { id: 'service-1', type: 'service', name: 'web', properties: { image: 'nginx:1.27' } },
    { id: 'database-1', type: 'database', name: 'orders-db' },
    { id: 'queue-1', type: 'queue', name: 'jobs' },
    { id: 'bucket-1', type: 'bucket', name: 'uploads' },
    { id: 'cache-1', type: 'cache', name: 'sessions' },
  ],
  edges: [
    { id: 'edge-1', from: 'gateway-1', to: 'function-1', relation: 'routes' },
    { id: 'edge-2', from: 'function-1', to: 'database-1', relation: 'reads' },
    { id: 'edge-3', from: 'gateway-1', to: 'function-2', relation: 'routes' },
    { id: 'edge-4', from: 'function-2', to: 'queue-1', relation: 'publishes' },
  ],
};

const overview: Record<string, Position> = {
  'gateway-1': { x: 20, y: 200 },
  'function-1': { x: 260, y: 80 },
  'function-2': { x: 20, y: 360 },
  'service-1': { x: 260, y: 200 },
  'database-1': { x: 500, y: 80 },
  'queue-1': { x: 740, y: 80 },
  'bucket-1': { x: 740, y: 200 },
  'cache-1': { x: 500, y: 200 },
};

const everything = new Set(shop.nodes.map((node) => node.id));
const inNetwork = ['function-1', 'service-1', 'database-1', 'cache-1'];
const outside = ['gateway-1', 'function-2', 'queue-1', 'bucket-1'];
const vpcRect = { x: 236, y: 44, width: 448, height: 236 };

const views: Views = {
  version: 1,
  views: [
    { id: 'overview', name: 'Overview', nodes: '*' },
    { id: 'orders', name: 'Orders path', nodes: ['gateway-1', 'function-1', 'database-1'] },
    { id: 'edge', name: 'Edge', nodes: ['gateway-1', 'function-2', 'queue-1'] },
  ],
};

const still = { x: 0, y: 0, zoom: 1 };

const placed: Layout = {
  version: 2,
  views: {
    overview: { nodes: overview, viewport: still },
    orders: {
      nodes: {
        'gateway-1': { x: 20, y: 80 },
        'function-1': { x: 260, y: 80 },
        'database-1': { x: 500, y: 80 },
      },
      viewport: still,
    },
    edge: {
      nodes: {
        'gateway-1': { x: 20, y: 80 },
        'function-2': { x: 260, y: 80 },
        'queue-1': { x: 500, y: 80 },
      },
      viewport: still,
    },
  },
};

type Screen = Awaited<ReturnType<typeof show>>;

async function boundary(screen: Screen, id = 'network'): Promise<HTMLElement> {
  return vi.waitFor(() => {
    const found = screen.container.querySelector<HTMLElement>(`[data-boundary="${id}"]`);
    expect(found).not.toBeNull();
    return found!;
  });
}

function drawn(screen: Screen): string[] {
  return [...screen.container.querySelectorAll('[data-boundary]')].map(
    (element) => element.getAttribute('data-boundary') ?? '',
  );
}

function parts(element: Element): string[] {
  return [...element.querySelectorAll('.label > span')].map(
    (span) => span.textContent?.trim() ?? '',
  );
}

async function card(screen: Screen, id: string): Promise<HTMLElement> {
  return vi.waitFor(() => {
    const found = screen.container.querySelector<HTMLElement>(
      `.svelte-flow__node[data-id="${id}"]`,
    );
    expect(found).not.toBeNull();
    return found!;
  });
}

function within(outer: DOMRect, inner: DOMRect): boolean {
  return (
    inner.left >= outer.left &&
    inner.top >= outer.top &&
    inner.right <= outer.right &&
    inner.bottom <= outer.bottom
  );
}

function centre(element: Element): Position {
  const box = element.getBoundingClientRect();
  return { x: box.left + box.width / 2, y: box.top + box.height / 2 };
}

function mouse(type: string, at: Position): MouseEvent {
  return new MouseEvent(type, {
    bubbles: true,
    cancelable: true,
    clientX: at.x,
    clientY: at.y,
    view: window,
  });
}

// Svelte Flow drags with d3, which takes the press on the card and the rest
// from the window the press names, and measures the drag from the first move
// past its threshold rather than from the press.
async function drag(screen: Screen, id: string, by: Position) {
  const node = await card(screen, id);
  const pressed = centre(node);
  const from = { x: pressed.x + 2, y: pressed.y };
  const to = { x: from.x + by.x, y: from.y + by.y };
  node.dispatchEvent(mouse('mousedown', pressed));
  window.dispatchEvent(mouse('mousemove', from));
  window.dispatchEvent(mouse('mousemove', to));
  window.dispatchEvent(mouse('mouseup', to));
  await settle(50);
}

function inspector(screen: Screen): Element | null {
  return screen.container.querySelector('aside[aria-label="Inspector"]');
}

// Hit testing needs the whole 900px screen on view, not the 414px default.
beforeEach(async () => {
  reset();
  await page.viewport(1200, 700);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test('aws boxes the services, databases and caches, and the functions that reach one', () => {
  expect(boundariesFor(shop, everything, overview, 'aws')).toEqual([
    {
      id: 'network',
      kind: 'VPC',
      label: 'shop-dev',
      color: '#8C4FFF',
      nodeIds: inNetwork,
      rect: vpcRect,
    },
  ]);
});

test('gcp draws its VPC network by the same rule', () => {
  expect(boundariesFor(shop, everything, overview, 'gcp')).toEqual([
    {
      id: 'network',
      kind: 'VPC network',
      label: 'shop-dev',
      color: '#4285F4',
      nodeIds: inNetwork,
      rect: vpcRect,
    },
  ]);
});

test('azure puts the resource group around every node, outside the VNet', () => {
  expect(boundariesFor(shop, everything, overview, 'azure')).toEqual([
    {
      id: 'resource-group',
      kind: 'Resource group',
      label: 'shop-dev-rg',
      color: '#0078D4',
      nodeIds: [...everything],
      rect: { x: -4, y: 8, width: 928, height: 432 },
    },
    {
      id: 'network',
      kind: 'VNet',
      label: 'shop-dev',
      color: '#0078D4',
      nodeIds: inNetwork,
      rect: vpcRect,
    },
  ]);
});

test('a function that calls a service is in the network too', () => {
  const calling: Project = {
    ...shop,
    edges: [...shop.edges, { id: 'edge-5', from: 'function-2', to: 'service-1', relation: 'calls' }],
  };

  expect(boundariesFor(calling, everything, overview, 'aws')[0].nodeIds).toEqual([
    'function-1',
    'function-2',
    'service-1',
    'database-1',
    'cache-1',
  ]);
});

test('nothing is drawn when no visible node needs the network, and a node without a position waits', () => {
  const edge = new Set(['gateway-1', 'function-2', 'queue-1']);
  expect(boundariesFor(shop, edge, overview, 'aws')).toEqual([]);
  expect(boundariesFor(shop, edge, overview, 'azure').map((found) => found.id)).toEqual([
    'resource-group',
  ]);

  const some = Object.fromEntries(
    Object.entries(overview).filter(([id]) => id !== 'database-1' && id !== 'cache-1'),
  );
  const [vpc] = boundariesFor(shop, everything, some, 'aws');
  expect(vpc.nodeIds).toEqual(['function-1', 'service-1']);
  expect(vpc.rect).toEqual({ x: 236, y: 44, width: 208, height: 236 });
});

test('the canvas draws the VPC around the nodes that need it, labelled, in mono, under the cards', async () => {
  serve(shop, placed, undefined, views);
  const screen = await show();

  const vpc = await boundary(screen);
  expect(drawn(screen)).toEqual(['network']);
  expect(parts(vpc)).toEqual(['VPC', 'shop-dev', 'implicit']);
  expect(vpc.dataset.nodes).toBe(inNetwork.join(' '));
  const kind = vpc.querySelector('.kind')!;
  expect(getComputedStyle(kind).color).toBe('rgb(140, 79, 255)');
  expect(getComputedStyle(kind).fontFamily).toContain('IBM Plex Mono');

  const box = vpc.getBoundingClientRect();
  for (const id of inNetwork) {
    expect(within(box, (await card(screen, id)).getBoundingClientRect()), id).toBe(true);
  }
  for (const id of outside) {
    expect(within(box, (await card(screen, id)).getBoundingClientRect()), id).toBe(false);
  }
  const web = await card(screen, 'service-1');
  const at = centre(web);
  expect(document.elementFromPoint(at.x, at.y)?.closest('.svelte-flow__node')).toBe(web);
  expect(screen.container.querySelectorAll('.svelte-flow__node')).toHaveLength(8);
});

test('the boundary follows the view: fewer members shrink it and none removes it', async () => {
  serve(shop, placed, undefined, views);
  const screen = await show();
  const whole = (await boundary(screen)).getBoundingClientRect();

  await screen.getByRole('button', { name: 'Orders path 3', exact: true }).click();

  await vi.waitFor(() =>
    expect(screen.container.querySelector('[data-boundary="network"]')?.getAttribute('data-nodes')).toBe(
      'function-1 database-1',
    ),
  );
  const part = (await boundary(screen)).getBoundingClientRect();
  expect(part.height).toBeLessThan(whole.height);
  expect(within(part, (await card(screen, 'gateway-1')).getBoundingClientRect())).toBe(false);

  await screen.getByRole('button', { name: 'Edge 3', exact: true }).click();

  await vi.waitFor(() => expect(drawn(screen)).toEqual([]));
  await vi.waitFor(() => expect(card(screen, 'queue-1')).resolves.toBeDefined());
});

test('the boundary moves with a dragged member and none of it is saved', async () => {
  serve(shop, placed, undefined, views);
  const screen = await show();
  const before = (await boundary(screen)).getBoundingClientRect();

  await drag(screen, 'service-1', { x: -120, y: 80 });

  await vi.waitFor(() => {
    const web = screen.container
      .querySelector('.svelte-flow__node[data-id="service-1"]')!
      .getBoundingClientRect();
    const after = screen.container
      .querySelector('[data-boundary="network"]')!
      .getBoundingClientRect();
    expect(after.left).toBeCloseTo(web.left - 24, 0);
    expect(after.bottom).toBeCloseTo(web.bottom + 24, 0);
    expect(after.left).toBeCloseTo(before.left - 120, 0);
    expect(after.bottom).toBeCloseTo(before.bottom + 80, 0);
    expect(after.right).toBeCloseTo(before.right, 0);
    expect(after.top).toBeCloseTo(before.top, 0);
  });

  await vi.waitFor(() => expect(puts('/api/layout')).toHaveLength(1));
  const layout = puts('/api/layout')[0].body as Layout;
  expect(layout.views.overview.nodes['service-1']).toEqual({ x: 140, y: 280 });
  expect(Object.keys(layout.views.overview.nodes).sort()).toEqual([...everything].sort());
  for (const call of calls().filter((made) => made.method === 'PUT')) {
    expect(call.rawBody).not.toMatch(/boundary|network|vpc|implicit|resource group/i);
  }
});

test('the boundary takes no clicks: one on it lands on the pane and selects nothing', async () => {
  serve(shop, placed, undefined, views);
  const screen = await show();
  const vpc = await boundary(screen);
  const api = await card(screen, 'gateway-1');
  api.querySelector('.card')!.dispatchEvent(new MouseEvent('click', { bubbles: true }));
  await vi.waitFor(() => expect(inspector(screen)).not.toBeNull());

  // On the fill, clear of the label, the cards and the edges between them.
  const box = vpc.getBoundingClientRect();
  const at = { x: box.left + 8, y: box.top + 30 };
  const hit = document.elementFromPoint(at.x, at.y);
  expect(hit?.closest('[data-boundary]')).toBeNull();
  expect(hit?.classList.contains('svelte-flow__pane')).toBe(true);

  hit!.dispatchEvent(mouse('click', at));

  await vi.waitFor(() => expect(inspector(screen)).toBeNull());
  expect(screen.container.querySelector('.svelte-flow__node.selected')).toBeNull();
  expect(vpc.closest('.svelte-flow__node')).toBeNull();
});

test('azure draws the resource group around everything with the VNet inside it', async () => {
  serve({ ...shop, provider: 'azure', region: 'uksouth' }, placed, undefined, views);
  const screen = await show();

  const group = await boundary(screen, 'resource-group');
  const vnet = await boundary(screen, 'network');
  expect(drawn(screen)).toEqual(['resource-group', 'network']);
  expect(parts(group)).toEqual(['Resource group', 'shop-dev-rg', 'implicit']);
  expect(parts(vnet)).toEqual(['VNet', 'shop-dev', 'implicit']);
  expect(getComputedStyle(group.querySelector('.kind')!).color).toBe('rgb(0, 120, 212)');
  expect(group.dataset.nodes?.split(' ')).toEqual([...everything]);

  const around = group.getBoundingClientRect();
  expect(within(around, vnet.getBoundingClientRect())).toBe(true);
  for (const id of outside) {
    expect(within(around, (await card(screen, id)).getBoundingClientRect()), id).toBe(true);
  }
});
