import { afterEach, beforeEach, expect, test, vi } from 'vitest';

import {
  calls,
  gets,
  invalid,
  overviewLayout,
  puts,
  refuse,
  reset,
  serve,
  settle,
  show,
  socket,
} from './harness.ts';
import { Store } from './lib/store.svelte.ts';
import type { Layout, Project } from './lib/types.ts';

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
  ],
  edges: [
    { id: 'edge-1', from: 'gateway-1', to: 'function-1', relation: 'routes' },
    { id: 'edge-2', from: 'function-1', to: 'database-1', relation: 'reads' },
  ],
};

const placed = overviewLayout({
  'gateway-1': { x: 0, y: 0 },
  'function-1': { x: 300, y: 0 },
  'database-1': { x: 600, y: 0 },
});

const unplaced = overviewLayout({});

function dropOn(target: Element, type: string, at: { x: number; y: number }) {
  const transfer = new DataTransfer();
  transfer.setData('application/togen-node', type);
  target.dispatchEvent(
    new DragEvent('dragover', { bubbles: true, cancelable: true, dataTransfer: transfer }),
  );
  target.dispatchEvent(
    new DragEvent('drop', {
      bubbles: true,
      cancelable: true,
      dataTransfer: transfer,
      clientX: at.x,
      clientY: at.y,
    }),
  );
}

