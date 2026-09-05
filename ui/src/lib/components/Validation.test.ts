import { afterEach, beforeEach, expect, test, vi } from 'vitest';

import { overviewLayout, reset, serve, show } from '../../harness.ts';
import type { Project } from '../types.ts';

const shop: Project = {
  version: 1,
  name: 'shop',
  provider: 'aws',
  region: 'eu-west-2',
  environment: 'dev',
  nodes: [
    { id: 'gateway-1', type: 'gateway', name: 'api' },
    { id: 'function-1', type: 'function', name: 'orders' },
  ],
  edges: [{ id: 'edge-1', from: 'gateway-1', to: 'function-1', relation: 'routes' }],
};

const placed = overviewLayout({ 'gateway-1': { x: 20, y: 20 }, 'function-1': { x: 260, y: 20 } });

type Screen = Awaited<ReturnType<typeof show>>;

function badged(screen: Screen, id: string): boolean {
  return screen.container.querySelector(`.svelte-flow__node[data-id="${id}"] .bg-err`) !== null;
}

function marked(screen: Screen, id: string): boolean {
  return screen.container.querySelector(`.svelte-flow__edge[data-id="${id}"].togen-edge-error`) !== null;
}

beforeEach(() => {
  reset();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test('a second gateway is counted, listed in the CLI words and badged on the card', async () => {
  serve(
    {
      ...shop,
      nodes: [...shop.nodes, { id: 'gateway-2', type: 'gateway', name: 'admin' }],
    },
    overviewLayout({ ...placed.views.overview.nodes, 'gateway-2': { x: 20, y: 200 } }),
  );
  const screen = await show();

  await screen.getByRole('button', { name: '1 problem' }).click();

  await expect
    .element(screen.getByText('nodes (node gateway-2): a project can have at most one gateway'))
    .toBeInTheDocument();
  await vi.waitFor(() => expect(badged(screen, 'gateway-2')).toBe(true));
  expect(badged(screen, 'gateway-1')).toBe(false);
});

test('a node named like another says so, and stops saying it once it is renamed', async () => {
  serve(
    { ...shop, nodes: [shop.nodes[0], { ...shop.nodes[1], name: 'api' }] },
    { ...placed },
  );
  const screen = await show();

  await screen.getByRole('button', { name: '1 problem' }).click();
  await expect
    .element(screen.getByText("nodes.1 (node function-1): duplicate node name 'api'"))
    .toBeInTheDocument();

  screen.container
    .querySelector('.svelte-flow__node[data-id="function-1"]')!
    .dispatchEvent(new MouseEvent('click', { bubbles: true }));
  await screen.getByLabelText('Name', { exact: true }).fill('orders');

  await vi.waitFor(() =>
    expect(screen.container.querySelector('ul[aria-label="Problems"]')).toBeNull(),
  );
  expect(badged(screen, 'function-1')).toBe(false);
});

test('an edge the relation table refuses is marked on the canvas', async () => {
  serve(
    {
      ...shop,
      nodes: [...shop.nodes, { id: 'database-1', type: 'database', name: 'orders-db' }],
      edges: [
        ...shop.edges,
        { id: 'edge-2', from: 'database-1', to: 'function-1', relation: 'calls' },
      ],
    },
    overviewLayout({ ...placed.views.overview.nodes, 'database-1': { x: 20, y: 200 } }),
  );
  const screen = await show();

  await screen.getByRole('button', { name: '1 problem' }).click();

  await expect
    .element(
      screen.getByText("edges.1 (edge edge-2): a database cannot have a 'calls' edge to a function"),
    )
    .toBeInTheDocument();
  await vi.waitFor(() => expect(marked(screen, 'edge-2')).toBe(true));
  expect(marked(screen, 'edge-1')).toBe(false);
});

// Svelte Flow has nothing to draw an edge to a node that is not there, so the
// list is the only place it can show.
test('an edge with no node at the end of it is listed', async () => {
  serve(
    { ...shop, edges: [{ id: 'edge-1', from: 'gateway-1', to: 'ghost', relation: 'routes' }] },
    placed,
  );
  const screen = await show();

  await screen.getByRole('button', { name: '1 problem' }).click();

  await expect
    .element(screen.getByText("edges.0 (edge edge-1): edge refers to missing node 'ghost'"))
    .toBeInTheDocument();
});

test('a sound project counts nothing', async () => {
  serve(shop, placed);
  const screen = await show();

  expect(screen.container.querySelector('header button[aria-expanded]')).toBeNull();
  expect(badged(screen, 'gateway-1')).toBe(false);
});
