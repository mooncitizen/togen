import { afterEach, beforeEach, expect, test, vi } from 'vitest';

import {
  answer,
  gets,
  invalid,
  overviewLayout,
  posts,
  refuse,
  reset,
  serve,
  serveEmpty,
  showFirstRun,
  socket,
} from './harness.ts';
import type { Project } from './lib/types.ts';

const shop: Project = {
  version: 1,
  name: 'shop',
  provider: 'aws',
  region: 'eu-west-2',
  environment: 'dev',
  nodes: [{ id: 'gateway-1', type: 'gateway', name: 'api' }],
  edges: [],
};

const placed = overviewLayout({ 'gateway-1': { x: 20, y: 20 } });

const kebab = 'lowercase letters, digits and hyphens, starting with a letter';

type Screen = Awaited<ReturnType<typeof showFirstRun>>;

function canvas(screen: Screen): Element | null {
  return screen.container.querySelector('.svelte-flow');
}

function regionSelect(screen: Screen): HTMLSelectElement {
  return screen.container.querySelector<HTMLSelectElement>('#sketch-region')!;
}

function regionIds(screen: Screen): string[] {
  return [...regionSelect(screen).options].map((option) => option.value);
}

function swatch(screen: Screen, provider: string, kind: string): string {
  const found = screen.container.querySelector(
    `[data-provider="${provider}"] [data-kind="${kind}"]`,
  )!;
  return getComputedStyle(found).backgroundColor;
}

async function openSketch(screen: Screen) {
  await screen.getByRole('button', { name: 'New sketch' }).click();
  await expect.element(screen.getByRole('form', { name: 'New sketch' })).toBeInTheDocument();
}

async function openExamples(screen: Screen) {
  await screen.getByRole('button', { name: 'Start from an example' }).click();
  await expect.element(screen.getByRole('button', { name: 'Use aws-full' })).toBeInTheDocument();
}

