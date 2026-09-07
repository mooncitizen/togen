import { afterEach, beforeEach, expect, test, vi } from 'vitest';

import { invalid, overviewLayout, puts, refuse, reset, serve, settle, show } from '../../harness.ts';
import { Store } from '../store.svelte.ts';
import type { Layout, Project } from '../types.ts';

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
    { id: 'service-1', type: 'service', name: 'web', properties: { image: 'nginx:1.27' } },
    { id: 'queue-1', type: 'queue', name: 'jobs' },
  ],
  edges: [
    { id: 'edge-1', from: 'gateway-1', to: 'function-1', relation: 'routes' },
    { id: 'edge-2', from: 'function-1', to: 'database-1', relation: 'reads' },
  ],
};

const placed = overviewLayout({
  'gateway-1': { x: 20, y: 20 },
  'function-1': { x: 260, y: 20 },
  'database-1': { x: 500, y: 20 },
  'service-1': { x: 20, y: 200 },
  'queue-1': { x: 500, y: 200 },
});

type Screen = Awaited<ReturnType<typeof show>>;

function saved(index = -1): Project {
  const call = puts('/api/project').at(index);
  expect(call).toBeDefined();
  return call!.body as Project;
}

function panel(screen: Screen): Element | null {
  return screen.container.querySelector('aside[aria-label="Inspector"]');
}

async function card(screen: Screen, id: string): Promise<Element> {
  return await vi.waitFor(() => {
    const found = screen.container.querySelector(`.svelte-flow__node[data-id="${id}"]`);
    expect(found).not.toBeNull();
    return found!;
  });
}

function handle(card: Element, kind: 'source' | 'target'): Element {
  const found = card.querySelector(`.svelte-flow__handle.${kind}`);
  expect(found).not.toBeNull();
  return found!;
}

function centre(element: Element): { x: number; y: number } {
  const box = element.getBoundingClientRect();
  return { x: box.left + box.width / 2, y: box.top + box.height / 2 };
}

// The real thing: a press on the source handle, a move to the target handle and a
// release, which is what Svelte Flow turns into a connection.
async function connect(screen: Screen, from: string, to: string) {
  const source = handle(await card(screen, from), 'source');
  const target = handle(await card(screen, to), 'target');
  const at = centre(source);
  const on = centre(target);
  source.dispatchEvent(
    new MouseEvent('mousedown', { bubbles: true, clientX: at.x, clientY: at.y }),
  );
  document.dispatchEvent(
    new MouseEvent('mousemove', { bubbles: true, clientX: on.x, clientY: on.y }),
  );
  document.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, clientX: on.x, clientY: on.y }));
  await settle(50);
}

async function clickEdge(screen: Screen, id: string) {
  const edge = await vi.waitFor(() => {
    const found = screen.container.querySelector(`.svelte-flow__edge[data-id="${id}"]`);
    expect(found).not.toBeNull();
    return found!;
  });
  edge.dispatchEvent(new MouseEvent('click', { bubbles: true }));
  await vi.waitFor(() => expect(panel(screen)).not.toBeNull());
}

function menuItems(screen: Screen): string[] {
  const menu = screen.container.querySelector('[role="menu"]');
  return menu === null
    ? []
    : [...menu.querySelectorAll('button')].map((button) => button.textContent?.trim() ?? '');
}

function edgeCount(screen: Screen): number {
  return screen.container.querySelectorAll('.svelte-flow__edge').length;
}

beforeEach(() => {
  reset();
  serve(shop, placed);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test('a pair with one legal relation is connected straight away and saved', async () => {
  const screen = await show();

  await connect(screen, 'gateway-1', 'service-1');

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  expect(saved().edges.at(-1)).toEqual({
    id: 'edge-3',
    from: 'gateway-1',
    to: 'service-1',
    relation: 'routes',
  });
  expect(menuItems(screen)).toEqual([]);
});

test('a pair with several legal relations asks, in the order the table lists them', async () => {
  const screen = await show();

  await connect(screen, 'service-1', 'database-1');

  await vi.waitFor(() => expect(menuItems(screen)).toEqual(['reads', 'writes']));
  expect(puts('/api/project')).toHaveLength(0);

  await screen.getByRole('menuitem', { name: 'writes' }).click();

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  expect(saved().edges.at(-1)).toEqual({
    id: 'edge-3',
    from: 'service-1',
    to: 'database-1',
    relation: 'writes',
  });
  expect(menuItems(screen)).toEqual([]);
});

test('escape cancels the menu and nothing is added, and the ghost edge goes with it', async () => {
  const screen = await show();
  const before = edgeCount(screen);

  await connect(screen, 'service-1', 'database-1');
  await vi.waitFor(() => expect(menuItems(screen)).toEqual(['reads', 'writes']));

  screen.container
    .querySelector('[role="menu"]')!
    .dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));

  await vi.waitFor(() => expect(menuItems(screen)).toEqual([]));
  await settle();
  expect(puts('/api/project')).toHaveLength(0);
  expect(edgeCount(screen)).toBe(before);
});

