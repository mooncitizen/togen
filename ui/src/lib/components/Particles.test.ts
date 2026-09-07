import { tick } from 'svelte';
import { afterEach, expect, test, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

import { gatewayServiceDatabase, storeContext, storeWith } from '../../harness.ts';
import Particles, { particleCount } from './Particles.svelte';

// The OS preference, as the studio would see it through matchMedia.
function prefer(reduced: boolean) {
  vi.stubGlobal(
    'matchMedia',
    vi.fn(() => ({ matches: reduced, media: '(prefers-reduced-motion: reduce)' })),
  );
}

// Svelte Flow is not mounted in these tests, so the elements the overlay and the stroke-weight
// rule reach for are stood up by hand, with the class and data-id Svelte Flow gives them.
function edgeElements(): HTMLElement[] {
  return ['edge-1', 'edge-2'].map((id) => {
    const el = document.createElement('div');
    el.className = 'svelte-flow__edge';
    el.dataset.id = id;
    document.body.append(el);
    return el;
  });
}

async function loaded() {
  const store = await storeWith(gatewayServiceDatabase());
  store.addSource();
  store.updateSource('source-1', { target: 'gateway-1', rate: '600/min' });
  store.setFanOut('edge-2', 4);
  return store;
}

afterEach(() => {
  vi.unstubAllGlobals();
  document.querySelectorAll('.svelte-flow__edge').forEach((el) => el.remove());
});

test('an edge carrying nothing gets no particles', async () => {
  prefer(false);
  const store = await storeWith(gatewayServiceDatabase());
  render(Particles, { context: storeContext(store) });
  expect(document.querySelectorAll('[data-particle]').length).toBe(0);
});

// particleCount is the real density logic; calling it directly proves the busiest edge
// carries the most without needing a Svelte Flow tree to draw a path first.
test('particle count rises with share', () => {
  expect(particleCount(0.1)).toBe(1);
  expect(particleCount(0.5)).toBe(3);
  expect(particleCount(1)).toBe(6);
  expect(particleCount(0.5)).toBeGreaterThan(particleCount(0.1));
  expect(particleCount(1)).toBeGreaterThan(particleCount(0.5));
});

// pathFor reads the d attribute Svelte Flow puts on the edge, and a stand-in element has none,
// so no circle is drawn even though there is load. This is the regression case: a particle must
// never render without a path to ride. Whether a drawn particle actually moves is only
// verifiable in a real browser against a real Svelte Flow tree.
test('an edge with no rendered path in the DOM gets no particles, even with load', async () => {
  prefer(false);
  edgeElements();
  const store = await loaded();
  render(Particles, { context: storeContext(store) });
  await tick();
  expect(document.querySelector('[data-particles]')).not.toBeNull();
  expect(document.querySelectorAll('[data-particle]').length).toBe(0);
});

// Zero particles is true either way while there is no path to ride, so what separates the two
// readings is the overlay itself: under reduced motion it is never mounted.
test('reduced motion mounts no particle overlay, and motion does', async () => {
  prefer(true);
  const store = await loaded();
  render(Particles, { context: storeContext(store) });
  await tick();
  expect(document.querySelector('[data-particles]')).toBeNull();

  prefer(false);
  const moving = await loaded();
  render(Particles, { context: storeContext(moving) });
  await tick();
  expect(document.querySelector('[data-particles]')).not.toBeNull();
});

// The reduced-motion reading is a stroke weight per edge, applied by the rule in app.css that
// keys off this custom property. edge-2 carries four times what edge-1 does, so it reads heavier.
test('reduced motion weights the edges instead of animating them', async () => {
  prefer(true);
  const [one, two] = edgeElements();
  const store = await loaded();
  render(Particles, { context: storeContext(store) });
  await tick();
  expect(Number(one.style.getPropertyValue('--togen-edge-weight'))).toBeCloseTo(1.5, 6);
  expect(Number(two.style.getPropertyValue('--togen-edge-weight'))).toBeCloseTo(3, 6);
});

test('with motion the edges are left unweighted', async () => {
  prefer(false);
  const [one, two] = edgeElements();
  const store = await loaded();
  render(Particles, { context: storeContext(store) });
  await tick();
  expect(one.style.getPropertyValue('--togen-edge-weight')).toBe('');
  expect(two.style.getPropertyValue('--togen-edge-weight')).toBe('');
});
