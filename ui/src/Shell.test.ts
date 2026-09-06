import { afterEach, beforeEach, expect, test, vi } from 'vitest';

import {
  defaults,
  gets,
  invalid,
  overviewLayout,
  refuse,
  reset,
  serve,
  show,
  socket,
} from './harness.ts';
import type { Project } from './lib/types.ts';

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

const panel = { dark: 'rgb(21, 26, 33)', light: 'rgb(255, 255, 255)' };

type Screen = Awaited<ReturnType<typeof show>>;

function theme(): string | undefined {
  return document.documentElement.dataset.theme;
}

function barBackground(screen: Screen): string {
  return getComputedStyle(screen.container.querySelector('header')!).backgroundColor;
}

// The OS preference, as the studio would see it through matchMedia.
function prefer(dark: boolean) {
  const listeners = new Set<() => void>();
  const media = {
    matches: dark,
    media: '(prefers-color-scheme: dark)',
    addEventListener: (_: string, listener: () => void) => listeners.add(listener),
    removeEventListener: (_: string, listener: () => void) => listeners.delete(listener),
  };
  vi.stubGlobal(
    'matchMedia',
    vi.fn(() => media),
  );
  return (next: boolean) => {
    media.matches = next;
    for (const listener of listeners) {
      listener();
    }
  };
}

beforeEach(() => {
  reset();
  serve(shop, placed);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test('the studio is dark until the configuration says otherwise', async () => {
  const screen = await show();

  await vi.waitFor(() => expect(theme()).toBe('dark'));
  expect(barBackground(screen)).toBe(panel.dark);
  await expect.element(screen.getByRole('button', { name: 'Switch to light' })).toBeInTheDocument();
});

test('style.theme light in togen.yml opens the studio in light', async () => {
  serve(shop, placed, { ...defaults, style: { theme: 'light' } });
  const screen = await show();

  await vi.waitFor(() => expect(theme()).toBe('light'));
  expect(barBackground(screen)).toBe(panel.light);
  await expect.element(screen.getByRole('button', { name: 'Switch to dark' })).toBeInTheDocument();
});

test('system follows the OS preference as it changes', async () => {
  const setDark = prefer(false);
  serve(shop, placed, { ...defaults, style: { theme: 'system' } });
  await show();

  await vi.waitFor(() => expect(theme()).toBe('light'));

  setDark(true);
  await vi.waitFor(() => expect(theme()).toBe('dark'));
});

test('a configuration the studio cannot read leaves it dark and says why', async () => {
  refuse((call) =>
    call.path === '/api/config'
      ? invalid([{ path: 'togen.yml', message: 'not valid YAML: line 3' }])
      : undefined,
  );
  const screen = await show();

  await vi.waitFor(() => expect(theme()).toBe('dark'));
  await expect.element(screen.getByText('togen.yml: not valid YAML: line 3')).toBeInTheDocument();
});

test('the toggle overrides the configuration for the session', async () => {
  serve(shop, placed, { ...defaults, style: { theme: 'light' } });
  let screen = await show();
  await vi.waitFor(() => expect(theme()).toBe('light'));

  await screen.getByRole('button', { name: 'Switch to dark' }).click();
  await vi.waitFor(() => expect(theme()).toBe('dark'));
  expect(barBackground(screen)).toBe(panel.dark);

  socket()?.onmessage?.({ data: JSON.stringify({ event: 'config-changed' }) });
  await vi.waitFor(() => expect(gets('/api/config')).toHaveLength(2));
  expect(theme()).toBe('dark');

  await screen.unmount();
  screen = await show();
  await vi.waitFor(() => expect(theme()).toBe('dark'));
  await expect.element(screen.getByRole('button', { name: 'Switch to light' })).toBeInTheDocument();
});

test('the bar names the project, provider, region and environment', async () => {
  const screen = await show();

  await expect.element(screen.getByText('Togen', { exact: true })).toBeInTheDocument();
  await expect.element(screen.getByText('studio', { exact: true })).toBeInTheDocument();
  await expect.element(screen.getByText('shop', { exact: true })).toBeInTheDocument();
  await expect.element(screen.getByText('aws', { exact: true })).toBeInTheDocument();
  await expect.element(screen.getByText('eu-west-2', { exact: true })).toBeInTheDocument();
  await expect.element(screen.getByText('dev', { exact: true })).toBeInTheDocument();
});

test('the status pill says there are no problems, or how many', async () => {
  const screen = await show();

  await expect.element(screen.getByText('No problems')).toBeInTheDocument();
  expect(screen.container.querySelector('header button[aria-expanded]')).toBeNull();

  serve({ ...shop, nodes: [...shop.nodes, { id: 'gateway-2', type: 'gateway', name: 'admin' }] }, placed);
  socket()?.onmessage?.({ data: JSON.stringify({ event: 'project-changed' }) });

  await expect.element(screen.getByRole('button', { name: '1 problem' })).toBeInTheDocument();
  await expect.element(screen.getByText('No problems')).not.toBeInTheDocument();
});

test('the status pill carries the note about the legacy configuration file', async () => {
  const note = 'togen/togen.json is deprecated; run togen init --migrate to move it to togen.yml';
  serve(shop, placed, { ...defaults, deprecated: note });
  const screen = await show();

  await expect.element(screen.getByText(note)).toBeInTheDocument();
});

test('export and generate are available once the project is loaded', async () => {
  const screen = await show();

  await expect.element(screen.getByRole('button', { name: 'Export' })).toBeEnabled();
  await expect.element(screen.getByRole('button', { name: 'Generate' })).toBeEnabled();
});

test('the rail has the views placeholder, the palette grid and the project file', async () => {
  const screen = await show();

  const rail = screen.getByLabelText('Rail');
  await expect.element(rail.getByText('Views')).toBeInTheDocument();
  await expect.element(rail.getByText('Overview')).toBeInTheDocument();
  await expect.element(screen.getByText('Palette')).toBeInTheDocument();
  await expect.element(screen.getByText('togen/project.json')).toBeInTheDocument();

  const tiles = screen.container.querySelectorAll('[data-node-type]');
  expect(tiles).toHaveLength(7);
  const grid = tiles[0].parentElement!;
  expect(getComputedStyle(grid).gridTemplateColumns.split(' ')).toHaveLength(2);
});

test('the four regions have the widths from the design', async () => {
  const screen = await show();

  expect(screen.container.querySelector('header')!.getBoundingClientRect().height).toBe(48);
  expect(screen.container.querySelector('aside[aria-label="Rail"]')!.getBoundingClientRect().width).toBe(240);

  screen.container
    .querySelector('.svelte-flow__node[data-id="function-1"]')!
    .dispatchEvent(new MouseEvent('click', { bubbles: true }));

  const inspector = await vi.waitFor(() => {
    const found = screen.container.querySelector('aside[aria-label="Inspector"]');
    expect(found).not.toBeNull();
    return found!;
  });
  await vi.waitFor(() => expect(inspector.getBoundingClientRect().width).toBe(320));
});