test('clicking away cancels the menu and nothing is added, and the ghost edge goes with it', async () => {
  const screen = await show();
  const before = edgeCount(screen);

  await connect(screen, 'service-1', 'database-1');
  await vi.waitFor(() => expect(menuItems(screen)).toEqual(['reads', 'writes']));

  screen.container
    .querySelector('[aria-label="Cancel"]')!
    .dispatchEvent(new MouseEvent('click', { bubbles: true }));

  await vi.waitFor(() => expect(menuItems(screen)).toEqual([]));
  await settle();
  expect(puts('/api/project')).toHaveLength(0);
  expect(edgeCount(screen)).toBe(before);
});

test('a compute node dropped on a queue offers consumes before publishes, the order the relations file lists them', async () => {
  const screen = await show();

  await connect(screen, 'service-1', 'queue-1');

  await vi.waitFor(() => expect(menuItems(screen)).toEqual(['consumes', 'publishes']));
  expect(puts('/api/project')).toHaveLength(0);
});

test('a pair with no legal relation is refused and the bar says why', async () => {
  const screen = await show();

  await connect(screen, 'database-1', 'function-1');

  await expect
    .element(screen.getByText('no relation is allowed from a database to a function'))
    .toBeInTheDocument();
  await settle();
  expect(puts('/api/project')).toHaveLength(0);
});

test('a node cannot be connected to itself', async () => {
  const screen = await show();

  await connect(screen, 'function-1', 'function-1');

  await expect
    .element(screen.getByText('an edge cannot connect a node to itself'))
    .toBeInTheDocument();
  await settle();
  expect(puts('/api/project')).toHaveLength(0);
});

test('an edge the project already has is refused in the words the CLI uses', async () => {
  const screen = await show();

  await connect(screen, 'function-1', 'database-1');
  await vi.waitFor(() => expect(menuItems(screen)).toEqual(['reads', 'writes']));
  await screen.getByRole('menuitem', { name: 'reads' }).click();

  await expect
    .element(screen.getByText("duplicate 'reads' edge from 'orders' to 'orders-db'"))
    .toBeInTheDocument();
  await settle();
  expect(puts('/api/project')).toHaveLength(0);
});

test('addEdge reports false for a duplicate, true once saved, and false when the save is refused', async () => {
  const store = new Store();
  await store.load();

  await expect(store.addEdge('function-1', 'database-1', 'reads')).resolves.toBe(false);

  await expect(store.addEdge('gateway-1', 'service-1', 'routes')).resolves.toBe(true);

  refuse((call) =>
    call.method === 'PUT' && call.path === '/api/project'
      ? invalid([{ path: 'edges.3', message: 'the studio said no' }])
      : undefined,
  );
  await expect(store.addEdge('service-1', 'queue-1', 'consumes')).resolves.toBe(false);
});

test('edges are drawn with square corners', async () => {
  const store = new Store();
  await store.load();

  const [edge] = store.flowEdges;
  expect(edge.type).toBe('smoothstep');
  if (edge.type !== 'smoothstep') throw new Error('expected a smoothstep edge');
  expect(edge.pathOptions).toEqual({ borderRadius: 4 });
});

test('a refused edge is taken back off the canvas', async () => {
  refuse((call) =>
    call.method === 'PUT' && call.path === '/api/project'
      ? invalid([{ path: 'edges.2', message: 'the studio said no' }])
      : undefined,
  );
  const screen = await show();

  await connect(screen, 'gateway-1', 'service-1');

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  await vi.waitFor(() =>
    expect(screen.container.querySelector('.svelte-flow__edge[data-id="edge-3"]')).toBeNull(),
  );
});

test('selecting a routes edge offers its path and methods, and saves them', async () => {
  const screen = await show();

  await clickEdge(screen, 'edge-1');
  expect(panel(screen)?.querySelector('header span')?.textContent).toBe('routes');
  await expect.element(screen.getByText('api → orders')).toBeInTheDocument();

  await screen.getByLabelText('Path', { exact: true }).fill('/orders');

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  expect(saved().edges[0].properties).toEqual({ path: '/orders' });

  await screen.getByLabelText('POST', { exact: true }).click();
  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(2));
  expect(saved().edges[0].properties).toEqual({ path: '/orders', methods: ['POST'] });
});

