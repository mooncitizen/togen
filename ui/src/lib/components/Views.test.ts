import { afterEach, beforeEach, expect, test, vi } from 'vitest';

import {
  calls,
  invalid,
  puts,
  refuse,
  reset,
  serve,
  settle,
  show,
  socket,
} from '../../harness.ts';
import { Store } from '../store.svelte.ts';
import type { Layout, Project, Views } from '../types.ts';

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
    { id: 'queue-1', type: 'queue', name: 'jobs' },
    { id: 'service-1', type: 'service', name: 'web', properties: { image: 'nginx:1.27' } },
  ],
  edges: [
    { id: 'edge-1', from: 'gateway-1', to: 'function-1', relation: 'routes' },
    { id: 'edge-2', from: 'function-1', to: 'database-1', relation: 'reads' },
    { id: 'edge-3', from: 'function-1', to: 'queue-1', relation: 'publishes' },
    { id: 'edge-4', from: 'service-1', to: 'database-1', relation: 'reads' },
  ],
};

const views: Views = {
  version: 1,
  views: [
    { id: 'overview', name: 'Overview', nodes: '*' },
    { id: 'orders', name: 'Orders path', nodes: ['gateway-1', 'function-1', 'database-1'] },
  ],
};

const overview = {
  nodes: {
    'gateway-1': { x: 20, y: 20 },
    'function-1': { x: 260, y: 20 },
    'database-1': { x: 500, y: 20 },
    'queue-1': { x: 500, y: 200 },
    'service-1': { x: 20, y: 200 },
  },
  viewport: { x: 0, y: 0, zoom: 1 },
};

const placed: Layout = {
  version: 2,
  views: {
    overview,
    orders: {
      nodes: { 'gateway-1': { x: 40, y: 60 }, 'function-1': { x: 300, y: 60 } },
      viewport: { x: -10, y: 5, zoom: 1.25 },
    },
  },
};

type Screen = Awaited<ReturnType<typeof show>>;

function rows(screen: Screen): { name: string; count: string; active: boolean }[] {
  const list = screen.container.querySelector('aside[aria-label="Rail"] ul')!;
  return [...list.querySelectorAll('li')]
    .filter((row) => row.querySelector('button') !== null)
    .map((row) => ({
      name: row.querySelector('.truncate')!.textContent!.trim(),
      count: row.querySelector('.font-mono')!.textContent!.trim(),
      active: row.querySelector('[aria-current="true"]') !== null,
    }));
}

function cards(screen: Screen): string[] {
  return [...screen.container.querySelectorAll('.svelte-flow__node')]
    .map((card) => card.getAttribute('data-id') ?? '')
    .sort();
}

function edges(screen: Screen): string[] {
  return [...screen.container.querySelectorAll('.svelte-flow__edge')]
    .map((edge) => edge.getAttribute('data-id') ?? '')
    .sort();
}

function dimmed(screen: Screen, id: string): boolean {
  return screen.container.querySelector(`.svelte-flow__node[data-id="${id}"] .card.dimmed`) !== null;
}

function editor(screen: Screen): Element | null {
  return screen.container.querySelector('aside[aria-label="View editor"]');
}

function box(screen: Screen, id: string): HTMLInputElement {
  const found = editor(screen)?.querySelector<HTMLInputElement>(`li[data-node-id="${id}"] input`);
  expect(found).toBeDefined();
  return found!;
}

function savedViews(index = -1): Views {
  const call = puts('/api/views').at(index);
  expect(call).toBeDefined();
  return call!.body as Views;
}

function savedLayout(index = -1): Layout {
  const call = puts('/api/layout').at(index);
  expect(call).toBeDefined();
  return call!.body as Layout;
}

function nodesOf(file: Views, id: string): string[] | '*' {
  const view = file.views.find((candidate) => candidate.id === id);
  expect(view).toBeDefined();
  return view!.nodes;
}

async function opened(): Promise<Store> {
  const store = new Store();
  await store.load();
  return store;
}

