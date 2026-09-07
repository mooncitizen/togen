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

// The same normalisation the Go side does, so a rate means one thing (ADR 0009).
export function parseRate(text: string): number {
  if (text === '') {
    return 0;
  }
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
const settleSweeps = 2000;
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
// The Go engine refuses a loop that does not settle (gain 1 or more, or one still shrinking too
// slowly to be practical) by returning an error. This function cannot do that: the studio calls
// it on every drag while the graph may be mid-edit, and a blank canvas is worse than a wrong one.
// So an unsettled graph comes back as zeros here instead of throwing; the validation message the
// project already carries for a cycle tells the story instead. The fixture cases are all acyclic,
// so this difference never touches them.
export function run(project: Project, sim: Simulation, injection: Record<string, number>): SimResult {
  const all = arcs(project, sim);
  const incoming = new Map<string, Arc[]>();
  for (const arc of all) {
    incoming.set(arc.to, [...(incoming.get(arc.to) ?? []), arc]);
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

  let settledOk = false;
  for (let sweep = 0; sweep < settleSweeps; sweep++) {
    let settled = true;
    let diverged = false;
    for (const id of ids) {
      let rate = entry[id] ?? 0;
      for (const arc of incoming.get(id) ?? []) {
        rate += (nodes[arc.from] ?? 0) * arc.per;
      }
      if (!Number.isFinite(rate) || Math.abs(rate) > runaway) {
        diverged = true;
        break;
      }
      if (!settledAt(nodes[id], rate)) {
        settled = false;
      }
      next[id] = rate;
    }
    if (diverged) {
      break;
    }
    [nodes, next] = [next, nodes];
    if (settled) {
      settledOk = true;
      break;
    }
  }

  const outNodes: Record<string, number> = {};
  const outEdges: Record<string, number> = {};
  for (const id of ids) {
    outNodes[id] = settledOk ? (nodes[id] ?? 0) : 0;
  }
  for (const arc of all) {
    outEdges[arc.edge] = settledOk ? (outNodes[arc.from] ?? 0) * arc.per : 0;
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
