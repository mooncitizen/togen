import type { Project, SimResult, Simulation } from './types.ts';

export const hoursPerMonth = 730;
export const secondsPerMonth = hoursPerMonth * 3600;

const perMonth: Record<string, number> = {
  min: 60 * hoursPerMonth,
  hour: hoursPerMonth,
  day: hoursPerMonth / 24,
  month: 1,
};

const scale: Record<string, number> = { '': 1, k: 1e3, M: 1e6 };
const rateForm = /^[0-9]+(\.[0-9]+)?[kM]?\/(min|hour|day|month)$/;

// The same normalisation the Go side does, so a rate means one thing (ADR 0009). Go returns an
// error on a rate it cannot read; here a half-typed rate is normal, so it reads as zero instead.
export function parseRate(text: string): number {
  if (!rateForm.test(text)) {
    return 0;
  }
  const [count, period] = text.split('/');
  const last = count[count.length - 1];
  const suffix = last === 'k' || last === 'M' ? last : '';
  const n = Number(suffix === '' ? count : count.slice(0, -1));
  return Math.round(n * scale[suffix] * perMonth[period]);
}

type Arc = { edge: string; from: string; to: string; per: number };

function arcs(project: Project, sim: Simulation): Arc[] {
  return project.edges.map((edge) => {
    const per = sim.edges?.[edge.id] ?? 1;
    return edge.relation === 'consumes'
      ? { edge: edge.id, from: edge.to, to: edge.from, per }
      : { edge: edge.id, from: edge.from, to: edge.to, per };
  });
}

const settleTolerance = 1e-9;
const settleSweeps = 20000;
const settleWindow = 1000;
const runawayFactor = 1e12;

function settledAt(was: number, now: number): boolean {
  let change = Math.abs(now - was);
  const scaleValue = Math.abs(now);
  if (scaleValue > 1e-12) {
    change /= scaleValue;
  }
  return change < settleTolerance;
}

// Reversing 'consumes' can close a loop, and the IR also allows plain call cycles, so the Go
// engine finds a fixed point by Jacobi iteration rather than a topological sort: every sweep
// reads only the previous sweep's rates, which is what makes the answer independent of node
// order. This mirrors that.
//
// The Go engine returns an error when a loop does not settle. This function cannot throw: the
// studio calls it on every drag while the graph may be mid-edit, and a blank canvas is worse than
// an approximate one. So the two engines agree on which loops are divergent and part company only
// on what they do about it. There are three outcomes.
//
// Settled: the rates, bit for bit what Go returns.
//
// Divergent: zeros. A rate that is not finite or past the injected total times runawayFactor, and
// also a loop that fails Go's contraction window, which is what catches a gain of exactly 1: the
// biggest change of any sweep in a window of settleWindow sweeps has to keep shrinking from one
// window to the next, and at gain 1 the rates climb by a constant amount for ever, so it does not.
// Neither has a drawable answer, and the validation message the project carries for the cycle
// tells the story.
//
// Still contracting when the sweeps run out: the last iterate. The sweep climbs towards the fixed
// point from below, so that is an underestimate of the real rates rather than a fiction. Go's
// budget is larger and it errors here instead, but the classification above is the same in both.
export function run(project: Project, sim: Simulation, injection: Record<string, number>): SimResult {
  const all = arcs(project, sim);
  const incoming = new Map<string, Arc[]>();
  for (const arc of all) {
    const at = incoming.get(arc.to);
    if (at) {
      at.push(arc);
    } else {
      incoming.set(arc.to, [arc]);
    }
  }

  const entry: Record<string, number> = {};
  let injected = 0;
  for (const source of sim.sources) {
    const value = injection[source.id] ?? 0;
    entry[source.target] = (entry[source.target] ?? 0) + value;
    injected += Math.abs(value);
  }
  const runaway = injected * runawayFactor;

  const ids: string[] = [];
  let nodes: Record<string, number> = {};
  for (const node of project.nodes) {
    if (!(node.id in nodes)) {
      ids.push(node.id);
    }
    nodes[node.id] = 0;
  }
  let next: Record<string, number> = {};

  let divergent = false;
  let thisWindow = 0;
  let lastWindow = 0;
  for (let sweep = 1; sweep <= settleSweeps; sweep++) {
    let settled = true;
    let biggest = 0;
    for (const id of ids) {
      let rate = entry[id] ?? 0;
      for (const arc of incoming.get(id) ?? []) {
        rate += (nodes[arc.from] ?? 0) * arc.per;
      }
      if (!Number.isFinite(rate) || Math.abs(rate) > runaway) {
        divergent = true;
        break;
      }
      if (!settledAt(nodes[id], rate)) {
        settled = false;
      }
      const change = Math.abs(rate - nodes[id]);
      if (change > biggest) {
        biggest = change;
      }
      next[id] = rate;
    }
    if (divergent) {
      break;
    }
    [nodes, next] = [next, nodes];
    if (settled) {
      break;
    }
    if (biggest > thisWindow) {
      thisWindow = biggest;
    }
    if (sweep % settleWindow === 0) {
      if (lastWindow > 0 && thisWindow >= lastWindow) {
        divergent = true;
        break;
      }
      lastWindow = thisWindow;
      thisWindow = 0;
    }
  }

  const outNodes: Record<string, number> = {};
  const outEdges: Record<string, number> = {};
  for (const id of ids) {
    outNodes[id] = divergent ? 0 : (nodes[id] ?? 0);
  }
  for (const arc of all) {
    outEdges[arc.edge] = divergent ? 0 : (outNodes[arc.from] ?? 0) * arc.per;
  }
  return { nodes: outNodes, edges: outEdges };
}