beforeEach(() => {
  reset();
  serve(shop, placed, undefined, views);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test('the rail lists the views with their node counts and marks the open one', async () => {
  const screen = await show();

  await vi.waitFor(() =>
    expect(rows(screen)).toEqual([
      { name: 'Overview', count: '5', active: true },
      { name: 'Orders path', count: '3', active: false },
    ]),
  );
  await expect
    .element(screen.getByLabelText('Current view'))
    .toHaveTextContent('Overview · 5 of 5 nodes');
});

test('opening a view shows its nodes and the edges between them, and places the node it had no position for', async () => {
  const screen = await show();
  await vi.waitFor(() => expect(cards(screen)).toHaveLength(5));

  await screen.getByRole('button', { name: 'Orders path 3', exact: true }).click();

  await vi.waitFor(() => expect(cards(screen)).toEqual(['database-1', 'function-1', 'gateway-1']));
  await vi.waitFor(() => expect(edges(screen)).toEqual(['edge-1', 'edge-2']));
  await expect
    .element(screen.getByLabelText('Current view'))
    .toHaveTextContent('Orders path · 3 of 5 nodes');
  expect(rows(screen)[1].active).toBe(true);

  await vi.waitFor(() => expect(puts('/api/layout')).toHaveLength(1));
  const layout = savedLayout();
  expect(layout.views.orders.nodes['gateway-1']).toEqual({ x: 40, y: 60 });
  expect(layout.views.orders.nodes['database-1']).toBeDefined();
  expect(layout.views.orders.viewport).toEqual({ x: -10, y: 5, zoom: 1.25 });
  expect(layout.views.overview).toEqual(overview);
});

test('a move and a pan in one view are saved into that view alone', async () => {
  const store = await opened();
  store.openView('orders');
  await vi.waitFor(() => expect(puts('/api/layout')).toHaveLength(1));

  store.moveNode('gateway-1', { x: 300, y: 300 });
  store.setViewport({ x: 8, y: 9, zoom: 2 });

  await vi.waitFor(() => expect(puts('/api/layout')).toHaveLength(2));
  const layout = savedLayout();
  expect(layout.views.orders.nodes['gateway-1']).toEqual({ x: 300, y: 300 });
  expect(layout.views.orders.viewport).toEqual({ x: 8, y: 9, zoom: 2 });
  expect(layout.views.overview).toEqual(overview);

  store.openView('overview');
  expect(store.positions['gateway-1']).toEqual({ x: 20, y: 20 });
  expect(store.viewport).toEqual({ x: 0, y: 0, zoom: 1 });
  await settle();
  expect(puts('/api/layout')).toHaveLength(2);
});

test('the plus button asks for a name and the new view opens in the editor', async () => {
  const screen = await show();

  await screen.getByRole('button', { name: 'New view' }).click();
  await screen.getByLabelText('View name').fill('Data stores');
  await screen.getByLabelText('View name').element().dispatchEvent(
    new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }),
  );

  await vi.waitFor(() => expect(puts('/api/views')).toHaveLength(1));
  expect(savedViews().views.at(-1)).toEqual({ id: 'data-stores', name: 'Data stores', nodes: [] });
  expect(rows(screen).at(-1)).toEqual({ name: 'Data stores', count: '0', active: true });
  await vi.waitFor(() => expect(editor(screen)).not.toBeNull());
  expect(editor(screen)!.querySelector<HTMLInputElement>('#view-name')!.value).toBe('Data stores');
  await expect
    .element(screen.getByLabelText('Current view'))
    .toHaveTextContent('Data stores · 0 of 5 nodes');
  await vi.waitFor(() => expect(cards(screen)).toHaveLength(5));
  expect(dimmed(screen, 'gateway-1')).toBe(true);
});

