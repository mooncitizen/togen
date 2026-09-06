import { afterEach, beforeEach, expect, test, vi } from 'vitest';

import {
  gets,
  invalid,
  puts,
  refuse,
  reset,
  serve,
  serveCost,
  settle,
  show,
  socket,
  overviewLayout,
} from '../../harness.ts';
import { Store } from '../store.svelte.ts';
import type { Cost, Project } from '../types.ts';

const shop: Project = {
  version: 1,
  name: 'shop',
  provider: 'aws',
  region: 'eu-west-2',
  environment: 'dev',
  nodes: [
    { id: 'gateway-1', type: 'gateway', name: 'api' },
    { id: 'function-1', type: 'function', name: 'orders' },
    { id: 'database-1', type: 'database', name: 'orders-db', properties: { engine: 'postgres' } },
    { id: 'service-1', type: 'service', name: 'web', properties: { image: 'nginx:1.27' } },
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
    'service-1': { x: 740, y: 20 },
  });

// The aws-basic golden table, as GET /api/cost sends it.
const estimate: Cost = {
  provider: 'aws',
  region: 'eu-west-2',
  currency: 'USD',
  items: [
    {
      name: 'api',
      kind: 'gateway',
      summary: 'HTTP API',
      note: 'priced at rest, usage not set',
      lines: [],
      subtotal: 0,
    },
    {
      name: 'orders',
      kind: 'function',
      summary: 'node, 512 MB, x86_64',
      note: 'priced at rest, usage not set',
      lines: [],
      subtotal: 0,
    },
    {
      name: 'orders-db',
      kind: 'database',
      summary: 'db.t4g.micro, postgres 17, single-AZ, 20 GB',
      lines: [
        {
          label: 'instance',
          quantity: 730,
          unit: 'h',
          unitPrice: 0.018,
          amount: 13.14,
          sku: 'sku-1',
        },
        {
          label: 'storage gp2',
          quantity: 20,
          unit: 'GB',
          unitPrice: 0.133,
          amount: 2.66,
          sku: 'sku-2',
        },
      ],
      subtotal: 15.8,
    },
    {
      name: 'network',
      kind: 'implicit',
      summary: 'VPC',
      lines: [
        {
          label: 'nat gateway',
          quantity: 730,
          unit: 'h',
          unitPrice: 0.05,
          amount: 36.5,
          sku: 'sku-3',
        },
      ],
      subtotal: 36.5,
    },
  ],
  notPriced: [
    { name: 'web', kind: 'service', reason: 'no aws prices for this node type yet' },
    { name: 'api', kind: 'gateway', reason: 'requests' },
    { name: 'api', kind: 'gateway', reason: 'data transfer' },
    { name: 'orders', kind: 'function', reason: 'requests' },
    { name: 'orders', kind: 'function', reason: 'duration' },
    { name: 'orders-db', kind: 'database', reason: 'backups beyond 20 GB' },
    { name: 'network', kind: 'implicit', reason: 'nat gateway data processed' },
  ],
  total: 52.3,
  snapshotDate: '2026-09-05',
  note: 'list prices from 2026-09-05, estimate not a quote',
};

type Screen = Awaited<ReturnType<typeof show>>;

function drawer(screen: Screen): Element | null {
  return screen.container.querySelector('aside[aria-label="Cost"]');
}

function inspector(screen: Screen): Element | null {
  return screen.container.querySelector('aside[aria-label="Inspector"]');
}

async function opened(screen: Screen): Promise<Element> {
  await screen.getByTitle('Monthly estimate').click();
  return vi.waitFor(() => {
    const found = drawer(screen);
    expect(found).not.toBeNull();
    return found!;
  });
}

function badge(screen: Screen, id: string): string | undefined {
  const card = `.svelte-flow__node[data-id="${id}"]`;
  return screen.container
    .querySelector(`${card} [title="Monthly subtotal"], ${card} [title="Not priced"]`)
    ?.textContent?.trim();
}

function click(screen: Screen, id: string) {
  screen.container
    .querySelector(`.svelte-flow__node[data-id="${id}"]`)!
    .dispatchEvent(new MouseEvent('click', { bubbles: true }));
}

function highlighted(screen: Screen): string[] {
  return [...(drawer(screen)?.querySelectorAll('[data-selected]') ?? [])].map(
    (row) => row.textContent ?? '',
  );
}

