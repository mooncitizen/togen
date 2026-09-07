---
title: How it is built
description: The pieces Togen is made of, and why the compilation happens in two stages.
---

Togen is a Go binary with an embedded Svelte canvas. There is no separate service to run and nothing to deploy: `togen studio` serves the UI and its API from the one binary, on `127.0.0.1`.

## The two-stage compilation

A sketch does not go straight to Terraform. It goes through a resource graph first: `internal/ir` defines both the sketch's schema (nodes, edges, properties) and the graph a resolver produces from it (providers, roles, security groups, the wiring an edge implies). `internal/resolve/aws`, `internal/resolve/gcp` and `internal/resolve/azure` each turn a validated sketch into that graph for their provider, and each also holds the cost matchers under `internal/resolve/<provider>/cost.go` that price the resources they know how to build. `internal/emit/hcl` then prints a resource graph as HCL; it has never seen a sketch, only resources.

Splitting the work this way is what keeps three providers from meaning three unrelated code generators. A `calls` edge from a function to a service means one thing in the sketch and three different sets of resources depending on the resolver, but by the time `internal/emit/hcl` runs, it is looking at resources and outputs, not at edges or providers. Adding a fourth target, Pulumi or CDKTF, means writing an emitter against the same graph; it does not mean touching the resolvers, and a bug in how AWS wires a VPC cannot leak into how GCP prices a function.

`internal/cost` owns the list price snapshot and turns a resolved graph plus a usage document into the estimate `togen cost` prints. `internal/simulate` is upstream of that: it takes `togen/simulation.json` and works out the rate every node and edge carries, which `togen cost` then prices instead of the defaults when a simulation exists. `internal/workspace` is the layer above all of it: it reads and writes the files under `togen/` and `togen.yml`, runs the validate and generate pipeline end to end, and is what both the CLI and the studio call into, so the two never disagree about what a project means.

`internal/server` is the studio's HTTP API, a thin layer over `internal/workspace` (see [the API reference](/togen/reference/api/) for its routes), and `internal/cli` is the layer Cobra commands call into, mostly `internal/workspace` and `internal/server` themselves. Neither owns logic the other lacks; they are the two ways in.

## The canvas

The studio's frontend is `ui/`, a Svelte 5 app: the diagram editor, the inspector panels, the cost and simulation views. It talks to `internal/server`'s API and nothing else, the same API documented at [the studio API reference](/togen/reference/api/). At build time it is compiled and embedded into the Go binary (`go:embed all:dist` in `internal/server`), so `togen studio` has no assets to find on disk and no version skew between the binary and the UI it serves.

## The site

This documentation is `site/`, a Starlight (Astro) site built separately from the Go binary and published from the same repository.

## Why trust the split

The two-stage compilation is also why a Terraform reviewer does not need to trust Togen's sketch format to trust its output: the HCL that lands on disk is a direct print of an explicit resource graph, not a template with sketch data spliced in. Reading `internal/resolve/<provider>` tells you exactly what a node and an edge become; reading `internal/emit/hcl` tells you exactly how a resource becomes text. Neither step is hidden behind the other.
