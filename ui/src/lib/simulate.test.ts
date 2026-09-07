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

  for (const held of cases) {
    test(held.name, () => {
      const got = run(held.project, held.simulation, held.injection);
      for (const [id, want] of Object.entries(held.expect.nodes)) {
        expect(got.nodes[id]).toBeCloseTo(want, 6);
      }
      for (const [id, want] of Object.entries(held.expect.edges)) {
        expect(got.edges[id]).toBeCloseTo(want, 6);
      }
    });
  }
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