beforeEach(() => {
  reset();
  serve(shop, placed);
  serveCost(estimate);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test('the pill shows the monthly total and opens the table the CLI prints', async () => {
  const screen = await show();

  await expect.element(screen.getByRole('button', { name: '52.30 USD/mo' })).toBeInTheDocument();
  expect(drawer(screen)).toBeNull();

  const panel = await opened(screen);

  expect(panel.getBoundingClientRect().width).toBe(320);
  await expect
    .element(screen.getByText('db.t4g.micro, postgres 17, single-AZ, 20 GB'))
    .toBeInTheDocument();
  const text = panel.textContent ?? '';
  for (const expected of [
    'api',
    'gateway',
    'HTTP API',
    'priced at rest, usage not set',
    'node, 512 MB, x86_64',
    'orders-db',
    'database',
    'instance',
    '730 h × 0.0180',
    '13.14',
    'storage gp2',
    '20 GB × 0.1330',
    '2.66',
    '15.80',
    'network',
    'implicit',
    'VPC',
    'nat gateway',
    '36.50',
    'Not priced',
    'web',
    'service',
    'no aws prices for this node type yet',
    'requests',
    'data transfer',
    'duration',
    'backups beyond 20 GB',
    'nat gateway data processed',
    '52.30 USD/month',
    'eu-west-2, list prices from 2026-09-05, estimate not a quote',
  ]) {
    expect(text).toContain(expected);
  }
  expect(text).not.toContain('days old');
  expect(text.match(/priced at rest, usage not set/g)).toHaveLength(2);

  await screen.getByRole('button', { name: 'Close cost' }).click();
  await vi.waitFor(() => expect(drawer(screen)).toBeNull());
});

test('the footer carries the staleness warning when there is one', async () => {
  serveCost({ ...estimate, warning: 'the bundled aws prices are 100 days old (taken 2026-09-05)' });
  const screen = await show();

  await opened(screen);

  await expect
    .element(screen.getByText('the bundled aws prices are 100 days old (taken 2026-09-05)'))
    .toBeInTheDocument();
});

test('the drawer takes the place of the inspector and gives it back', async () => {
  const screen = await show();
  click(screen, 'database-1');
  await vi.waitFor(() => expect(inspector(screen)).not.toBeNull());

  await opened(screen);
  await vi.waitFor(() => expect(inspector(screen)).toBeNull());

  await screen.getByRole('button', { name: 'Close cost' }).click();

  await vi.waitFor(() => expect(inspector(screen)).not.toBeNull());
  await vi.waitFor(() => expect(drawer(screen)).toBeNull());
});

test('selecting a node while the drawer is open highlights its rows', async () => {
  const screen = await show();
  await opened(screen);
  expect(highlighted(screen)).toHaveLength(0);

  click(screen, 'database-1');

  await vi.waitFor(() => expect(highlighted(screen)).toHaveLength(2));
  const [priced, omitted] = highlighted(screen);
  expect(priced).toContain('orders-db');
  expect(priced).toContain('15.80');
  expect(omitted).toContain('backups beyond 20 GB');
  expect(inspector(screen)).toBeNull();

  click(screen, 'gateway-1');

  await vi.waitFor(() => expect(highlighted(screen)).toHaveLength(3));
  const [atRest, requests, transfer] = highlighted(screen);
  expect(atRest).toContain('priced at rest, usage not set');
  expect(atRest).toContain('0.00');
  expect(requests).toContain('requests');
  expect(transfer).toContain('data transfer');

  click(screen, 'service-1');

  await vi.waitFor(() => expect(highlighted(screen)).toHaveLength(1));
  expect(highlighted(screen)[0]).toContain('no aws prices for this node type yet');
});

test('each card carries its subtotal, 0.00 when it is priced at rest, or a dash when it is not priced', async () => {
  const screen = await show();

  await vi.waitFor(() => expect(badge(screen, 'database-1')).toBe('15.80'));
  expect(badge(screen, 'gateway-1')).toBe('0.00');
  expect(badge(screen, 'function-1')).toBe('0.00');
  expect(badge(screen, 'service-1')).toBe('–');
});

test('a project the studio cannot price lists why, and the last total goes with it', async () => {
  const screen = await show();
  await opened(screen);
  await expect.element(screen.getByText('52.30 USD/month')).toBeInTheDocument();

  refuse((call) =>
    call.path === '/api/cost'
      ? invalid([
          { path: 'nodes', nodeId: 'gateway-2', message: 'a project can have at most one gateway' },
        ])
      : undefined,
  );
  socket()?.onmessage?.({ data: JSON.stringify({ event: 'project-changed' }) });

  await expect
    .element(screen.getByText('nodes (node gateway-2): a project can have at most one gateway'))
    .toBeInTheDocument();
  expect(drawer(screen)!.textContent).not.toContain('USD');
  await expect.element(screen.getByTitle('Monthly estimate')).toHaveTextContent('–');
  await vi.waitFor(() => expect(badge(screen, 'database-1')).toBe('–'));
});

test('a project-changed event refetches the estimate', async () => {
  const screen = await show();
  await expect.element(screen.getByRole('button', { name: '52.30 USD/mo' })).toBeInTheDocument();

  serveCost({ ...estimate, total: 60.1 });
  socket()?.onmessage?.({ data: JSON.stringify({ event: 'project-changed' }) });

  await expect.element(screen.getByRole('button', { name: '60.10 USD/mo' })).toBeInTheDocument();
  expect(gets('/api/cost')).toHaveLength(2);
});

test('an edit moves the number once its save has landed', async () => {
  const screen = await show();
  await expect.element(screen.getByRole('button', { name: '52.30 USD/mo' })).toBeInTheDocument();
  click(screen, 'database-1');

  serveCost({ ...estimate, total: 70.88 });
  await screen.getByLabelText('Storage gb', { exact: true }).fill('200');

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  expect(gets('/api/cost')).toHaveLength(1);
  await expect.element(screen.getByRole('button', { name: '70.88 USD/mo' })).toBeInTheDocument();
  expect(gets('/api/cost')).toHaveLength(2);
});

test('two saves in quick succession ask for the estimate once', async () => {
  const store = new Store();
  await store.load();
  expect(gets('/api/cost')).toHaveLength(1);

  await store.addNode('queue', { x: 0, y: 0 });
  await store.addNode('bucket', { x: 0, y: 200 });
  expect(puts('/api/project')).toHaveLength(2);
  expect(gets('/api/cost')).toHaveLength(1);

  await settle();
  expect(gets('/api/cost')).toHaveLength(2);
});