export function monthly(sim: Simulation): Record<string, number> {
  const out: Record<string, number> = {};
  for (const source of sim.sources) {
    const base = parseRate(source.rate);
    let total = base;
    for (const burst of sim.bursts ?? []) {
      if (burst.source === source.id) {
        total += (burst.multiplier - 1) * (base / secondsPerMonth) * burst.minutes * 60 * burst.timesPerMonth;
      }
    }
    out[source.id] = total;
  }
  return out;
}

const displayPeriods: [string, number][] = [
  ['min', perMonth.min],
  ['hour', perMonth.hour],
  ['day', perMonth.day],
  ['month', perMonth.month],
];

// The inverse of parseRate: a per-second figure back into "800/min" or "2M/month", picking
// the smallest period that keeps the number at least one, and a k/M suffix once it is large.
export function rateLabel(perSecond: number): string {
  if (!Number.isFinite(perSecond) || perSecond <= 0) {
    return '0/min';
  }
  const perMonthTotal = perSecond * secondsPerMonth;
  const [period, count] =
    displayPeriods.find(([, count]) => perMonthTotal / count >= 1) ??
    displayPeriods[displayPeriods.length - 1];
  const value = perMonthTotal / count;
  const [suffix, factor] = value >= 1e6 ? ['M', 1e6] : value >= 1e3 ? ['k', 1e3] : ['', 1];
  const scaled = value / factor;
  const digits = scaled >= 100 ? 0 : scaled >= 10 ? 1 : 2;
  return `${trimmed(scaled.toFixed(digits))}${suffix}/${period}`;
}

function trimmed(text: string): string {
  return text.includes('.') ? text.replace(/0+$/, '').replace(/\.$/, '') : text;
}

// Rates per second at one scenario: an id that matches no burst gives the flat rate,
// otherwise the burst that multiplies its own source.
export function instant(sim: Simulation, scenario: string): Record<string, number> {
  const out: Record<string, number> = {};
  for (const source of sim.sources) {
    let rate = parseRate(source.rate) / secondsPerMonth;
    for (const burst of sim.bursts ?? []) {
      if (burst.id === scenario && burst.source === source.id) {
        rate *= burst.multiplier;
      }
    }
    out[source.id] = rate;
  }
  return out;
}
