import { nodeHeight } from './layout.ts';
import type { Edge, Relation } from './types.ts';

export type Texture = 'solid' | 'dashed' | 'dotted';
export type Band = 1 | 2 | 3;

export const sourceGap = 5;
export const targetGap = 10;
export const cornerRadius = 12;

// Past this many relations the resting canvas drops its chips and leans on focus.
export const labelBudget = 24;

const textures: Record<Relation, Texture> = {
  routes: 'solid',
  calls: 'solid',
  publishes: 'dashed',
  consumes: 'dashed',
  reads: 'dotted',
  writes: 'dotted',
};

export function textureFor(relation: Relation): Texture {
  return textures[relation] ?? 'solid';
}

// Three bands rather than a continuous width: a scale the eye can rank, not a
// smear. Without a simulation nothing is ranked and everything sits in band one.
export function bandFor(rate: number, busiest: number): Band {
  if (busiest <= 0 || rate <= 0) {
    return 1;
  }
  const share = rate / busiest;
  if (share >= 0.66) {
    return 3;
  }
  return share >= 0.25 ? 2 : 1;
}

const bandWidths: Record<Band, number> = { 1: 1.25, 2: 2, 3: 3 };

export function headLength(width: number): number {
  return 6.5 + width * 0.7;
}

export type Stroke = { width: number; dash: string | null; cap: 'butt' | 'round' };

// A dot has to be round and it has to have a body, so a dotted edge never drops
// below 1.5px and its gap scales with the stroke: the rhythm stays put across
// the bands instead of the pattern changing meaning. Zoomed out it gives up
// detail a step at a time rather than dissolving.
export function strokeFor(texture: Texture, band: Band, zoom: number): Stroke {
  const base = bandWidths[band];
  if (texture === 'dashed') {
    return { width: base, dash: `${(base * 4).toFixed(1)} ${(base * 2.6).toFixed(1)}`, cap: 'butt' };
  }
  if (texture === 'dotted') {
    if (zoom < 0.45) {
      return { width: base, dash: null, cap: 'round' };
    }
    if (zoom < 0.7) {
      return { width: Math.max(1.25, base), dash: '2 3', cap: 'butt' };
    }
    const width = Math.max(1.5, base);
    return { width, dash: `0.1 ${(width * 2.9).toFixed(2)}`, cap: 'round' };
  }
  return { width: base, dash: null, cap: 'round' };
}

// The head is filled and its base sits exactly where the stroke ends, so the two
// read as one mark rather than an arrow parked on a line.
export function headPath(targetX: number, y: number, width: number): string {
  const length = headLength(width);
  const half = length * 0.52;
  const tip = targetX - targetGap;
  return `M${tip - length} ${y - half}L${tip} ${y}L${tip - length} ${y + half}Z`;
}

export function strokeEnd(targetX: number, width: number): number {
  return targetX - targetGap - headLength(width);
}

function rounded(points: [number, number][], radius: number): string {
  let d = `M${points[0][0]} ${points[0][1]}`;
  for (let i = 1; i < points.length - 1; i += 1) {
    const [px, py] = points[i - 1];
    const [x, y] = points[i];
    const [nx, ny] = points[i + 1];
    const back = Math.hypot(x - px, y - py);
    const on = Math.hypot(nx - x, ny - y);
    if (back === 0 || on === 0) {
      continue;
    }
    const r = Math.min(radius, back / 2, on / 2);
    d += `L${x - ((x - px) / back) * r} ${y - ((y - py) / back) * r}`;
    d += `Q${x} ${y} ${x + ((nx - x) / on) * r} ${y + ((ny - y) / on) * r}`;
  }
  const last = points[points.length - 1];
  return `${d}L${last[0]} ${last[1]}`;
}

export type PathPoints = {
  sourceX: number;
  sourceY: number;
  targetX: number;
  targetY: number;
  lane?: number;
  width?: number;
};

