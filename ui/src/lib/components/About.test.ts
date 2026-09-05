import { afterEach, beforeEach, expect, test, vi } from 'vitest';

import { reset, serve, show } from '../../harness.ts';
import type { Layout, Project } from '../types.ts';

const shop: Project = {
  version: 1,
  name: 'shop',
  provider: 'aws',
  region: 'eu-west-2',
  environment: 'dev',
  nodes: [{ id: 'gateway-1', type: 'gateway', name: 'api' }],
  edges: [],
};

const placed: Layout = {
  version: 1,
  nodes: { 'gateway-1': { x: 20, y: 20 } },
  viewport: { x: 0, y: 0, zoom: 1 },
};

beforeEach(() => {
  reset();
  serve(shop, placed);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test('About lists the three icon sets with their terms, and closes with Escape or the button', async () => {
  const screen = await show();
  expect(screen.container.querySelector('[role="dialog"]')).toBeNull();

  await screen.getByRole('button', { name: 'About' }).click();

  const dialog = screen.getByRole('dialog', { name: 'About' });
  await expect.element(dialog).toBeInTheDocument();
  const sets = [...screen.container.querySelectorAll('[aria-label="Icon licences"] > li')];
  expect(sets.map((set) => set.querySelector('h3')?.textContent)).toEqual([
    'AWS Architecture Icons',
    'Google Cloud architecture icons',
    'Azure architecture icons',
  ]);
  const notices = sets.map((set) => set.textContent?.replace(/\s+/g, ' ') ?? '');
  expect(notices[0]).toContain('https://aws.amazon.com/architecture/icons/');
  expect(notices[1]).toContain('https://cloud.google.com/icons');
  expect(notices[2]).toContain('https://learn.microsoft.com/en-us/azure/architecture/icons/');
  const quoted = sets.map((set) =>
    [...set.querySelectorAll('blockquote')].map((quote) => quote.textContent?.replace(/\s+/g, ' ').trim()),
  );
  expect(quoted[0][0]).toContain('We allow customers and partners to use these toolkits');
  expect(quoted[1][0]).toContain('Official Google Cloud icons');
  expect(quoted[2][0]).toMatch(/^Microsoft permits the use of these icons/);
  expect(sets[0].textContent).not.toContain('#');

  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
  await expect.element(dialog).not.toBeInTheDocument();

  await screen.getByRole('button', { name: 'About' }).click();
  await expect.element(screen.getByRole('dialog', { name: 'About' })).toBeInTheDocument();
  await screen.getByRole('button', { name: 'Close about' }).click();
  await expect.element(screen.getByRole('dialog', { name: 'About' })).not.toBeInTheDocument();
});
