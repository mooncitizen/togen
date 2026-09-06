import { afterEach, beforeEach, expect, test, vi } from 'vitest';

import { overviewLayout, puts, reset, serve, settle, show, socket } from '../../harness.ts';
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
  ],
  edges: [{ id: 'edge-1', from: 'gateway-1', to: 'function-1', relation: 'routes' }],
};

const placed = overviewLayout({ 'gateway-1': { x: 20, y: 20 }, 'function-1': { x: 260, y: 20 } });

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

function press(on: Element | Document, key: string, held: { shift?: boolean } = {}) {
  on.dispatchEvent(
    new KeyboardEvent('keydown', {
      key,
      metaKey: true,
      shiftKey: held.shift === true,
      bubbles: true,
      cancelable: true,
    }),
  );
}

function named(store: Store, id: string): string | undefined {
  return store.project?.nodes.find((node) => node.id === id)?.name;
}

function ids(store: Store): string[] {
  return (store.project?.nodes ?? []).map((node) => node.id);
}

beforeEach(() => {
  reset();
  serve(shop, placed);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test('undo takes an added node away and redo puts it back, saving each time', async () => {
  const screen = await show();

  dropOn(screen.container.querySelector('.svelte-flow')!, 'database', { x: 400, y: 260 });
  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  await expect.element(screen.getByText('database-1')).toBeInTheDocument();

  await screen.getByRole('button', { name: 'Undo' }).click();

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(2));
  expect((puts('/api/project')[1].body as Project).nodes.map((node) => node.id)).toEqual([
    'gateway-1',
    'function-1',
  ]);
  await expect.element(screen.getByText('database-1')).not.toBeInTheDocument();

  await screen.getByRole('button', { name: 'Redo' }).click();

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(3));
  expect((puts('/api/project')[2].body as Project).nodes.map((node) => node.id)).toContain(
    'database-1',
  );
  await expect.element(screen.getByText('database-1')).toBeInTheDocument();
});

test('undoing a delete puts the node back where it was, edges and all', async () => {
  const store = new Store();
  await store.load();

  await store.deleteNode('function-1');
  expect(store.positions['function-1']).toBeUndefined();

  await store.undo();

  expect(ids(store)).toEqual(['gateway-1', 'function-1']);
  expect(store.project?.edges).toHaveLength(1);
  expect(store.positions['function-1']).toEqual({ x: 260, y: 20 });
  await vi.waitFor(() => {
    const layout = puts('/api/layout').at(-1)?.body as Layout;
    expect(layout.views.overview.nodes['function-1']).toEqual({ x: 260, y: 20 });
  });
});

test('a burst of keystrokes on one name is a single undo', async () => {
  const store = new Store();
  await store.load();

  store.updateNode('function-1', { name: 'ord' });
  store.updateNode('function-1', { name: 'orde' });
  store.updateNode('function-1', { name: 'order' });
  expect(named(store, 'function-1')).toBe('order');

  await store.undo();

  expect(named(store, 'function-1')).toBe('orders');
  expect(store.canUndo).toBe(false);
  await settle();
});

test('a change after an undo drops what could have been redone', async () => {
  const store = new Store();
  await store.load();

  await store.addNode('database', { x: 0, y: 400 });
  await store.undo();
  expect(store.canRedo).toBe(true);

  await store.addNode('cache', { x: 0, y: 500 });

  expect(store.canRedo).toBe(false);
  expect(ids(store)).toContain('cache-1');
});

test('an edit made outside the studio starts the history again', async () => {
  const screen = await show();

  dropOn(screen.container.querySelector('.svelte-flow')!, 'database', { x: 400, y: 260 });
  await expect.element(screen.getByRole('button', { name: 'Undo' })).toBeEnabled();

  serve({ ...shop, name: 'market' }, placed);
  socket()?.onmessage?.({ data: JSON.stringify({ event: 'project-changed' }) });

  await expect.element(screen.getByText('market')).toBeInTheDocument();
  await expect.element(screen.getByRole('button', { name: 'Undo' })).toBeDisabled();
  await expect.element(screen.getByRole('button', { name: 'Redo' })).toBeDisabled();
});

test('the undo shortcut leaves a text field to the browser', async () => {
  const screen = await show();

  dropOn(screen.container.querySelector('.svelte-flow')!, 'database', { x: 400, y: 260 });
  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  screen.container
    .querySelector('.svelte-flow__node[data-id="function-1"]')!
    .dispatchEvent(new MouseEvent('click', { bubbles: true }));
  const field = await vi.waitFor(() => {
    const found = screen.container.querySelector('#function-1-name');
    expect(found).not.toBeNull();
    return found!;
  });

  await settle();
  const saves = puts('/api/project').length;

  press(field, 'z');
  await settle();

  expect(puts('/api/project')).toHaveLength(saves);
  await expect.element(screen.getByText('database-1')).toBeInTheDocument();

  press(document.body, 'z');

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(saves + 1));
  await expect.element(screen.getByText('database-1')).not.toBeInTheDocument();

  press(document.body, 'z', { shift: true });

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(saves + 2));
  await expect.element(screen.getByText('database-1')).toBeInTheDocument();
});