// A back edge is a back edge: it drops below both cards and returns along the
// outside rather than being smuggled into the forward run, because a cycle
// between two services is worth seeing.
export function edgePath({
  sourceX,
  sourceY,
  targetX,
  targetY,
  lane = 0,
  width = 1.25,
}: PathPoints): string {
  const from = sourceX + sourceGap;
  const to = strokeEnd(targetX, width);
  if (targetX < sourceX) {
    const under = Math.max(sourceY, targetY) + nodeHeight;
    return rounded(
      [
        [from, sourceY],
        [from + 18, sourceY],
        [from + 18, under],
        [targetX - 30, under],
        [targetX - 30, targetY],
        [to, targetY],
      ],
      cornerRadius,
    );
  }
  if (Math.abs(sourceY - targetY) < 1) {
    return `M${from} ${sourceY}L${to} ${targetY}`;
  }
  const mid = (sourceX + targetX) / 2 + lane;
  return rounded(
    [
      [from, sourceY],
      [mid, sourceY],
      [mid, targetY],
      [to, targetY],
    ],
    cornerRadius,
  );
}

export type Offsets = {
  pair: number;
  paired: boolean;
  source: number;
  target: number;
  chip: number;
};

// Chips dock behind the head, so several relations arriving at one card would
// stack them. They step back along the run instead, one chip's width at a time.
export const chipStep = 62;
export const chipGap = 9;

// A chip that would have to sit in the first half of its run is not shown at all:
// an overlapping chip is worse than none, and the rate is on the card either way.
export function chipAt(
  sourceX: number,
  targetX: number,
  width: number,
  index: number,
): number | null {
  const end = strokeEnd(targetX, width);
  const back = chipGap + index * chipStep;
  return back > (targetX - sourceX) * 0.4 ? null : end - back;
}

const pairKey = (edge: Edge) => [edge.from, edge.to].sort().join(' ');
const byId = (a: Edge, b: Edge) => a.id.localeCompare(b.id);
const spread = (index: number, count: number, step: number) =>
  count < 2 ? 0 : (index - (count - 1) / 2) * step;

function group(edges: Edge[], key: (edge: Edge) => string): Edge[][] {
  const groups = new Map<string, Edge[]>();
  for (const edge of edges) {
    groups.set(key(edge), [...(groups.get(key(edge)) ?? []), edge]);
  }
  return [...groups.values()];
}

// Everything one edge cannot work out on its own: how many others join the same
// two cards, and how many leave or arrive at the same side. Positions come from
// the store, so this settles as a drag does.
export function offsetsFor(edges: Edge[], centreY: (id: string) => number): Record<string, Offsets> {
  const out: Record<string, Offsets> = {};
  for (const edge of edges) {
    out[edge.id] = { pair: 0, paired: false, source: 0, target: 0, chip: 0 };
  }

  for (const pair of group(edges, pairKey)) {
    const ordered = [...pair].sort((a, b) => a.from.localeCompare(b.from) || byId(a, b));
    ordered.forEach((edge, index) => {
      out[edge.id].pair = spread(index, ordered.length, 7);
      out[edge.id].paired = ordered.length > 1;
    });
  }

  const fan = (side: 'from' | 'to', far: 'to' | 'from', key: 'source' | 'target') => {
    for (const sharing of group(edges, (edge) => edge[side])) {
      const ordered = [...sharing].sort((a, b) => centreY(a[far]) - centreY(b[far]) || byId(a, b));
      const step = ordered.length < 2 ? 9 : Math.min(9, 34 / (ordered.length - 1));
      ordered.forEach((edge, index) => {
        out[edge.id][key] = spread(index, ordered.length, step);
        if (key === 'target') {
          out[edge.id].chip = index;
        }
      });
    }
  };
  fan('from', 'to', 'source');
  fan('to', 'from', 'target');

  return out;
}

// Runs that would share a mid-column are pushed apart, which is the one thing
// the casing cannot fix. Bucketed, not routed: nothing here reruns per frame.
export function lanesFor(edges: Edge[], midOf: (edge: Edge) => number): Record<string, number> {
  const out: Record<string, number> = {};
  for (const bucket of group(edges, (edge) => String(Math.round(midOf(edge) / 12)))) {
    const ordered = [...bucket].sort(byId);
    ordered.forEach((edge, index) => {
      out[edge.id] = spread(index, ordered.length, 9);
    });
  }
  return out;
}
