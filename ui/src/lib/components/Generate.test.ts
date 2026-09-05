import { afterEach, beforeEach, expect, test, vi } from 'vitest';

import { answer, calls, posts, refuse, reset, serve, settle, show } from '../../harness.ts';
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
  ],
  edges: [{ id: 'edge-1', from: 'gateway-1', to: 'function-1', relation: 'routes' }],
};

const placed: Layout = {
  version: 1,
  nodes: { 'gateway-1': { x: 20, y: 20 }, 'function-1': { x: 260, y: 20 } },
  viewport: { x: 0, y: 0, zoom: 1 },
};

const written = {
  generated: [{ dir: 'infra/hcl', files: ['main.tf', 'outputs.tf', 'variables.tf'] }],
};

type Screen = Awaited<ReturnType<typeof show>>;

function generateReplies(response: () => Response) {
  refuse((call) => (call.path === '/api/generate' ? response() : undefined));
}

function badged(screen: Screen, id: string): boolean {
  return screen.container.querySelector(`.svelte-flow__node[data-id="${id}"] .bg-red-600`) !== null;
}

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

test('generate lists the directory it wrote and every file in it', async () => {
  generateReplies(() => answer(200, written));
  const screen = await show();

  await screen.getByRole('button', { name: 'Generate' }).click();

  await expect.element(screen.getByLabelText('Generated')).toBeInTheDocument();
  await expect.element(screen.getByText('infra/hcl')).toBeInTheDocument();
  for (const file of written.generated[0].files) {
    await expect.element(screen.getByText(file)).toBeInTheDocument();
  }
  expect(posts('/api/generate')).toHaveLength(1);
  expect(screen.container.querySelector('header button[aria-expanded]')).toBeNull();

  await screen.getByRole('button', { name: 'Dismiss' }).click();
  await vi.waitFor(() =>
    expect(screen.container.querySelector('section[aria-label="Generated"]')).toBeNull(),
  );
});

test('a refused generate badges the node it names, until the next change', async () => {
  generateReplies(() =>
    answer(422, {
      errors: [
        { path: 'nodes.0', nodeId: 'gateway-1', message: 'no resolver for gateway on aws yet' },
      ],
    }),
  );
  const screen = await show();

  await screen.getByRole('button', { name: 'Generate' }).click();

  await vi.waitFor(() => expect(badged(screen, 'gateway-1')).toBe(true));
  await screen.getByRole('button', { name: '1 problem' }).click();
  await expect
    .element(screen.getByText('nodes.0 (node gateway-1): no resolver for gateway on aws yet'))
    .toBeInTheDocument();

  dropOn(screen.container.querySelector('.svelte-flow')!, 'database', { x: 400, y: 300 });

  await vi.waitFor(() => expect(badged(screen, 'gateway-1')).toBe(false));
  expect(screen.container.querySelector('header button[aria-expanded]')).toBeNull();
});

test('a directory holding files Togen did not write says how to replace it', async () => {
  generateReplies(() =>
    answer(409, {
      error:
        'infra/hcl contains files Togen did not write: notes.txt\nMove them, or run again with --force to replace the whole directory.',
    }),
  );
  const screen = await show();

  await screen.getByRole('button', { name: 'Generate' }).click();

  await expect
    .element(
      screen.getByText(
        "infra/hcl contains files Togen did not write: notes.txt. Run 'togen generate --force' in a terminal to replace the directory.",
      ),
    )
    .toBeInTheDocument();
  expect(screen.container.querySelector('section[aria-label="Generated"]')).toBeNull();
});

test('generate waits until the problems are gone', async () => {
  serve({ ...shop, nodes: [shop.nodes[0], { ...shop.nodes[1], name: 'api' }] }, placed);
  generateReplies(() => answer(200, written));
  const screen = await show();

  const generate = screen.getByRole('button', { name: 'Generate' });
  await expect.element(generate).toBeDisabled();

  screen.container
    .querySelector('.svelte-flow__node[data-id="function-1"]')!
    .dispatchEvent(new MouseEvent('click', { bubbles: true }));
  await screen.getByLabelText('Name', { exact: true }).fill('orders');

  await expect.element(generate).toBeEnabled();
});

test('a pending edit is saved before the generate it would change', async () => {
  generateReplies(() => answer(200, written));
  const screen = await show();

  screen.container
    .querySelector('.svelte-flow__node[data-id="function-1"]')!
    .dispatchEvent(new MouseEvent('click', { bubbles: true }));
  await screen.getByLabelText('Name', { exact: true }).fill('billing');
  await screen.getByRole('button', { name: 'Generate' }).click();

  await vi.waitFor(() => expect(posts('/api/generate')).toHaveLength(1));
  await settle();
  const order = calls().map((call) => `${call.method} ${call.path}`);
  expect(order.indexOf('PUT /api/project')).toBeGreaterThan(-1);
  expect(order.indexOf('PUT /api/project')).toBeLessThan(order.indexOf('POST /api/generate'));
  expect(order.filter((line) => line === 'POST /api/generate')).toHaveLength(1);
});
