---
title: Simulation
description: Describing the traffic a sketch carries, and what togen simulate prints.
---

A simulation is a description of the traffic a sketch carries: one or more sources injecting requests at a gateway, service or function, an optional burst laid over a source for part of the month, and the fan-out on any edge that is not a straight one call in for one call out. It lives at `togen/simulation.json`, next to `project.json`, and the studio writes it as you edit traffic on the canvas. A project with no file has no simulation and behaves as it always did.

## Sources, bursts and fan-out

A source names the node it enters at and a rate, `800/min`, `40k/day` or `2M/month`. A burst multiplies one source for part of the month, a launch or a sale, and is priced whether or not it is the scenario the canvas is currently showing. Fan-out is the calls an edge makes per call in, keyed by edge id: a cache with an 80% hit rate reads through at `0.8`, a queue that only a fifth of writes reach fans out at `0.2`. Left unset, an edge fans out at `1`.

`examples/aws-full/togen/simulation.json` is the worked example:

```json
{
  "version": 1,
  "sources": [
    { "id": "web", "name": "Web traffic", "target": "gateway-1", "rate": "800/min", "bytesPerRequest": 4096 },
    { "id": "admin", "name": "Admin console", "target": "service-2", "rate": "40/min" }
  ],
  "bursts": [
    { "id": "launch", "name": "Product launch", "source": "web", "multiplier": 4, "minutes": 30, "timesPerMonth": 2 }
  ],
  "edges": {
    "edge-9": 0.8,
    "edge-12": 0.2,
    "edge-14": 0.3,
    "edge-18": 0.5,
    "edge-22": 0.5
  }
}
```

`edge-12` and `edge-22` are not there for realism alone: `orders` publishing to `jobs` and `orders` calling `mailer` sit on the project's two loops (through `worker` and `web` back to `orders`), and at the default fan-out of 1 both loops amplify without limit. Damping `edge-12` to 0.2 and `edge-22` to 0.5 is what makes the sweep settle at all; the rest of the fan-outs above just describe cache hit rates and how much of each request reaches a downstream edge.

## In the studio

Editing traffic on the canvas, adding a source, dragging a burst's multiplier, setting an edge's fan-out in its inspector, writes straight to `togen/simulation.json`. With a simulation present, the top bar grows a scenario selector next to the cost pill, Baseline plus one entry per burst, and the canvas draws each node and edge's rate for whichever scenario is selected. A loop that never settles is reported rather than drawn as zeros: the traffic still amplifying is named, and a loop that is only slowly converging is shown with a note that the figures are still low. See [the studio](/togen/guides/the-studio/) for the rest of the layout.

## Feeding togen cost

When `togen/simulation.json` is there, `togen cost` prices every node's usage from the rates the simulation works out, rather than from defaults. A hand-written `usage` block in `togen.yml` still wins field by field: a simulated `requests` figure is only a starting point, and setting `requests` by hand for one node overrides just that key, leaving the rest of that node's usage, and every other node's, to the simulation. See [cost estimates](/togen/guides/cost-estimates/) for what each figure prices to.

## togen simulate

`togen simulate` prints the rates the sweep settles on, per node and per edge, as a table, or with `--json`, a document. Run against `examples/aws-full`:

```
nodes
  api          gateway         35184000 /month  requests
  orders       function       234559999 /month  invocations
  worker       function       164191999 /month  invocations
  mailer       function       117279999 /month  invocations
  web          service        199375999 /month  requests
  admin        service        296124799 /month  requests
  orders-db    database       633311997 /month
  reports-db   database       592249597 /month
  jobs         queue           46912000 /month  messages
  events       queue           59812800 /month  messages
  uploads      bucket         398751998 /month  requests
  assets       bucket         395812798 /month  requests
  sessions     cache          159500799 /month
  rate-limits  cache          398751998 /month

edges
  api routes orders  x1        35184000 /month
  api routes web  x1        35184000 /month
  api routes admin  x1        35184000 /month
  orders reads orders-db  x1       234559999 /month
  orders writes orders-db  x1       234559999 /month
  worker reads orders-db  x1       164191999 /month
  admin reads reports-db  x1       296124799 /month
  admin writes reports-db  x1       296124799 /month
  web reads sessions  x0.8       159500799 /month
  orders writes rate-limits  x1       234559999 /month
  worker reads rate-limits  x1       164191999 /month
  orders publishes jobs  x0.2        46912000 /month
  worker consumes jobs  x1        46912000 /month
  web publishes events  x0.3        59812800 /month
  admin consumes events  x1        59812800 /month
  orders writes uploads  x1       234559999 /month
  worker reads uploads  x1       164191999 /month
  web reads assets  x0.5        99687999 /month
  admin writes assets  x1       296124799 /month
  web calls orders  x1       199375999 /month
  worker calls web  x1       164191999 /month
  orders calls mailer  x0.5       117279999 /month
  web calls admin  x1       199375999 /month
  mailer calls worker  x1       117279999 /month
```

A database or a cache carries a rate for the canvas, `reads` and `writes` together, but neither takes a usage key: `togen cost` has no traffic meter for either, so their rate does not feed a price. Run against a project with no `togen/simulation.json`, such as `examples/aws-basic`, it prints `this project has no togen/simulation.json, so there is no load to simulate` instead of a table of zeros.

If the fan-out round a loop leaves at least as much traffic coming back as going in, the sweep never settles. `togen simulate` refuses rather than print a rate that would keep climbing, naming the nodes on the loop and which edge's fan-out to lower.
