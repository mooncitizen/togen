import { expect, test } from 'vitest';

import { gatewayServiceDatabase, storeWith } from '../../harness.ts';
import type { Project } from '../types.ts';

// A queue closes the loop, which is the aws-basic shape: the IR allows it and carries no
// validation message for it, so the divergent flag is the only warning the studio has.
function queueLoop(): Project {
  return {
    version: 1,
    name: 'shop',
    provider: 'aws',
    region: 'eu-west-2',
    environment: 'dev',
    nodes: [
      { id: 'gateway-1', type: 'gateway', name: 'api' },
      { id: 'function-1', type: 'function', name: 'orders' },
      { id: 'queue-1', type: 'queue', name: 'jobs' },
      { id: 'function-2', type: 'function', name: 'worker' },
    ],
    edges: [
      { id: 'edge-1', from: 'gateway-1', to: 'function-1', relation: 'routes' },
      { id: 'edge-2', from: 'function-1', to: 'queue-1', relation: 'publishes' },
      { id: 'edge-3', from: 'function-2', to: 'queue-1', relation: 'consumes' },
      { id: 'edge-4', from: 'function-2', to: 'function-1', relation: 'calls' },
    ],
  };
}

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

test('a loop that never settles flags the rates as divergent', async () => {
  const store = await storeWith(queueLoop());
  store.addSource();
  store.updateSource('source-1', { target: 'gateway-1', rate: '60/min' });
  expect(store.rates.divergent).toBe(true);
  expect(store.rates.nodes['function-1']).toBe(0);
  store.setFanOut('edge-2', 0.2);
  expect(store.rates.divergent).toBe(false);
  expect(store.rates.nodes['function-1']).toBeCloseTo(1.25, 6);
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