test('a path that does not start with a slash says why and is not saved', async () => {
  const screen = await show();

  await clickEdge(screen, 'edge-1');
  await screen.getByLabelText('Path', { exact: true }).fill('orders');

  await settle();
  expect(puts('/api/project')).toHaveLength(0);
  await expect.element(screen.getByText('a path starting with /')).toBeInTheDocument();
});

test('ANY and a named method are mutually exclusive, and none at all leaves the file', async () => {
  const screen = await show();

  await clickEdge(screen, 'edge-1');
  await screen.getByLabelText('ANY', { exact: true }).click();
  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  expect(saved().edges[0].properties).toEqual({ methods: ['ANY'] });

  await screen.getByLabelText('GET', { exact: true }).click();
  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(2));
  expect(saved().edges[0].properties).toEqual({ methods: ['GET'] });

  await screen.getByLabelText('GET', { exact: true }).click();
  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(3));
  expect(saved().edges[0]).toEqual({
    id: 'edge-1',
    from: 'gateway-1',
    to: 'function-1',
    relation: 'routes',
  });
});

test('an edge that is not a route has nothing to set', async () => {
  const screen = await show();

  await clickEdge(screen, 'edge-2');

  expect(panel(screen)?.querySelector('header span')?.textContent).toBe('reads');
  expect(panel(screen)?.querySelectorAll('input')).toHaveLength(0);
  await expect.element(screen.getByText('orders → orders-db')).toBeInTheDocument();
});

test('deleting a node takes its edges with it and saves both files', async () => {
  const screen = await show();

  (await card(screen, 'function-1')).dispatchEvent(new MouseEvent('click', { bubbles: true }));
  await vi.waitFor(() => expect(panel(screen)).not.toBeNull());
  await screen.getByText('Delete node').click();

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  const project = saved();
  expect(project.nodes.map((node) => node.id)).toEqual([
    'gateway-1',
    'database-1',
    'service-1',
    'queue-1',
  ]);
  expect(project.edges).toEqual([]);

  await vi.waitFor(() => expect(puts('/api/layout')).toHaveLength(1));
  expect((puts('/api/layout')[0].body as Layout).views.overview.nodes['function-1']).toBeUndefined();
});

test('deleting an edge from the inspector saves the project', async () => {
  const screen = await show();

  await clickEdge(screen, 'edge-2');
  await screen.getByText('Delete edge').click();

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  expect(saved().edges.map((edge) => edge.id)).toEqual(['edge-1']);
});

test('the delete key removes the selection when the canvas has it', async () => {
  const screen = await show();

  const selected = await card(screen, 'queue-1');
  selected.dispatchEvent(new MouseEvent('click', { bubbles: true }));
  await vi.waitFor(() => expect(panel(screen)).not.toBeNull());
  (selected as HTMLElement).focus();
  selected.dispatchEvent(new KeyboardEvent('keydown', { key: 'Delete', bubbles: true }));

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  expect(saved().nodes.map((node) => node.id)).not.toContain('queue-1');
});

test('a refused delete puts the node and its edges back', async () => {
  refuse((call) =>
    call.method === 'PUT' && call.path === '/api/project'
      ? invalid([{ path: '', message: 'the studio said no' }])
      : undefined,
  );
  const screen = await show();

  (await card(screen, 'function-1')).dispatchEvent(new MouseEvent('click', { bubbles: true }));
  await vi.waitFor(() => expect(panel(screen)).not.toBeNull());
  await screen.getByText('Delete node').click();

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  await vi.waitFor(() => expect(screen.getByText('orders').elements()).toHaveLength(1));
  await vi.waitFor(() =>
    expect(screen.container.querySelector('.svelte-flow__edge[data-id="edge-2"]')).not.toBeNull(),
  );
  await vi.waitFor(() => expect(panel(screen)).not.toBeNull());
  expect(puts('/api/layout')).toHaveLength(0);
});

test('deleting a node clears any edge selection with it, and a refused delete restores both', async () => {
  refuse((call) =>
    call.method === 'PUT' && call.path === '/api/project'
      ? invalid([{ path: '', message: 'the studio said no' }])
      : undefined,
  );
  const store = new Store();
  await store.load();
  store.selectEdge('edge-2');

  const attempt = store.deleteNode('function-1');
  expect(store.selectedEdgeId).toBeNull();

  await attempt;
  expect(store.selectedEdgeId).toBe('edge-2');
});
