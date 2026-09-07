import { describe, expect, test } from 'vitest';

import { instant, monthly, run, secondsPerMonth } from './simulate.ts';
import type { Project } from './types.ts';

type Case = {
  name: string;
  project: Project;
  simulation: Parameters<typeof run>[1];
  injection: Record<string, number>;
  expect: { nodes: Record<string, number>; edges: Record<string, number> };
};

const cases = Object.values(
  import.meta.glob<Case>('./testdata/cases/*.json', { eager: true, import: 'default' }),
);

describe('the shared cases', () => {
  test('there are some', () => {
    expect(cases.length).toBeGreaterThan(0);
  });

  // The same tolerance the Go side holds the cases to: relative once the rate is above one,
  // since a settled loop lands within 1e-9 of the fixed point rather than exactly on it.
  const near = (got: number, want: number) => {
    expect(Math.abs(got - want)).toBeLessThanOrEqual(1e-6 * Math.max(1, Math.abs(want)));
  };

  for (const held of cases) {
    test(held.name, () => {
      const got = run(held.project, held.simulation, held.injection);
      for (const [id, want] of Object.entries(held.expect.nodes)) {
        near(got.nodes[id], want);
      }
      for (const [id, want] of Object.entries(held.expect.edges)) {
        near(got.edges[id], want);
      }
    });
  }
});

const loop: Project = {
  version: 1,
  name: 'loop',
  provider: 'aws',
  region: 'eu-west-2',
  environment: 'dev',
  nodes: [
    { id: 'service-1', type: 'service', name: 'a' },
    { id: 'service-2', type: 'service', name: 'b' },
  ],
  edges: [
    { id: 'edge-1', from: 'service-1', to: 'service-2', relation: 'calls' },
    { id: 'edge-2', from: 'service-2', to: 'service-1', relation: 'calls' },
  ],
};

const roundTheLoop = (gain: number) =>
  run(
    loop,
    {
      version: 1,
      sources: [{ id: 's', name: 'S', target: 'service-1', rate: '60/min' }],
      edges: { 'edge-1': gain, 'edge-2': 1 },
    },
    { s: 1 },
  );

test('a runaway loop draws nothing', () => {
  expect(roundTheLoop(2).nodes['service-1']).toBe(0);
  expect(roundTheLoop(2).edges['edge-1']).toBe(0);
});

test('a loop too slow to settle draws the last iterate, not zero', () => {
  const got = roundTheLoop(0.9999).nodes['service-1'];
  expect(got).toBeGreaterThan(0);
  expect(got).toBeLessThan(10000);
});

const sim = {
  version: 1,
  sources: [{ id: 'mobile', name: 'Mobile app', target: 'gateway-1', rate: '800/min' }],
  bursts: [{ id: 'launch', name: 'Launch', source: 'mobile', multiplier: 6, minutes: 30, timesPerMonth: 2 }],
  edges: {},
};

test('monthly folds in the burst', () => {
  const base = 800 * 60 * 730;
  expect(monthly(sim).mobile).toBeCloseTo(base + 5 * (base / secondsPerMonth) * 30 * 60 * 2, 6);
});

test('instant applies the chosen burst only', () => {
  const base = (800 * 60 * 730) / secondsPerMonth;
  expect(instant(sim, '').mobile).toBeCloseTo(base, 6);
  expect(instant(sim, 'launch').mobile).toBeCloseTo(base * 6, 6);
});
