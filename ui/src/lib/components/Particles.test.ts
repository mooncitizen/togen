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

afterEach(() => {
  vi.unstubAllGlobals();
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

// Outside a real Svelte Flow tree there is no rendered path for any edge, so pathFor always
// returns null and no circle is drawn even though there is load. This is the regression case:
// a particle must never render without a path to ride. Whether a drawn particle actually
// moves is only verifiable in a real browser against a real Svelte Flow tree.
test('an edge with no rendered path in the DOM gets no particles, even with load', async () => {
  prefer(false);
  const store = await storeWith(gatewayServiceDatabase());
  store.addSource();
  store.updateSource('source-1', { target: 'gateway-1', rate: '600/min' });
  store.setFanOut('edge-2', 4);
  render(Particles, { context: storeContext(store) });
  await tick();
  expect(document.querySelectorAll('[data-particle]').length).toBe(0);
});

test('reduced motion draws no particles', async () => {
  prefer(true);
  const store = await storeWith(gatewayServiceDatabase());
  store.addSource();
  store.updateSource('source-1', { target: 'gateway-1', rate: '600/min' });
  render(Particles, { context: storeContext(store) });
  await tick();
  expect(document.querySelectorAll('[data-particle]').length).toBe(0);
});