test('a second view with the same name gets the next free id', async () => {
  const store = await opened();

  expect(store.createView('Orders path')).toBe('orders-path');
  expect(store.createView('Orders path')).toBe('orders-path-2');
  expect(store.createView('  ')).toBeNull();
  expect(store.createView('2nd Tier!')).toBe('nd-tier');
  await settle();
});

test('the editor renames the view and the rail follows', async () => {
  const screen = await show();

  await screen.getByRole('button', { name: 'Edit Orders path' }).click();
  await vi.waitFor(() => expect(editor(screen)).not.toBeNull());
  await screen.getByLabelText('Name', { exact: true }).fill('Order flow');

  await vi.waitFor(() => expect(puts('/api/views')).toHaveLength(1));
  expect(savedViews().views[1]).toEqual({
    id: 'orders',
    name: 'Order flow',
    nodes: ['gateway-1', 'function-1', 'database-1'],
  });
  expect(rows(screen)[1].name).toBe('Order flow');
});

test('ticking a node in the editor adds it to the view and undims it, and unticking dims it again', async () => {
  const screen = await show();

  await screen.getByRole('button', { name: 'Edit Orders path' }).click();
  await vi.waitFor(() => expect(editor(screen)).not.toBeNull());

  const headings = [...editor(screen)!.querySelectorAll('h3')].map((heading) =>
    heading.textContent?.trim(),
  );
  expect(headings).toEqual(['Service', 'Function', 'Database', 'Gateway', 'Queue']);
  await vi.waitFor(() => expect(cards(screen)).toHaveLength(5));
  expect(dimmed(screen, 'queue-1')).toBe(true);
  expect(dimmed(screen, 'function-1')).toBe(false);
  await vi.waitFor(() => expect(edges(screen)).toEqual(['edge-1', 'edge-2']));
  expect(box(screen, 'queue-1').checked).toBe(false);

  box(screen, 'queue-1').click();

  await vi.waitFor(() => expect(puts('/api/views')).toHaveLength(1));
  expect(nodesOf(savedViews(), 'orders')).toEqual([
    'gateway-1',
    'function-1',
    'database-1',
    'queue-1',
  ]);
  await vi.waitFor(() => expect(dimmed(screen, 'queue-1')).toBe(false));
  await vi.waitFor(() => expect(edges(screen)).toEqual(['edge-1', 'edge-2', 'edge-3']));
  await expect
    .element(screen.getByLabelText('Current view'))
    .toHaveTextContent('Orders path · 4 of 5 nodes');
  expect(box(screen, 'queue-1').checked).toBe(true);

  box(screen, 'queue-1').click();

  await vi.waitFor(() => expect(puts('/api/views')).toHaveLength(2));
  expect(nodesOf(savedViews(), 'orders')).toEqual(['gateway-1', 'function-1', 'database-1']);
  await vi.waitFor(() => expect(dimmed(screen, 'queue-1')).toBe(true));

  await screen.getByRole('button', { name: 'Done' }).click();

  await vi.waitFor(() => expect(editor(screen)).toBeNull());
  await vi.waitFor(() =>
    expect(cards(screen)).toEqual(['database-1', 'function-1', 'gateway-1']),
  );
});

