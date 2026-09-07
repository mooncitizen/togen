import { expect, test } from 'vitest';

import { gatewayServiceDatabase, storeWith } from '../../harness.ts';

test('a source gives every node on the path a rate', async () => {
  const store = await storeWith(gatewayServiceDatabase());
  store.addSource();
  store.updateSource('source-1', { target: 'gateway-1', rate: '60/min' });
  expect(store.rates.nodes['gateway-1']).toBeCloseTo(1, 6);
  expect(store.rates.nodes['service-1']).toBeCloseTo(1, 6);
});

test('a fan-out multiplies the branch below it', async () => {
  const store = await storeWith(gatewayServiceDatabase());
  store.addSource();
  store.updateSource('source-1', { target: 'gateway-1', rate: '60/min' });
  store.setFanOut('edge-2', 3);
  expect(store.rates.nodes['database-1']).toBeCloseTo(3, 6);
  store.setFanOut('edge-2', undefined);
  expect(store.rates.nodes['database-1']).toBeCloseTo(1, 6);
});

test('the scenario changes the rates and nothing else', async () => {
  const store = await storeWith(gatewayServiceDatabase());
  store.addSource();
  store.updateSource('source-1', { target: 'gateway-1', rate: '60/min' });
  store.addBurst();
  store.updateBurst('burst-1', { source: 'source-1', multiplier: 4, minutes: 10, timesPerMonth: 1 });
  store.setScenario('burst-1');
  expect(store.rates.nodes['gateway-1']).toBeCloseTo(4, 6);
  store.setScenario('');
  expect(store.rates.nodes['gateway-1']).toBeCloseTo(1, 6);
});
