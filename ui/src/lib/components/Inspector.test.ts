import { afterEach, beforeEach, expect, test, vi } from 'vitest';

import { invalid, overviewLayout, puts, refuse, reset, serve, settle, show } from '../../harness.ts';
import type { Call } from '../../harness.ts';
import type { Node, Project } from '../types.ts';

const shop: Project = {
  version: 1,
  name: 'shop',
  provider: 'aws',
  region: 'eu-west-2',
  environment: 'dev',
  nodes: [
    { id: 'gateway-1', type: 'gateway', name: 'api' },
    {
      id: 'function-1',
      type: 'function',
      name: 'orders',
      properties: { env: { LOG_LEVEL: 'info' } },
    },
    {
      id: 'database-1',
      type: 'database',
      name: 'orders-db',
      properties: { engine: 'postgres', size: 'small' },
    },
    { id: 'service-1', type: 'service', name: 'web', properties: { image: 'nginx:1.27' } },
    { id: 'queue-1', type: 'queue', name: 'jobs' },
    { id: 'bucket-1', type: 'bucket', name: 'uploads' },
    { id: 'cache-1', type: 'cache', name: 'sessions' },
    { id: 'database-2', type: 'database', name: 'events-db', properties: { version: '15' } },
  ],
  edges: [],
};

const placed = overviewLayout({
  'gateway-1': { x: 0, y: 0 },
  'function-1': { x: 200, y: 0 },
  'database-1': { x: 400, y: 0 },
  'service-1': { x: 0, y: 150 },
  'queue-1': { x: 200, y: 150 },
  'bucket-1': { x: 400, y: 150 },
  'cache-1': { x: 0, y: 300 },
  'database-2': { x: 200, y: 300 },
});

type Screen = Awaited<ReturnType<typeof show>>;

function panel(screen: Screen): Element {
  const found = screen.container.querySelector('aside[aria-label="Inspector"]');
  expect(found).not.toBeNull();
  return found!;
}

async function open(screen: Screen, id: string) {
  const card = await vi.waitFor(() => {
    const found = screen.container.querySelector(`.svelte-flow__node[data-id="${id}"]`);
    expect(found).not.toBeNull();
    return found!;
  });
  card.dispatchEvent(new MouseEvent('click', { bubbles: true }));
  await vi.waitFor(() => panel(screen));
}

// Every labelled control in the panel, by its label, so one assertion says which
// fields a node type offers and what each one is.
function controls(screen: Screen): Record<string, string> {
  const out: Record<string, string> = {};
  for (const element of panel(screen).querySelectorAll('input, select')) {
    out[named(element)] = kindOf(element);
  }
  return out;
}

function named(element: Element): string {
  const aria = element.getAttribute('aria-label');
  if (aria !== null) {
    return aria;
  }
  const label =
    element.closest('label') ?? element.ownerDocument.querySelector(`label[for="${element.id}"]`);
  return label?.textContent?.trim() ?? element.id;
}

function kindOf(element: Element): string {
  if (element.tagName === 'SELECT') {
    return 'select';
  }
  const type = (element as HTMLInputElement).type;
  return type === 'checkbox' ? 'toggle' : type;
}

function control(screen: Screen, label: string): HTMLInputElement {
  return screen.getByLabelText(label, { exact: true }).element() as HTMLInputElement;
}

function choice(screen: Screen, label: string): HTMLSelectElement {
  return screen.getByLabelText(label, { exact: true }).element() as HTMLSelectElement;
}

function options(screen: Screen, label: string): string[] {
  return [...choice(screen, label).options].map((option) => option.value);
}

function saved(call: Call, id: string): Node {
  const node = (call.body as Project).nodes.find((candidate) => candidate.id === id);
  expect(node).toBeDefined();
  return node!;
}

function ringed(screen: Screen, id: string): boolean {
  return screen.container.querySelector(`.svelte-flow__node[data-id="${id}"] .ring-1`) !== null;
}

function reason(screen: Screen): string | null {
  return panel(screen).querySelector('.text-err')?.textContent ?? null;
}

