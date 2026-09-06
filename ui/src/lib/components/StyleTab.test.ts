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
    { id: 'database-1', type: 'database', name: 'orders-db' },
    { id: 'database-2', type: 'database', name: 'reports-db' },
  ],
  edges: [],
};

const placed = overviewLayout({
    'gateway-1': { x: 20, y: 20 },
    'database-1': { x: 260, y: 20 },
    'database-2': { x: 260, y: 140 },
  });

const kinds: Config = {
  ...defaults,
  style: { theme: 'dark', kinds: { database: { color: '#112233' } } },
};

const nodes: Config = {
  ...defaults,
  style: {
    theme: 'dark',
    kinds: { database: { color: '#112233' } },
    nodes: { 'orders-db': { icon: './icons/orders.svg' } },
  },
};

const scheme = { value: '', source: 'provider scheme' };

const ordersSnippet = [
  'style:',
  '  nodes:',
  '    orders-db:',
  '      color: "#C925D1"',
  '      icon: aws/rds',
  '      shape: cylinder',
].join('\n');

const databaseSnippet = [
  'style:',
  '  kinds:',
  '    database:',
  '      color: "#C925D1"',
  '      icon: aws/rds',
  '      shape: cylinder',
].join('\n');

type Screen = Awaited<ReturnType<typeof show>>;

function panel(screen: Screen): Element {
  const found = screen.container.querySelector('aside[aria-label="Inspector"]');
  expect(found).not.toBeNull();
  return found!;
}

async function open(screen: Screen, id: string, type: string) {
  const card = await vi.waitFor(() => {
    const found = screen.container.querySelector(`.svelte-flow__node[data-id="${id}"]`);
    expect(found).not.toBeNull();
    return found!;
  });
  card.dispatchEvent(new MouseEvent('click', { bubbles: true }));
  await vi.waitFor(() => expect(panel(screen).querySelector('header span')?.textContent).toBe(type));
}

async function openStyle(screen: Screen, id: string, type: string) {
  await open(screen, id, type);
  await screen.getByRole('tab', { name: 'Style' }).click();
  await vi.waitFor(() => expect(Object.keys(rows(screen))).toHaveLength(3));
}

function tabs(screen: Screen): Record<string, boolean> {
  const out: Record<string, boolean> = {};
  for (const tab of panel(screen).querySelectorAll('[role="tab"]')) {
    out[tab.textContent ?? ''] = tab.getAttribute('aria-selected') === 'true';
  }
  return out;
}

// The three Applied rows by label: what is drawn and where it came from.
function rows(screen: Screen): Record<string, { value: string; source: string }> {
  const out: Record<string, { value: string; source: string }> = {};
  for (const row of panel(screen).querySelectorAll('dl > div')) {
    const [value, source] = [...row.querySelectorAll('dd')].map((cell) => cell.textContent?.trim());
    out[row.querySelector('dt')?.textContent ?? ''] = { value: value ?? '', source: source ?? '' };
  }
  return out;
}

function snippet(screen: Screen, which: 'nodes' | 'kinds'): string {
  return panel(screen).querySelector(`pre[data-snippet="${which}"]`)?.textContent ?? '';
}

function preview(screen: Screen): HTMLElement {
  const found = panel(screen).querySelector<HTMLElement>('[data-icon]');
  expect(found).not.toBeNull();
  return found!;
}

function swatch(screen: Screen): string {
  const found = panel(screen).querySelector('dl > div:first-child dd > span');
  expect(found).not.toBeNull();
  return getComputedStyle(found!).backgroundColor;
}

// navigator.clipboard is a getter on the prototype, so an own property on the
// instance stands in for it and deleting that puts the real one back.
function clipboard(writeText: ((text: string) => Promise<void>) | undefined) {
  Object.defineProperty(navigator, 'clipboard', {
    value: writeText === undefined ? undefined : { writeText },
    configurable: true,
  });
}

beforeEach(() => {
  reset();
});

afterEach(() => {
  delete (navigator as { clipboard?: Clipboard }).clipboard;
  vi.unstubAllGlobals();
});

test('the inspector opens on Properties and the chosen tab outlives the selection', async () => {
  serve(shop, placed);
  const screen = await show();
  await open(screen, 'gateway-1', 'gateway');

  expect(tabs(screen)).toEqual({ Properties: true, Style: false });
  expect(panel(screen).querySelector('input')).not.toBeNull();
  expect(panel(screen).querySelector('dl')).toBeNull();

  await screen.getByRole('tab', { name: 'Style' }).click();

  await vi.waitFor(() => expect(tabs(screen)).toEqual({ Properties: false, Style: true }));
  expect(panel(screen).querySelector('input')).toBeNull();
  expect(rows(screen).Icon.value).toBe('aws/api-gateway');

  await open(screen, 'database-1', 'database');

  expect(tabs(screen)).toEqual({ Properties: false, Style: true });
  await vi.waitFor(() => expect(rows(screen).Icon.value).toBe('aws/rds'));

  await screen.getByRole('tab', { name: 'Properties' }).click();

  await vi.waitFor(() => expect(panel(screen).querySelector('input')).not.toBeNull());
  expect(tabs(screen)).toEqual({ Properties: true, Style: false });
});

