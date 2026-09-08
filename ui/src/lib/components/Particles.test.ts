import { afterEach, beforeEach, expect, test, vi } from 'vitest';

import { gatewayServiceDatabase, overviewLayout, reset, serve, show } from '../../harness.ts';
import { Store } from '../store.svelte.ts';
import type { Simulation } from '../types.ts';
import { particleCount } from './Particles.svelte';

const placed = overviewLayout({
  'gateway-1': { x: 20, y: 20 },
  'service-1': { x: 260, y: 20 },
  'database-1': { x: 500, y: 20 },
});

// edge-2 carries four times what edge-1 does, so it reads as the busiest.
const loaded: Simulation = {
  version: 1,
  sources: [{ id: 'source-1', name: 'traffic', target: 'gateway-1', rate: '600/min' }],
  edges: { 'edge-2': 4 },
};

// The OS preference, as the studio would see it through matchMedia. Only the
// reduced-motion query is answered by the stub; the theme query keeps its own reading.
function prefer(reduced: boolean) {
  vi.stubGlobal(
    'matchMedia',
    vi.fn((query: string) => ({
      matches: query.includes('prefers-reduced-motion') ? reduced : false,
      media: query,
      addEventListener: () => {},
      removeEventListener: () => {},
    })),
  );
}

function circles(edgeId?: string): Element[] {
  const selector =
    edgeId === undefined ? '[data-particle]' : `[data-particle][data-edge="${edgeId}"]`;
  return Array.from(document.querySelectorAll(selector));
}

async function moving() {
  const screen = await show();
  await vi.waitFor(() => expect(circles().length).toBeGreaterThan(0));
  return screen;
}

beforeEach(() => {
  reset();
  serve(gatewayServiceDatabase(), placed, undefined, undefined, loaded);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

// particleCount is the density logic on its own: the busiest edge carries the most.
test('particle count rises with share', () => {
  expect(particleCount(0.1)).toBe(1);
  expect(particleCount(0.5)).toBe(3);
  expect(particleCount(1)).toBe(6);
  expect(particleCount(0.5)).toBeGreaterThan(particleCount(0.1));
  expect(particleCount(1)).toBeGreaterThan(particleCount(0.5));
});

// The regression: the paths are read after Svelte Flow paints, so a first load with a
// scenario already running draws particles with nothing else touched.
test('particles appear on first load, with no further interaction', async () => {
  prefer(false);
  await moving();
  expect(circles('edge-1').length).toBe(particleCount(0.25));
  expect(circles('edge-2').length).toBe(particleCount(1));
});

// The other regression: drawn outside the transformed viewport the dots sit in empty
// space and pan and zoom leave them behind.
test('the overlay is drawn inside the flow viewport', async () => {
  prefer(false);
  await moving();
  const overlay = document.querySelector('[data-particles]');
  expect(overlay).not.toBeNull();
  expect(overlay!.closest('.svelte-flow__viewport')).not.toBeNull();
});

// A circle with no animateMotion would sit as a static dot at the origin, so every one
// drawn must carry the path it rides.
test('every particle rides a path Svelte Flow drew', async () => {
  prefer(false);
  await moving();
  const drawn = new Set(
    Array.from(document.querySelectorAll('.svelte-flow__edges .svelte-flow__edge')).map(
      (edge) => edge.querySelector('path.svelte-flow__edge-path')?.getAttribute('d') ?? '',
    ),
  );
  for (const circle of circles()) {
    const motion = circle.querySelector('animateMotion');
    expect(motion).not.toBeNull();
    expect(drawn.has(motion!.getAttribute('path') ?? '')).toBe(true);
  }
});

// The whole-particle gate, and the re-read that keeps the dots on the line when Svelte
// Flow redraws an edge: take the path away and that edge's particles go with it.
test('an edge that loses its path loses its particles', async () => {
  prefer(false);
  await moving();
  const path = document.querySelector(
    '.svelte-flow__edge[data-id="edge-1"] path.svelte-flow__edge-path',
  );
  expect(path).not.toBeNull();
  path!.removeAttribute('d');
  await vi.waitFor(() => expect(circles('edge-1').length).toBe(0));
  expect(circles('edge-2').length).toBe(particleCount(1));
});

test('reduced motion mounts no particle overlay', async () => {
  prefer(true);
  await show();
  await vi.waitFor(() =>
    expect(document.querySelector('.svelte-flow__edge[data-id="edge-1"]')).not.toBeNull(),
  );
  expect(document.querySelector('[data-particles]')).toBeNull();
  expect(circles().length).toBe(0);
});

// The traffic reading with motion off is the weight band the edge already carries,
// which is on the canvas whether or not the dots are.
test('reduced motion leaves the busiest edge in a heavier band than the quietest', async () => {
  prefer(true);
  const store = new Store();
  await store.load();

  const band = (id: string) =>
    (store.flowEdges.find((edge) => edge.id === id)?.data as { band: number }).band;
  await vi.waitFor(() => expect(band('edge-2')).toBe(3));
  expect(band('edge-1')).toBeLessThan(3);
});

// No scenario means no rates, so nothing is drawn even though the overlay would mount.
test('an edge carrying nothing gets no particles', async () => {
  prefer(false);
  reset();
  serve(gatewayServiceDatabase(), placed);
  await show();
  expect(circles().length).toBe(0);
});