beforeEach(() => {
  reset();
  serve(shop, placed);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test('the bar names the project and its provider, and the canvas is on the page', async () => {
  const screen = await show();

  await expect.element(screen.getByText('Togen', { exact: true })).toBeInTheDocument();
  await expect.element(screen.getByText('shop')).toBeInTheDocument();
  await expect.element(screen.getByText('aws')).toBeInTheDocument();
});

test('the palette lists what the provider offers, grouped in catalogue order', async () => {
  const screen = await show();

  const entries = screen.container.querySelectorAll('[data-node-type]');
  expect([...entries].map((entry) => entry.getAttribute('data-node-type'))).toEqual([
    'service',
    'function',
    'database',
    'cache',
    'gateway',
    'queue',
    'bucket',
  ]);
  expect(
    [...screen.container.querySelectorAll('[data-palette-group]')].map((g) =>
      g.getAttribute('data-palette-group'),
    ),
  ).toEqual(['Compute', 'Databases', 'Networking', 'Messaging', 'Storage']);
  for (const label of ['Service', 'Function', 'Database', 'Gateway', 'Queue', 'Bucket', 'Cache']) {
    await expect.element(screen.getByText(label, { exact: true })).toBeInTheDocument();
  }
});

test('dropping a type on the canvas adds a named node and saves the project and the layout', async () => {
  const screen = await show();
  const pane = screen.container.querySelector('.svelte-flow')!;

  dropOn(pane, 'database', { x: 400, y: 260 });

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  const project = puts('/api/project')[0].body as Project;
  expect(project.nodes.at(-1)).toEqual({
    id: 'database-2',
    type: 'database',
    name: 'database-2',
    properties: {},
  });

  await vi.waitFor(() => expect(puts('/api/layout')).toHaveLength(1));
  const layout = puts('/api/layout')[0].body as Layout;
  expect(layout.views.overview.nodes['database-2']).toBeDefined();
  expect(Number.isFinite(layout.views.overview.nodes['database-2'].x)).toBe(true);

  await expect.element(screen.getByText('database-2')).toBeInTheDocument();
});

test('a project PUT body matches the CLI: two-space indent and a trailing newline', async () => {
  const screen = await show();

  dropOn(screen.container.querySelector('.svelte-flow')!, 'database', { x: 400, y: 260 });

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  const raw = puts('/api/project')[0].rawBody ?? '';
  expect(raw.startsWith('{\n  "version"')).toBe(true);
  expect(raw.endsWith('\n')).toBe(true);
  expect(raw.endsWith('\n\n')).toBe(false);
});

test('a second drop of the same type takes the next free number', async () => {
  const screen = await show();
  const pane = screen.container.querySelector('.svelte-flow')!;

  dropOn(pane, 'database', { x: 300, y: 200 });
  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  dropOn(pane, 'database', { x: 500, y: 300 });
  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(2));

  const project = puts('/api/project')[1].body as Project;
  expect(project.nodes.map((node) => node.id)).toContain('database-3');
});

test('a dropped service carries the placeholder image so it validates', async () => {
  const screen = await show();

  dropOn(screen.container.querySelector('.svelte-flow')!, 'service', { x: 300, y: 200 });

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  const project = puts('/api/project')[0].body as Project;
  expect(project.nodes.at(-1)?.properties).toEqual({ image: 'nginx:1.27' });
});

test('a project with no layout is laid out left to right and saved', async () => {
  serve(shop, unplaced);
  const store = new Store();
  await store.load();

  await vi.waitFor(() => expect(puts('/api/layout')).toHaveLength(1));
  const layout = puts('/api/layout')[0].body as Layout;
  expect(Object.keys(layout.views.overview.nodes).sort()).toEqual(['database-1', 'function-1', 'gateway-1']);
  expect(layout.views.overview.nodes['gateway-1'].x).toBeLessThan(layout.views.overview.nodes['function-1'].x);
  expect(layout.views.overview.nodes['function-1'].x).toBeLessThan(layout.views.overview.nodes['database-1'].x);
});

test('an existing position survives the auto layout', async () => {
  serve(shop, overviewLayout({ 'gateway-1': { x: -50, y: -50 } }));
  const store = new Store();
  await store.load();

  await vi.waitFor(() => expect(puts('/api/layout')).toHaveLength(1));
  const layout = puts('/api/layout')[0].body as Layout;
  expect(layout.views.overview.nodes['gateway-1']).toEqual({ x: -50, y: -50 });
  expect(layout.views.overview.nodes['database-1']).toBeDefined();
});

test('a broken layout fetch surfaces the error and leaves positions alone, until a reload succeeds', async () => {
  const store = new Store();
  await store.load();
  const loaded = store.positions;
  expect(loaded['gateway-1']).toEqual(placed.views.overview.nodes['gateway-1']);

  let failNext = true;
  refuse((call) => {
    if (failNext && call.method === 'GET' && call.path === '/api/layout') {
      failNext = false;
      return new Response(null, { status: 500 });
    }
    return undefined;
  });

  store.reload();

  await vi.waitFor(() => expect(store.error).not.toBeNull());
  expect(puts('/api/layout')).toHaveLength(0);
  expect(store.positions).toEqual(loaded);

  store.reload();

  await vi.waitFor(() => expect(store.error).toBeNull());
  expect(store.positions).toEqual(placed.views.overview.nodes);
});

test('moving a node saves the layout once after the debounce', async () => {
  const store = new Store();
  await store.load();

  store.moveNode('function-1', { x: 120.4, y: 240.6 });
  store.moveNode('function-1', { x: 130, y: 250 });
  expect(puts('/api/layout')).toHaveLength(0);

  await vi.waitFor(() => expect(puts('/api/layout')).toHaveLength(1));
  await settle();
  expect(puts('/api/layout')).toHaveLength(1);
  const layout = puts('/api/layout')[0].body as Layout;
  expect(layout.views.overview.nodes['function-1']).toEqual({ x: 130, y: 250 });
});

test('a multi-node drag saves every dragged node, once, after the debounce', async () => {
  const store = new Store();
  await store.load();

  store.moveNodes({
    'function-1': { x: 120.4, y: 240.6 },
    'database-1': { x: 620, y: 240 },
  });
  expect(puts('/api/layout')).toHaveLength(0);

  await vi.waitFor(() => expect(puts('/api/layout')).toHaveLength(1));
  await settle();
  expect(puts('/api/layout')).toHaveLength(1);
  const layout = puts('/api/layout')[0].body as Layout;
  expect(layout.views.overview.nodes['function-1']).toEqual({ x: 120, y: 241 });
  expect(layout.views.overview.nodes['database-1']).toEqual({ x: 620, y: 240 });
});

test('a viewport change is saved', async () => {
  const store = new Store();
  await store.load();

  store.setViewport({ x: -40, y: 25, zoom: 1.5 });

  await vi.waitFor(() => expect(puts('/api/layout')).toHaveLength(1));
  const layout = puts('/api/layout')[0].body as Layout;
  expect(layout.views.overview.viewport).toEqual({ x: -40, y: 25, zoom: 1.5 });
});

test('saving the layout keeps the entries of other views', async () => {
  const orders = { nodes: { 'function-1': { x: 1, y: 2 } }, viewport: { x: 0, y: 0, zoom: 2 } };
  serve(shop, { ...placed, views: { ...placed.views, orders } });
  const store = new Store();
  await store.load();

  store.moveNode('function-1', { x: 10, y: 20 });

  await vi.waitFor(() => expect(puts('/api/layout')).toHaveLength(1));
  const layout = puts('/api/layout')[0].body as Layout;
  expect(layout.version).toBe(2);
  expect(layout.views.orders).toEqual(orders);
  expect(layout.views.overview.nodes['function-1']).toEqual({ x: 10, y: 20 });
});

// Svelte Flow hides a node it has not measured yet, and a hidden node cannot be
// grabbed, so handing it a fresh object for a node that has not moved costs a drag.
test('panning leaves every node object alone, and moving one node leaves the rest', async () => {
  const store = new Store();
  await store.load();
  const before = store.flowNodes;

  store.setViewport({ x: -100, y: 50, zoom: 0.8 });
  expect(store.flowNodes[0]).toBe(before[0]);
  expect(store.flowNodes[1]).toBe(before[1]);

  store.moveNode('function-1', { x: 10, y: 20 });
  const after = store.flowNodes;
  expect(after[0]).toBe(before[0]);
  expect(after[2]).toBe(before[2]);
  expect(after[1]).not.toBe(before[1]);
  expect(after[1].position).toEqual({ x: 10, y: 20 });
});

test('a refused project reverts the dropped node and the bar shows why', async () => {
  refuse((call) =>
    call.method === 'PUT' && call.path === '/api/project'
      ? invalid([
          { path: 'nodes', nodeId: 'gateway-2', message: 'a project can have at most one gateway' },
        ])
      : undefined,
  );

  const screen = await show();

  dropOn(screen.container.querySelector('.svelte-flow')!, 'gateway', { x: 300, y: 200 });

  await expect
    .element(screen.getByText('nodes (node gateway-2): a project can have at most one gateway'))
    .toBeInTheDocument();
  await expect.element(screen.getByText('gateway-2')).not.toBeInTheDocument();
  expect(puts('/api/layout')).toHaveLength(0);
});

test('a project-changed event reloads the project', async () => {
  const screen = await show();
  await expect.element(screen.getByText('shop')).toBeInTheDocument();

  serve({ ...shop, name: 'market' }, placed);
  socket()?.onmessage?.({ data: JSON.stringify({ event: 'project-changed' }) });

  await expect.element(screen.getByText('market')).toBeInTheDocument();
});

test('a reload waits for a pending save rather than clobbering it', async () => {
  const store = new Store();
  await store.load();
  const before = calls().length;

  store.moveNode('function-1', { x: 500, y: 500 });
  store.reload();
  expect(calls().length).toBe(before);

  await vi.waitFor(() => expect(puts('/api/layout')).toHaveLength(1));
  await vi.waitFor(() => expect(gets('/api/project')).toHaveLength(2));
  await vi.waitFor(() => expect(gets('/api/layout')).toHaveLength(2));
});