test('the overview shows every node and lets none be unticked', async () => {
  const screen = await show();

  await screen.getByRole('button', { name: 'Edit Overview' }).click();
  await vi.waitFor(() => expect(editor(screen)).not.toBeNull());

  await expect.element(screen.getByText(/Show every node/)).toBeInTheDocument();
  const boxes = [...editor(screen)!.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')];
  expect(boxes).toHaveLength(5);
  expect(boxes.every((input) => input.checked && input.disabled)).toBe(true);
  expect(editor(screen)!.textContent).not.toContain('Delete view');
  expect(screen.container.querySelector('button[aria-label="Delete Overview"]')).toBeNull();

  const store = await opened();
  store.setViewNodes('overview', ['gateway-1']);
  store.deleteView('overview');
  await settle();
  expect(store.views.map((view) => view.id)).toEqual(['overview', 'orders']);
  expect(store.views[0].nodes).toBe('*');
  expect(puts('/api/views')).toHaveLength(0);
});

test('deleting the open view returns to the overview and drops its layout', async () => {
  const screen = await show();

  await screen.getByRole('button', { name: 'Orders path 3', exact: true }).click();
  await vi.waitFor(() => expect(rows(screen)[1].active).toBe(true));
  await vi.waitFor(() => expect(puts('/api/layout')).toHaveLength(1));

  await screen.getByRole('button', { name: 'Delete Orders path' }).click();

  await vi.waitFor(() => expect(puts('/api/views')).toHaveLength(1));
  expect(savedViews().views.map((view) => view.id)).toEqual(['overview']);
  expect(rows(screen)).toEqual([{ name: 'Overview', count: '5', active: true }]);
  await vi.waitFor(() => expect(cards(screen)).toHaveLength(5));
  await vi.waitFor(() => expect(puts('/api/layout')).toHaveLength(2));
  expect(Object.keys(savedLayout().views)).toEqual(['overview']);
});

test('undo puts a membership change back and redo makes it again', async () => {
  const store = await opened();

  store.toggleViewNode('orders', 'queue-1');
  expect(store.views[1].nodes).toEqual(['gateway-1', 'function-1', 'database-1', 'queue-1']);
  await vi.waitFor(() => expect(puts('/api/views')).toHaveLength(1));

  await store.undo();

  expect(store.views[1].nodes).toEqual(['gateway-1', 'function-1', 'database-1']);
  expect(nodesOf(savedViews(), 'orders')).toEqual(['gateway-1', 'function-1', 'database-1']);
  expect(puts('/api/project')).toHaveLength(0);
  expect(store.canRedo).toBe(true);

  await store.redo();

  expect(store.views[1].nodes).toEqual(['gateway-1', 'function-1', 'database-1', 'queue-1']);
  expect(nodesOf(savedViews(), 'orders')).toContain('queue-1');
  expect(puts('/api/project')).toHaveLength(0);
});

test('undo removes a created view and redo brings it back, layout and all', async () => {
  const store = await opened();
  store.select('queue-1');

  store.createView('Data stores');
  expect(store.activeView).toBe('data-stores');
  expect(store.editing).toBe(true);
  expect(store.views[2].nodes).toEqual(['queue-1']);
  await vi.waitFor(() => expect(puts('/api/views')).toHaveLength(1));
  await vi.waitFor(() => expect(puts('/api/layout')).toHaveLength(1));
  const placedAt = store.positions['queue-1'];
  expect(placedAt).toBeDefined();
  store.moveNode('queue-1', { x: 77, y: 88 });
  await vi.waitFor(() => expect(puts('/api/layout')).toHaveLength(2));

  await store.undo();

  expect(store.views.map((view) => view.id)).toEqual(['overview', 'orders']);
  expect(store.activeView).toBe('overview');
  expect(store.editing).toBe(false);
  expect(savedViews().views.map((view) => view.id)).toEqual(['overview', 'orders']);
  expect(Object.keys(savedLayout().views).sort()).toEqual(['orders', 'overview']);

  await store.redo();

  expect(store.views.map((view) => view.id)).toEqual(['overview', 'orders', 'data-stores']);
  expect(store.activeView).toBe('overview');
  expect(savedLayout().views['data-stores'].nodes['queue-1']).toEqual({ x: 77, y: 88 });
});

test('a moved node is not undone with the view change before it', async () => {
  const store = await opened();

  store.toggleViewNode('orders', 'queue-1');
  store.moveNode('gateway-1', { x: 1, y: 1 });
  await settle();

  await store.undo();

  expect(store.positions['gateway-1']).toEqual({ x: 1, y: 1 });
  expect(store.canUndo).toBe(false);
});

test('a refused views file is reported and put back', async () => {
  refuse((call) =>
    call.method === 'PUT' && call.path === '/api/views'
      ? invalid([
          { path: 'views.1.nodes.3', message: "view 'orders' refers to missing node 'queue-1'" },
        ])
      : undefined,
  );
  const screen = await show();

  await screen.getByRole('button', { name: 'Edit Orders path' }).click();
  await vi.waitFor(() => expect(editor(screen)).not.toBeNull());
  box(screen, 'queue-1').click();
  await vi.waitFor(() => expect(dimmed(screen, 'queue-1')).toBe(false));

  await expect
    .element(screen.getByText("views.1.nodes.3: view 'orders' refers to missing node 'queue-1'"))
    .toBeInTheDocument();
  await vi.waitFor(() => expect(box(screen, 'queue-1').checked).toBe(false));
  expect(dimmed(screen, 'queue-1')).toBe(true);
  expect(rows(screen)[1].count).toBe('3');
});

test('a views-changed event reloads the views', async () => {
  const screen = await show();
  await vi.waitFor(() => expect(rows(screen)).toHaveLength(2));

  serve(shop, placed, undefined, {
    version: 1,
    views: [
      ...views.views,
      { id: 'stores', name: 'Data stores', nodes: ['database-1', 'service-1'] },
    ],
  });
  socket()?.onmessage?.({ data: JSON.stringify({ event: 'views-changed' }) });

  await vi.waitFor(() =>
    expect(rows(screen)).toEqual([
      { name: 'Overview', count: '5', active: true },
      { name: 'Orders path', count: '3', active: false },
      { name: 'Data stores', count: '2', active: false },
    ]),
  );
});

test('a views file the studio cannot read degrades to the overview and says why', async () => {
  refuse((call) =>
    call.method === 'GET' && call.path === '/api/views'
      ? invalid([{ path: 'views', message: "every project has an 'overview' view" }])
      : undefined,
  );
  const screen = await show();

  await vi.waitFor(() =>
    expect(rows(screen)).toEqual([{ name: 'Overview', count: '5', active: true }]),
  );
  await expect
    .element(screen.getByText("views: every project has an 'overview' view"))
    .toBeInTheDocument();
});

test('deleting a node takes it out of every view, and undo puts it back in, project first', async () => {
  const store = await opened();

  await store.deleteNode('database-1');

  expect(store.views[1].nodes).toEqual(['gateway-1', 'function-1']);
  expect(nodesOf(savedViews(), 'orders')).toEqual(['gateway-1', 'function-1']);

  await store.undo();

  expect(store.views[1].nodes).toEqual(['gateway-1', 'function-1', 'database-1']);
  expect(nodesOf(savedViews(), 'orders')).toEqual(['gateway-1', 'function-1', 'database-1']);
  const order = calls()
    .filter((call) => call.method === 'PUT')
    .map((call) => call.path);
  expect(order.lastIndexOf('/api/project')).toBeLessThan(order.lastIndexOf('/api/views'));
});

test('a node dropped while a view is open joins that view', async () => {
  const store = await opened();
  store.openView('orders');

  await store.addNode('cache', { x: 10, y: 10 });

  expect(store.views[1].nodes).toEqual(['gateway-1', 'function-1', 'database-1', 'cache-1']);
  expect(nodesOf(savedViews(), 'orders')).toContain('cache-1');
  expect(savedLayout().views.orders.nodes['cache-1']).toEqual({ x: 10, y: 10 });
  expect(store.visibleNodeIds.has('cache-1')).toBe(true);
});

test('a refused drop leaves the view as it was', async () => {
  refuse((call) =>
    call.method === 'PUT' && call.path === '/api/project'
      ? invalid([
          { path: 'nodes', nodeId: 'gateway-2', message: 'a project can have at most one gateway' },
        ])
      : undefined,
  );
  const store = await opened();
  store.openView('orders');

  await store.addNode('gateway', { x: 10, y: 10 });

  expect(store.views[1].nodes).toEqual(['gateway-1', 'function-1', 'database-1']);
  expect(puts('/api/views')).toHaveLength(0);
});