test('with nothing configured every row comes from the provider scheme', async () => {
  serve(shop, placed);
  const screen = await show();
  await openStyle(screen, 'database-1', 'database');

  expect(rows(screen)).toEqual({
    Colour: { ...scheme, value: '#C925D1' },
    Icon: { ...scheme, value: 'aws/rds' },
    Shape: { ...scheme, value: 'cylinder' },
  });
  expect(preview(screen).dataset.icon).toBe('aws/rds');
  expect(preview(screen).dataset.shape).toBe('cylinder');
  expect(swatch(screen)).toBe('rgb(201, 37, 209)');
  expect(snippet(screen, 'nodes')).toBe(ordersSnippet);
  expect(snippet(screen, 'kinds')).toBe(databaseSnippet);
  expect(panel(screen).textContent).toContain('Or for every database');
  const note = panel(screen).querySelector('p');
  expect(note?.textContent?.replace(/\s+/g, ' ')).toContain(
    'The studio reads that file and never writes it',
  );
  expect(note?.querySelector('code')?.textContent).toBe('togen.yml');
});

test('a kind override is named on the row it changes and the others stay with the scheme', async () => {
  serve(shop, placed, kinds);
  const screen = await show();
  await openStyle(screen, 'database-1', 'database');

  expect(rows(screen)).toEqual({
    Colour: { value: '#112233', source: 'kind override' },
    Icon: { ...scheme, value: 'aws/rds' },
    Shape: { ...scheme, value: 'cylinder' },
  });
  expect(swatch(screen)).toBe('rgb(17, 34, 51)');
  expect(snippet(screen, 'nodes')).toContain('      color: "#112233"');
  expect(snippet(screen, 'kinds')).toContain('      color: "#112233"');
});

test('a node override is named on its row, the others keep their sources, and the kind snippet leaves it out', async () => {
  serve(shop, placed, nodes);
  const screen = await show();
  await openStyle(screen, 'database-1', 'database');

  expect(rows(screen)).toEqual({
    Colour: { value: '#112233', source: 'kind override' },
    Icon: { value: './icons/orders.svg', source: 'node override' },
    Shape: { ...scheme, value: 'cylinder' },
  });
  expect(preview(screen).querySelector('img')?.getAttribute('src')).toBe(
    '/api/icon?path=.%2Ficons%2Forders.svg',
  );
  expect(snippet(screen, 'nodes')).toBe(
    [
      'style:',
      '  nodes:',
      '    orders-db:',
      '      color: "#112233"',
      '      icon: ./icons/orders.svg',
      '      shape: cylinder',
    ].join('\n'),
  );
  expect(snippet(screen, 'kinds')).toBe(
    [
      'style:',
      '  kinds:',
      '    database:',
      '      color: "#112233"',
      '      icon: aws/rds',
      '      shape: cylinder',
    ].join('\n'),
  );

  await open(screen, 'database-2', 'database');

  await vi.waitFor(() => expect(rows(screen).Icon).toEqual({ ...scheme, value: 'aws/rds' }));
  expect(snippet(screen, 'nodes')).toContain('    reports-db:');
});

test('copy writes the snippet to the clipboard and says so for a moment', async () => {
  const writeText = vi.fn(async () => {});
  clipboard(writeText);
  serve(shop, placed);
  const screen = await show();
  await openStyle(screen, 'database-1', 'database');

  await screen.getByRole('button', { name: 'Copy the style.nodes snippet' }).click();

  await expect.element(screen.getByText('Copied')).toBeInTheDocument();
  expect(writeText).toHaveBeenCalledWith(ordersSnippet);
  await vi.waitFor(() => expect(panel(screen).textContent).not.toContain('Copied'), {
    timeout: 4000,
  });

  await screen.getByRole('button', { name: 'Copy the style.kinds snippet' }).click();

  await expect.element(screen.getByText('Copied')).toBeInTheDocument();
  expect(writeText).toHaveBeenLastCalledWith(databaseSnippet);
});

test('without a clipboard the button is still there and says so', async () => {
  clipboard(undefined);
  serve(shop, placed);
  const screen = await show();
  await openStyle(screen, 'database-1', 'database');

  await screen.getByRole('button', { name: 'Copy the style.nodes snippet' }).click();

  await expect.element(screen.getByText('Clipboard unavailable')).toBeInTheDocument();
  expect(panel(screen).textContent).not.toContain('Copied');
});

test('the rows follow togen.yml without reselecting the node', async () => {
  serve(shop, placed);
  const screen = await show();
  await openStyle(screen, 'database-1', 'database');
  expect(rows(screen).Colour).toEqual({ ...scheme, value: '#C925D1' });

  serve(shop, placed, nodes);
  socket()?.onmessage?.({ data: JSON.stringify({ event: 'config-changed' }) });

  await vi.waitFor(() =>
    expect(rows(screen)).toEqual({
      Colour: { value: '#112233', source: 'kind override' },
      Icon: { value: './icons/orders.svg', source: 'node override' },
      Shape: { ...scheme, value: 'cylinder' },
    }),
  );
  expect(tabs(screen)).toEqual({ Properties: false, Style: true });
  expect(snippet(screen, 'nodes')).toContain('      icon: ./icons/orders.svg');
});
