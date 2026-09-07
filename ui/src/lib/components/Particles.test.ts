import { tick } from 'svelte';
import { afterEach, expect, test, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

import { gatewayServiceDatabase, storeContext, storeWith } from '../../harness.ts';
import Particles from './Particles.svelte';

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

test('the busiest edge carries the most', async () => {
  prefer(false);
  const store = await storeWith(gatewayServiceDatabase());
  store.addSource();
  store.updateSource('source-1', { target: 'gateway-1', rate: '600/min' });
  store.setFanOut('edge-2', 4);
  render(Particles, { context: storeContext(store) });
  await tick();
  const first = document.querySelectorAll('[data-particle][data-edge="edge-1"]').length;
  const second = document.querySelectorAll('[data-particle][data-edge="edge-2"]').length;
  expect(second).toBeGreaterThan(first);
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
