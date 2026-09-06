import { getFontEmbedCSS } from 'html-to-image';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';

import { answer, refuse, reset, serve, show } from '../../harness.ts';
import type { Layout, Project, Views } from '../types.ts';

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
  ],
  edges: [
    { id: 'edge-1', from: 'gateway-1', to: 'function-1', relation: 'routes' },
    { id: 'edge-2', from: 'function-1', to: 'database-1', relation: 'reads' },
  ],
};

const views: Views = {
  version: 1,
  views: [
    { id: 'overview', name: 'Overview', nodes: '*' },
    { id: 'orders', name: 'Orders path', nodes: ['gateway-1', 'function-1'] },
  ],
};

const still = { x: 0, y: 0, zoom: 1 };

// Three cards in a row, 0 to 640 by 0 to 56, with the VPC round the function
// and the database padded out to 216 to 664 by -36 to 80. A 32px margin and a
// 32px title band make the overview 728 by 212 at 1x, and its content starts
// at (32, 100). The orders view keeps the VPC round the function alone, out to
// 424, and is 488 by 212.
const placed: Layout = {
  version: 2,
  views: {
    overview: {
      nodes: {
        'gateway-1': { x: 0, y: 0 },
        'function-1': { x: 240, y: 0 },
        'database-1': { x: 480, y: 0 },
      },
      viewport: still,
    },
    orders: {
      nodes: { 'gateway-1': { x: 0, y: 0 }, 'function-1': { x: 240, y: 0 } },
      viewport: still,
    },
  },
};

const darkCanvas = [15, 18, 22, 255];
const white = [255, 255, 255, 255];
const clear = [0, 0, 0, 0];
const content = { x: 32, y: 100 };
const band = { x: 16, y: 8, width: 120, height: 22 };

type Screen = Awaited<ReturnType<typeof show>>;
type Saved = { blob: Blob; name: string };
type Picture = { width: number; height: number; at: (x: number, y: number) => number[] };

let saved: Saved[] = [];
let handed: Blob | null = null;

// The browser's save is an anchor click on an object URL; both are caught so
// the test can read the file instead of the download directory.
function catchDownloads() {
  vi.spyOn(URL, 'createObjectURL').mockImplementation((object) => {
    handed = object as Blob;
    return 'blob:togen/export';
  });
  vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined);
  const click = HTMLElement.prototype.click;
  vi.spyOn(HTMLElement.prototype, 'click').mockImplementation(function (this: HTMLElement) {
    if (this instanceof HTMLAnchorElement && this.download !== '') {
      saved.push({ blob: handed!, name: this.download });
      return;
    }
    click.call(this);
  });
}

async function open(screen: Screen) {
  await screen.getByRole('button', { name: 'Export', exact: true }).click();
  await expect.element(screen.getByRole('dialog')).toBeVisible();
}

function radio(screen: Screen, name: string) {
  return screen.getByRole('radio', { name, exact: true });
}

function size(screen: Screen) {
  return screen.getByRole('dialog').getByRole('status');
}

function dialog(screen: Screen): Element | null {
  return screen.container.querySelector('[role="dialog"]');
}

async function exported(screen: Screen, label = 'Export PNG'): Promise<Saved> {
  const before = saved.length;
  await screen.getByRole('button', { name: label }).click();
  await vi.waitFor(() => expect(saved).toHaveLength(before + 1), { timeout: 15000 });
  return saved[before];
}

async function decode(blob: Blob): Promise<Picture> {
  const bitmap = await createImageBitmap(blob);
  const canvas = document.createElement('canvas');
  canvas.width = bitmap.width;
  canvas.height = bitmap.height;
  const context = canvas.getContext('2d')!;
  context.drawImage(bitmap, 0, 0);
  return {
    width: bitmap.width,
    height: bitmap.height,
    at: (x, y) => [...context.getImageData(x, y, 1, 1).data],
  };
}

function inked(picture: Picture, box: typeof band, ground: number[]): number {
  let count = 0;
  for (let y = box.y; y < box.y + box.height; y += 1) {
    for (let x = box.x; x < box.x + box.width; x += 1) {
      if (picture.at(x, y).some((channel, index) => channel !== ground[index])) {
        count += 1;
      }
    }
  }
  return count;
}

function dark(picture: Picture, box: typeof band): number {
  let count = 0;
  for (let y = box.y; y < box.y + box.height; y += 1) {
    for (let x = box.x; x < box.x + box.width; x += 1) {
      const [r, g, b, a] = picture.at(x, y);
      if (a === 255 && r + g + b < 300) {
        count += 1;
      }
    }
  }
  return count;
}

