import { expect, test } from 'vitest';

import {
  bandFor,
  edgePath,
  headLength,
  headPath,
  labelBudget,
  lanesFor,
  offsetsFor,
  sourceGap,
  strokeEnd,
  strokeFor,
  targetGap,
  textureFor,
} from './edges.ts';
import type { Edge } from './types.ts';

const edge = (id: string, from: string, to: string, relation: Edge['relation']): Edge => ({
  id,
  from,
  to,
  relation,
});

test('the texture follows the relation family, not the pair of node types', () => {
  expect(textureFor('routes')).toBe('solid');
  expect(textureFor('calls')).toBe('solid');
  expect(textureFor('publishes')).toBe('dashed');
  expect(textureFor('consumes')).toBe('dashed');
  expect(textureFor('reads')).toBe('dotted');
  expect(textureFor('writes')).toBe('dotted');
});

test('the rate lands in one of three bands, and nothing to rank is band one', () => {
  expect(bandFor(100, 100)).toBe(3);
  expect(bandFor(40, 100)).toBe(2);
  expect(bandFor(5, 100)).toBe(1);
  expect(bandFor(0, 0)).toBe(1);
  expect(bandFor(0, 100)).toBe(1);
});

test('a dotted gap scales with the stroke, so the rhythm holds across the bands', () => {
  const thin = strokeFor('dotted', 1, 1);
  const thick = strokeFor('dotted', 3, 1);
  expect(thin.width).toBe(1.5);
  expect(thick.width).toBe(3);
  expect(Number(thin.dash!.split(' ')[1]) / thin.width).toBeCloseTo(
    Number(thick.dash!.split(' ')[1]) / thick.width,
    6,
  );
  expect(thin.cap).toBe('round');
});

test('dotted gives up detail a step at a time as the canvas pulls back', () => {
  expect(strokeFor('dotted', 2, 1).dash).toMatch(/^0\.1 /);
  expect(strokeFor('dotted', 2, 0.6).dash).toBe('2 3');
  expect(strokeFor('dotted', 2, 0.3).dash).toBeNull();
});

test('a solid relation is never broken, whatever the zoom', () => {
  for (const zoom of [1, 0.6, 0.3]) {
    expect(strokeFor('solid', 3, zoom).dash).toBeNull();
  }
});

test('the head sits in the gap and the stroke stops at its base', () => {
  const width = 2;
  const end = strokeEnd(400, width);
  expect(end).toBeCloseTo(400 - targetGap - headLength(width), 6);
  const [, base, tip] = /M([\d.]+) [\d.]+L([\d.]+) /.exec(headPath(400, 50, width))!;
  expect(Number(base)).toBeCloseTo(end, 6);
  expect(Number(tip)).toBeCloseTo(400 - targetGap, 6);
});

test('a path clears its own card and stops short of the next', () => {
  const d = edgePath({ sourceX: 100, sourceY: 50, targetX: 400, targetY: 120, width: 2 });
  expect(d.startsWith(`M${100 + sourceGap} 50`)).toBe(true);
  expect(d.endsWith(`L${strokeEnd(400, 2)} 120`)).toBe(true);
});

test('a lane nudge moves the mid-column and nothing else', () => {
  const plain = edgePath({ sourceX: 100, sourceY: 50, targetX: 400, targetY: 120 });
  const nudged = edgePath({ sourceX: 100, sourceY: 50, targetX: 400, targetY: 120, lane: 9 });
  expect(nudged).not.toBe(plain);
  expect(nudged.startsWith(`M${100 + sourceGap} 50`)).toBe(true);
});

test('a back edge drops below both cards and returns outside the target', () => {
  const d = edgePath({ sourceX: 400, sourceY: 50, targetX: 100, targetY: 200, width: 1.25 });
  const ys = [...d.matchAll(/[ML]([\d.-]+) ([\d.-]+)/g)].map((match) => Number(match[2]));
  expect(Math.max(...ys)).toBeGreaterThan(200);
  const xs = [...d.matchAll(/[ML]([\d.-]+) ([\d.-]+)/g)].map((match) => Number(match[1]));
  expect(Math.min(...xs)).toBeLessThan(100);
});

test('edges joining the same two cards are split, whichever way round they run', () => {
  const edges = [
    edge('a', 'orders', 'orders-db', 'writes'),
    edge('b', 'orders', 'orders-db', 'reads'),
    edge('c', 'web', 'worker', 'calls'),
    edge('d', 'worker', 'web', 'calls'),
  ];
  const offsets = offsetsFor(edges, () => 0);
  expect(offsets.a.paired).toBe(true);
  expect(offsets.a.pair).toBeCloseTo(-offsets.b.pair, 6);
  expect(offsets.a.pair).not.toBe(0);
  expect(offsets.c.paired).toBe(true);
  expect(offsets.c.pair).toBeCloseTo(-offsets.d.pair, 6);
});

test('a single relation between two cards is not split', () => {
  const offsets = offsetsFor([edge('a', 'api', 'orders', 'routes')], () => 0);
  expect(offsets.a).toEqual({ pair: 0, paired: false, source: 0, target: 0, chip: 0 });
});

test('edges leaving one card fan across its side in the order of their targets', () => {
  const ys: Record<string, number> = { api: 100, high: 0, middle: 100, low: 200 };
  const offsets = offsetsFor(
    [
      edge('a', 'api', 'low', 'routes'),
      edge('b', 'api', 'high', 'routes'),
      edge('c', 'api', 'middle', 'routes'),
    ],
    (id) => ys[id] ?? 0,
  );
  expect(offsets.b.source).toBeLessThan(offsets.c.source);
  expect(offsets.c.source).toBeLessThan(offsets.a.source);
  expect(offsets.b.source + offsets.a.source).toBeCloseTo(0, 6);
});

test('runs that would share a mid-column are pushed apart, and lone runs are left alone', () => {
  const together = [
    edge('a', 'one', 'two', 'calls'),
    edge('b', 'three', 'four', 'calls'),
  ];
  const lanes = lanesFor(together, () => 500);
  expect(lanes.a).toBeCloseTo(-lanes.b, 6);
  expect(lanes.a).not.toBe(0);

  const apart = lanesFor(together, (item) => (item.id === 'a' ? 200 : 900));
  expect(apart.a).toBe(0);
  expect(apart.b).toBe(0);
});

test('the label budget is a number of relations, not a guess per canvas', () => {
  expect(labelBudget).toBeGreaterThan(0);
});