beforeEach(() => {
  reset();
  serveEmpty();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test('a directory with no project opens on the first-run screen, named, with three choices', async () => {
  const screen = await showFirstRun();

  await expect.element(screen.getByText('No project in shop yet.')).toBeInTheDocument();
  await expect.element(screen.getByText('What do you want to do?')).toBeInTheDocument();
  expect(screen.container.querySelector('header')!.textContent).toContain('shop');
  await expect.element(screen.getByText('Togen', { exact: true })).toBeInTheDocument();
  await expect.element(screen.getByText('studio', { exact: true })).toBeInTheDocument();
  await expect.element(screen.getByRole('button', { name: 'New sketch' })).toBeEnabled();
  await expect.element(screen.getByRole('button', { name: 'Start from an example' })).toBeEnabled();
  await expect.element(screen.getByRole('button', { name: 'Import existing Terraform' })).toBeDisabled();
  await expect.element(screen.getByText('Coming later')).toBeInTheDocument();
  expect(gets('/api/workspace')).toHaveLength(1);
  expect(canvas(screen)).toBeNull();
});

test('the first-run screen is dark by default and the toggle works there too', async () => {
  const screen = await showFirstRun();

  await vi.waitFor(() => expect(document.documentElement.dataset.theme).toBe('dark'));
  await screen.getByRole('button', { name: 'Switch to light' }).click();
  await vi.waitFor(() => expect(document.documentElement.dataset.theme).toBe('light'));
});

test('import is drawn but does nothing', async () => {
  const screen = await showFirstRun();
  const disabled = screen.getByRole('button', { name: 'Import existing Terraform' });

  await expect.element(disabled).toBeDisabled();
  await expect.element(screen.getByText('What do you want to do?')).toBeInTheDocument();
  expect(posts('/api/project/init')).toHaveLength(0);
});

test('new sketch suggests the directory name, defaults to dev and aws, and refuses a bad name before posting', async () => {
  const screen = await showFirstRun();
  await openSketch(screen);

  await expect.element(screen.getByLabelText('Project name')).toHaveValue('shop');
  await expect.element(screen.getByLabelText('Environment')).toHaveValue('dev');
  await expect.element(screen.getByRole('radio', { name: 'AWS' })).toBeChecked();
  expect(regionSelect(screen).value).toBe('eu-west-2');

  await screen.getByLabelText('Project name').fill('Bad Name');
  await screen.getByRole('button', { name: 'Create sketch' }).click();

  await expect.element(screen.getByText(kebab)).toBeInTheDocument();
  expect(posts('/api/project/init')).toHaveLength(0);
  expect(canvas(screen)).toBeNull();
});

test('the provider cards carry their palettes and the region list follows the choice', async () => {
  const screen = await showFirstRun();
  await openSketch(screen);

  expect(screen.container.querySelectorAll('[data-provider="aws"] [data-kind]')).toHaveLength(7);
  expect(swatch(screen, 'aws', 'database')).toBe('rgb(201, 37, 209)');
  expect(swatch(screen, 'gcp', 'database')).toBe('rgb(66, 133, 244)');
  expect(swatch(screen, 'azure', 'queue')).toBe('rgb(255, 185, 0)');

  await screen.getByRole('radio', { name: 'GCP' }).click();

  await expect.element(screen.getByRole('radio', { name: 'GCP' })).toBeChecked();
  await expect.element(screen.getByRole('radio', { name: 'AWS' })).not.toBeChecked();
  await vi.waitFor(() => expect(regionSelect(screen).value).toBe('europe-west2'));
  expect(regionIds(screen)).toContain('europe-west1');
  expect(regionIds(screen)).not.toContain('eu-west-2');

  await screen.getByRole('radio', { name: 'Azure' }).click();
  await vi.waitFor(() => expect(regionSelect(screen).value).toBe('uksouth'));
});

test('a sound sketch is posted as the studio expects and the empty canvas opens', async () => {
  const screen = await showFirstRun();
  await openSketch(screen);

  await screen.getByLabelText('Project name').fill('market');
  await screen.getByLabelText('Environment').fill('staging');
  await screen.getByRole('radio', { name: 'GCP' }).click();
  await screen.getByLabelText('Region').selectOptions('europe-west1');
  await screen.getByRole('button', { name: 'Create sketch' }).click();

  await vi.waitFor(() => expect(posts('/api/project/init')).toHaveLength(1));
  expect(posts('/api/project/init')[0].body).toEqual({
    name: 'market',
    environment: 'staging',
    provider: 'gcp',
    region: 'europe-west1',
  });

  await vi.waitFor(() => expect(canvas(screen)).not.toBeNull());
  await expect.element(screen.getByText('market', { exact: true })).toBeInTheDocument();
  await expect.element(screen.getByText('gcp', { exact: true })).toBeInTheDocument();
  await expect.element(screen.getByText('europe-west1', { exact: true })).toBeInTheDocument();
  await expect.element(screen.getByText('staging', { exact: true })).toBeInTheDocument();
  expect(screen.container.querySelectorAll('.svelte-flow__node')).toHaveLength(0);
});

test('the example screen lists the bundled examples and posts the chosen one', async () => {
  const screen = await showFirstRun();
  await openExamples(screen);

  expect(gets('/api/examples')).toHaveLength(1);
  await expect.element(screen.getByText('aws-basic')).toBeInTheDocument();
  await expect
    .element(screen.getByText('One of every node type, wired up with the defaults'))
    .toBeInTheDocument();
  await expect.element(screen.getByText('aws-full')).toBeInTheDocument();

  await screen.getByRole('button', { name: 'Use aws-full' }).click();

  await vi.waitFor(() => expect(posts('/api/project/init')).toHaveLength(1));
  expect(posts('/api/project/init')[0].body).toEqual({ example: 'aws-full' });
  await vi.waitFor(() => expect(canvas(screen)).not.toBeNull());
  await expect.element(screen.getByText('aws-full', { exact: true })).toBeInTheDocument();
  await expect.element(screen.getByText('api', { exact: true })).toBeInTheDocument();
});

test('back returns to the choices from either screen', async () => {
  const screen = await showFirstRun();

  await openSketch(screen);
  await screen.getByRole('button', { name: 'Back' }).click();
  await expect.element(screen.getByText('What do you want to do?')).toBeInTheDocument();

  await openExamples(screen);
  await screen.getByRole('button', { name: 'Back' }).click();
  await expect.element(screen.getByText('What do you want to do?')).toBeInTheDocument();
});

test('a refusal from the studio lands on the field it names', async () => {
  refuse((call) =>
    call.path === '/api/project/init'
      ? invalid([
          { path: 'name', message: "'shop' is not allowed here" },
          { path: 'region', message: "'eu-west-2' is not a region id such as eu-west-2" },
          { path: '', message: 'the sketch as a whole was refused' },
        ])
      : undefined,
  );
  const screen = await showFirstRun();
  await openSketch(screen);

  await screen.getByRole('button', { name: 'Create sketch' }).click();

  await expect.element(screen.getByText("'shop' is not allowed here")).toBeInTheDocument();
  await expect
    .element(screen.getByText("'eu-west-2' is not a region id such as eu-west-2"))
    .toBeInTheDocument();
  await expect
    .element(screen.getByText('project: the sketch as a whole was refused'))
    .toBeInTheDocument();
  expect(posts('/api/project/init')).toHaveLength(1);
  expect(canvas(screen)).toBeNull();
  await expect.element(screen.getByRole('button', { name: 'Create sketch' })).toBeEnabled();
});

test('a refused example says why and stays on the list', async () => {
  refuse((call) =>
    call.path === '/api/project/init'
      ? invalid([{ path: 'example', message: "unknown example 'aws-full'" }])
      : undefined,
  );
  const screen = await showFirstRun();
  await openExamples(screen);

  await screen.getByRole('button', { name: 'Use aws-full' }).click();

  await expect
    .element(screen.getByText("example: unknown example 'aws-full'"))
    .toBeInTheDocument();
  expect(canvas(screen)).toBeNull();
});

test('a project that appeared meanwhile is offered to open', async () => {
  const screen = await showFirstRun();
  await openSketch(screen);

  serve(shop, placed);
  refuse((call) =>
    call.path === '/api/project/init'
      ? answer(409, { error: 'togen/ already exists in /home/paul/code/shop' })
      : undefined,
  );
  await screen.getByRole('button', { name: 'Create sketch' }).click();

  await expect
    .element(screen.getByText('A project now exists in this directory.'))
    .toBeInTheDocument();
  await expect
    .element(screen.getByText('togen/ already exists in /home/paul/code/shop'))
    .toBeInTheDocument();
  expect(canvas(screen)).toBeNull();

  await screen.getByRole('button', { name: 'Open it' }).click();

  await vi.waitFor(() => expect(canvas(screen)).not.toBeNull());
  await expect.element(screen.getByText('shop', { exact: true })).toBeInTheDocument();
});

test('togen init in a terminal opens the canvas through the file event', async () => {
  const screen = await showFirstRun();

  serve(shop, placed);
  socket()?.onmessage?.({ data: JSON.stringify({ event: 'project-changed' }) });

  await vi.waitFor(() => expect(canvas(screen)).not.toBeNull());
  await expect.element(screen.getByText('shop', { exact: true })).toBeInTheDocument();
  expect(screen.container.querySelector('[data-screen="first-run"]')).toBeNull();
});