beforeEach(async () => {
  reset();
  saved = [];
  handed = null;
  serve(shop, placed, undefined, views);
  await page.viewport(1200, 700);
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

test('Export waits for a project', async () => {
  refuse((call) => (call.path === '/api/project' ? answer(500, { message: 'gone' }) : undefined));
  const screen = await show();

  await expect.element(screen.getByRole('button', { name: 'Export', exact: true })).toBeDisabled();
});

test('the dialog opens for the current view with PNG, 2x, the theme ground and the title on', async () => {
  const screen = await show();
  await open(screen);

  const opened = screen.getByRole('dialog', { name: 'Export Overview' });
  await expect.element(opened).toBeVisible();
  await expect
    .element(opened.getByText('The view as drawn, boundary and labels included.'))
    .toBeVisible();
  for (const name of ['PNG', '2x', 'Theme']) {
    await expect.element(radio(screen, name)).toHaveAttribute('aria-checked', 'true');
  }
  for (const name of ['JPEG', '1x', '3x', 'Transparent', 'White']) {
    await expect.element(radio(screen, name)).toHaveAttribute('aria-checked', 'false');
    await expect.element(radio(screen, name)).toBeEnabled();
  }
  await expect
    .element(screen.getByRole('switch', { name: 'Title' }))
    .toHaveAttribute('aria-checked', 'true');
  await expect.element(size(screen)).toHaveTextContent('1456 × 424 px');
  await expect.element(screen.getByRole('button', { name: 'Export PNG' })).toBeEnabled();
  expect(dialog(screen)!.querySelectorAll('[data-preview-node]')).toHaveLength(3);
  expect(dialog(screen)!.querySelector('[role="img"]')?.textContent).toContain('Overview');
});

test('the pixel size follows the scale and the title band', async () => {
  const screen = await show();
  await open(screen);

  await radio(screen, '3x').click();
  await expect.element(size(screen)).toHaveTextContent('2184 × 636 px');

  await radio(screen, '1x').click();
  await expect.element(size(screen)).toHaveTextContent('728 × 212 px');

  await screen.getByRole('switch', { name: 'Title' }).click();
  await expect.element(size(screen)).toHaveTextContent('728 × 180 px');
  expect(dialog(screen)!.querySelector('[role="img"]')?.textContent).not.toContain('Overview');
});

test('a PNG export is the stated size, named after the project and view, with the view drawn where the frame puts it', async () => {
  catchDownloads();
  const screen = await show();
  await open(screen);
  await radio(screen, '1x').click();
  await expect.element(size(screen)).toHaveTextContent('728 × 212 px');

  const file = await exported(screen);

  expect(file.name).toBe('shop-overview.png');
  expect(file.blob.type).toBe('image/png');
  const picture = await decode(file.blob);
  expect([picture.width, picture.height]).toEqual([728, 212]);
  expect(picture.at(2, 2)).toEqual(darkCanvas);
  expect(inked(picture, band, darkCanvas)).toBeGreaterThan(20);
  // The api card's icon square, 12px into the card and 28px down.
  expect(picture.at(content.x + 26, content.y + 28)).not.toEqual(darkCanvas);
  // The VPC's top edge runs along the top of the frame, under the title band.
  expect(inked(picture, { x: 260, y: 62, width: 200, height: 5 }, darkCanvas)).toBeGreaterThan(20);
  await vi.waitFor(() => expect(dialog(screen)).toBeNull());
}, 30000);

test('the title can be left off, and transparent and white grounds do as they say', async () => {
  catchDownloads();
  const screen = await show();
  await open(screen);
  await radio(screen, '1x').click();
  await radio(screen, 'Transparent').click();
  await screen.getByRole('switch', { name: 'Title' }).click();

  const bare = await decode((await exported(screen)).blob);
  expect([bare.width, bare.height]).toEqual([728, 180]);
  expect(bare.at(2, 2)).toEqual(clear);
  expect(inked(bare, band, clear)).toBe(0);
  expect(bare.at(content.x + 26, content.y - 32 + 28)).not.toEqual(clear);

  await open(screen);
  await radio(screen, '1x').click();
  await radio(screen, 'White').click();
  const light = await decode((await exported(screen)).blob);
  expect(light.at(2, 2)).toEqual(white);
  expect(dark(light, band)).toBeGreaterThan(10);
}, 30000);

test('JPEG cannot be transparent and saves a jpg', async () => {
  catchDownloads();
  const screen = await show();
  await open(screen);
  await radio(screen, 'Transparent').click();
  await expect.element(radio(screen, 'Transparent')).toHaveAttribute('aria-checked', 'true');

  await radio(screen, 'JPEG').click();

  await expect.element(radio(screen, 'Transparent')).toBeDisabled();
  await expect.element(radio(screen, 'Transparent')).toHaveAttribute('aria-checked', 'false');
  await expect.element(radio(screen, 'Theme')).toHaveAttribute('aria-checked', 'true');
  await radio(screen, '1x').click();

  const file = await exported(screen, 'Export JPEG');

  expect(file.name).toBe('shop-overview.jpg');
  expect(file.blob.type).toBe('image/jpeg');
  const picture = await decode(file.blob);
  expect([picture.width, picture.height]).toEqual([728, 212]);
}, 30000);

test('the export is the open view', async () => {
  catchDownloads();
  const screen = await show();
  await screen.getByRole('button', { name: 'Orders path 2', exact: true }).click();
  await vi.waitFor(() =>
    expect(screen.container.querySelectorAll('.svelte-flow__node')).toHaveLength(2),
  );
  await open(screen);

  await expect.element(screen.getByRole('dialog', { name: 'Export Orders path' })).toBeVisible();
  expect(dialog(screen)!.querySelectorAll('[data-preview-node]')).toHaveLength(2);
  await radio(screen, '1x').click();
  await expect.element(size(screen)).toHaveTextContent('488 × 212 px');

  const file = await exported(screen);

  expect(file.name).toBe('shop-orders.png');
  const picture = await decode(file.blob);
  expect([picture.width, picture.height]).toEqual([488, 212]);
}, 30000);

test('a card the view editor dims stays out of the picture', async () => {
  // The database sits where the overview has it while the orders view is
  // edited: at (240, 62), which the frame's bottom margin would show.
  const overview = { ...placed.views.overview.nodes, 'database-1': { x: 240, y: 62 } };
  serve(
    shop,
    { ...placed, views: { ...placed.views, overview: { nodes: overview, viewport: still } } },
    undefined,
    views,
  );
  catchDownloads();
  const screen = await show();
  await screen.getByRole('button', { name: 'Edit Orders path' }).click();
  await vi.waitFor(() =>
    expect(screen.container.querySelectorAll('.svelte-flow__node .card.dimmed')).toHaveLength(1),
  );
  await open(screen);
  await radio(screen, '1x').click();

  const picture = await decode((await exported(screen)).blob);

  expect([picture.width, picture.height]).toEqual([488, 212]);
  expect(picture.at(272 + 26, 162 + 28)).toEqual(darkCanvas);
  expect(picture.at(content.x + 26, content.y + 28)).not.toEqual(darkCanvas);
}, 30000);

test('Escape, the backdrop, Cancel and the close button each close it', async () => {
  const screen = await show();

  await open(screen);
  await userEvent.keyboard('{Escape}');
  await vi.waitFor(() => expect(dialog(screen)).toBeNull());

  await open(screen);
  screen.container
    .querySelector('[role="presentation"]')!
    .dispatchEvent(new MouseEvent('click', { bubbles: true }));
  await vi.waitFor(() => expect(dialog(screen)).toBeNull());

  await open(screen);
  await screen.getByRole('button', { name: 'Cancel' }).click();
  await vi.waitFor(() => expect(dialog(screen)).toBeNull());

  await open(screen);
  await screen.getByRole('button', { name: 'Close export' }).click();
  await vi.waitFor(() => expect(dialog(screen)).toBeNull());
  await expect.element(screen.getByRole('button', { name: 'Export', exact: true })).toBeEnabled();
});

test('a render that fails says so and leaves the dialog open', async () => {
  catchDownloads();
  vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation((callback) =>
    callback(null),
  );
  const screen = await show();
  await open(screen);

  await screen.getByRole('button', { name: 'Export PNG' }).click();

  await expect
    .element(screen.getByRole('alert'), { timeout: 15000 })
    .toHaveTextContent('the browser could not encode the image');
  expect(dialog(screen)).not.toBeNull();
  await expect.element(screen.getByRole('button', { name: 'Export PNG' })).toBeEnabled();
  expect(saved).toHaveLength(0);
}, 30000);

test('the renderer embeds both Plex families from the studio stylesheet', async () => {
  const screen = await show();
  await vi.waitFor(() =>
    expect(screen.container.querySelector('[data-boundary="network"]')).not.toBeNull(),
  );

  const css = await getFontEmbedCSS(screen.container.querySelector<HTMLElement>('.svelte-flow__viewport')!);

  expect(css.match(/@font-face/g)).toHaveLength(5);
  expect(css).toContain('IBM Plex Sans');
  expect(css).toContain('IBM Plex Mono');
  expect(css.match(/url\("data:/g)).toHaveLength(5);
  expect(css).not.toMatch(/\.otf/);
}, 30000);