beforeEach(() => {
  reset();
  serve(shop, placed);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test('a function offers its runtime, handler, size, timeout and env', async () => {
  const screen = await show();
  await open(screen, 'function-1');

  expect(controls(screen)).toEqual({
    Name: 'text',
    'Env key 1': 'text',
    'Env value 1': 'text',
    Handler: 'text',
    Runtime: 'select',
    Size: 'select',
    'Timeout seconds': 'number',
  });
  expect(options(screen, 'Runtime')).toEqual(['', 'node', 'python', 'go']);
  expect(control(screen, 'Handler').placeholder).toBe('index.handler');
  expect(control(screen, 'Timeout seconds').max).toBe('900');
  expect(control(screen, 'Env key 1').value).toBe('LOG_LEVEL');
  await expect
    .element(screen.getByText('Seconds a single invocation may run for'))
    .toBeInTheDocument();
  await expect.element(screen.getByText('Add variable')).toBeInTheDocument();
});

test('a database offers its engine, versions, size, storage and standby', async () => {
  const screen = await show();
  await open(screen, 'database-1');

  expect(controls(screen)).toEqual({
    Name: 'text',
    Engine: 'select',
    'High availability': 'toggle',
    Size: 'select',
    'Storage gb': 'number',
    Version: 'select',
  });
  expect(options(screen, 'Version')).toEqual(['', '17', '16', '15']);
  expect(choice(screen, 'Version').options[0].textContent).toBe('newest, 17');
  expect(control(screen, 'Storage gb').min).toBe('20');
  expect(control(screen, 'Storage gb').value).toBe('');
  expect(control(screen, 'Storage gb').placeholder).toBe('20');
});

test('a service offers its image, port, size, replicas, reach and env', async () => {
  const screen = await show();
  await open(screen, 'service-1');

  expect(controls(screen)).toEqual({
    Name: 'text',
    Image: 'text',
    'Max replicas': 'number',
    'Min replicas': 'number',
    Port: 'number',
    Public: 'toggle',
    Size: 'select',
  });
  expect(control(screen, 'Image').required).toBe(true);
  expect(control(screen, 'Image').value).toBe('nginx:1.27');
  expect(control(screen, 'Port').placeholder).toBe('8080');
  await expect.element(screen.getByText('required')).toBeInTheDocument();
  await expect.element(screen.getByText('Add variable')).toBeInTheDocument();
});

test('a queue offers ordering, a dead letter queue and retention', async () => {
  const screen = await show();
  await open(screen, 'queue-1');

  expect(controls(screen)).toEqual({
    Name: 'text',
    'Dead letter': 'toggle',
    Fifo: 'toggle',
    'Retention days': 'number',
  });
  expect(control(screen, 'Retention days').min).toBe('1');
  expect(control(screen, 'Retention days').max).toBe('14');
  expect(control(screen, 'Retention days').placeholder).toBe('4');
  expect(control(screen, 'Dead letter').checked).toBe(true);
  expect(control(screen, 'Fifo').checked).toBe(false);
});

test('a bucket offers versioning and reach', async () => {
  const screen = await show();
  await open(screen, 'bucket-1');

  expect(controls(screen)).toEqual({ Name: 'text', Public: 'toggle', Versioning: 'toggle' });
});

test('a cache offers its size', async () => {
  const screen = await show();
  await open(screen, 'cache-1');

  expect(controls(screen)).toEqual({ Name: 'text', Size: 'select' });
  expect(options(screen, 'Size')).toEqual(['', 'small', 'medium', 'large']);
  expect(choice(screen, 'Size').options[0].textContent).toBe('small (default)');
});

test('a gateway has nothing to set but its name', async () => {
  const screen = await show();
  await open(screen, 'gateway-1');

  expect(controls(screen)).toEqual({ Name: 'text' });
  expect(panel(screen).querySelector('header span')?.textContent).toBe('gateway');
});

test('a toggle saves the project once after the debounce', async () => {
  const screen = await show();
  await open(screen, 'queue-1');

  await screen.getByLabelText('Fifo', { exact: true }).click();
  expect(puts('/api/project')).toHaveLength(0);

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  await settle();
  expect(puts('/api/project')).toHaveLength(1);
  expect(saved(puts('/api/project')[0], 'queue-1').properties).toEqual({ fifo: true });
});

test('clearing a field drops the key rather than writing the default', async () => {
  const screen = await show();
  await open(screen, 'database-1');

  await screen.getByLabelText('Engine', { exact: true }).selectOptions('');

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  expect(saved(puts('/api/project')[0], 'database-1').properties).toEqual({ size: 'small' });
});

test('switching the engine offers that engine versions and drops one it does not support', async () => {
  const screen = await show();
  await open(screen, 'database-2');

  await screen.getByLabelText('Engine', { exact: true }).selectOptions('mysql');

  await vi.waitFor(() => expect(options(screen, 'Version')).toEqual(['', '8.4', '8.0']));
  expect(choice(screen, 'Version').options[0].textContent).toBe('newest, 8.4');

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  expect(saved(puts('/api/project')[0], 'database-2').properties).toEqual({ engine: 'mysql' });
});

test('an env row is saved', async () => {
  const screen = await show();
  await open(screen, 'function-1');

  await screen.getByText('Add variable').click();
  await screen.getByLabelText('Env key 2', { exact: true }).fill('API_URL');
  await screen.getByLabelText('Env value 2', { exact: true }).fill('https://orders');

  await vi.waitFor(() => expect(puts('/api/project').length).toBeGreaterThan(0));
  await settle();
  expect(saved(puts('/api/project').at(-1)!, 'function-1').properties).toEqual({
    env: { LOG_LEVEL: 'info', API_URL: 'https://orders' },
  });
});

test('an env key the schema refuses says why and is not saved', async () => {
  const screen = await show();
  await open(screen, 'function-1');

  await screen.getByText('Add variable').click();
  await screen.getByLabelText('Env key 2', { exact: true }).fill('api url');

  await expect.element(screen.getByText(/api url is not a valid name/)).toBeInTheDocument();
  await vi.waitFor(() => expect(puts('/api/project').length).toBeGreaterThan(0));
  await settle();
  expect(saved(puts('/api/project').at(-1)!, 'function-1').properties).toEqual({
    env: { LOG_LEVEL: 'info' },
  });
});

test('renaming a node keeps its id and the card follows', async () => {
  const screen = await show();
  await open(screen, 'service-1');

  await screen.getByLabelText('Name', { exact: true }).fill('web-app');

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  const node = saved(puts('/api/project')[0], 'service-1');
  expect(node.name).toBe('web-app');
  expect(node.id).toBe('service-1');
  await expect.element(screen.getByText('web-app')).toBeInTheDocument();
});

test('a name that is still being typed is not sent', async () => {
  const screen = await show();
  await open(screen, 'service-1');

  await screen.getByLabelText('Name', { exact: true }).fill('web-');
  await settle();
  expect(puts('/api/project')).toHaveLength(0);

  await screen.getByLabelText('Name', { exact: true }).fill('web-app');

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  expect(saved(puts('/api/project')[0], 'service-1').name).toBe('web-app');
});

test('an empty required field says why and is not saved', async () => {
  const screen = await show();
  await open(screen, 'service-1');

  await screen.getByLabelText('Image', { exact: true }).fill('');

  await settle();
  expect(puts('/api/project')).toHaveLength(0);
  expect(reason(screen)).toBe('required');
});

test('a number outside its range says why, then saves once it is back in range', async () => {
  const screen = await show();
  await open(screen, 'database-1');

  await screen.getByLabelText('Storage gb', { exact: true }).fill('5');
  await settle();
  expect(puts('/api/project')).toHaveLength(0);
  expect(reason(screen)).toBe('at least 20');

  await screen.getByLabelText('Storage gb', { exact: true }).fill('50');

  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  expect(saved(puts('/api/project')[0], 'database-1').properties).toEqual({
    engine: 'postgres',
    size: 'small',
    storageGb: 50,
  });
});

test('a name failing the kebab-case pattern says why and is not saved', async () => {
  const screen = await show();
  await open(screen, 'service-1');

  await screen.getByLabelText('Name', { exact: true }).fill('Bad Name');

  await settle();
  expect(puts('/api/project')).toHaveLength(0);
  expect(reason(screen)).toBe('lowercase letters, digits and hyphens, starting with a letter');
});

test('a refused change puts the field back and the bar says why', async () => {
  refuse((call) =>
    call.method === 'PUT' && call.path === '/api/project'
      ? invalid([
          {
            path: 'nodes.2.properties.size',
            nodeId: 'database-1',
            message: "size 'large' is not supported",
          },
        ])
      : undefined,
  );
  const screen = await show();
  await open(screen, 'database-1');

  await screen.getByLabelText('Size', { exact: true }).selectOptions('large');
  expect(choice(screen, 'Size').value).toBe('large');

  const line = "nodes.2.properties.size (node database-1): size 'large' is not supported";
  await expect.element(screen.getByText(line)).toBeInTheDocument();
  await vi.waitFor(() => expect(choice(screen, 'Size').value).toBe('small'));
});

test('escape closes the inspector, and so does clicking the canvas', async () => {
  const screen = await show();
  await open(screen, 'cache-1');

  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));

  await vi.waitFor(() =>
    expect(screen.container.querySelector('aside[aria-label="Inspector"]')).toBeNull(),
  );

  await open(screen, 'cache-1');
  screen.container
    .querySelector('.svelte-flow__pane')!
    .dispatchEvent(new MouseEvent('click', { bubbles: true }));

  await vi.waitFor(() =>
    expect(screen.container.querySelector('aside[aria-label="Inspector"]')).toBeNull(),
  );
});

test('the selection ring survives an edit and clears when the inspector closes', async () => {
  const screen = await show();
  await open(screen, 'queue-1');
  expect(ringed(screen, 'queue-1')).toBe(true);

  await screen.getByLabelText('Fifo', { exact: true }).click();
  await vi.waitFor(() => expect(puts('/api/project')).toHaveLength(1));
  expect(ringed(screen, 'queue-1')).toBe(true);

  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  await vi.waitFor(() => expect(ringed(screen, 'queue-1')).toBe(false));
});
