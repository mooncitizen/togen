---
title: togen.yml
description: Every key in togen.yml, its type, and its default.
---

See [configuration](/togen/guides/configuration/) for what the file is for. This page is the key by key reference, taken from `schema/togen.schema.json`.

Every key has a default. A project with no `togen.yml` at all generates the same as one with `targets: [hcl]` and `outDir: infra`.

## version

Integer, the file format version. A `togen.yml` with a version newer than the running Togen understands is refused.

## targets

List of strings, the code generators to run. Only `hcl` is implemented today; see [the roadmap](/togen/about/roadmap/) for `pulumi` and `cdktf`. Default `[hcl]`.

## outDir

String, the directory generated code is written to. Default `infra`.

## style

How the studio draws the diagram. All of it is optional.

```yaml
style:
  theme: dark
  kinds:
    function:
      color: "#7c5cff"
      icon: aws/lambda
      shape: card
  nodes:
    orders:
      color: "#ff8800"
```

- `theme`: `dark`, `light` or `system`. Default `dark`.
- `kinds`: style for every node of a type, keyed by `service`, `function`, `database`, `gateway`, `queue`, `bucket` or `cache`.
- `nodes`: style for one node, keyed by its name.

Both `kinds` and `nodes` entries take the same three fields, and a `nodes` entry wins over a `kinds` entry for the same node:

- `color`: fill colour as `#rrggbb`.
- `icon`: a bundled icon id such as `aws/rds`, or a path relative to `togen.yml` starting `./` or `../`.
- `shape`: `card`, `cylinder`, `hexagon` or `circle`.

The studio reads this block and never writes it: it shows the YAML that would apply a style, for you to paste in yourself. See [nodes and edges](/togen/guides/nodes-and-edges/) for the node types themselves.

## usage

Expected monthly usage, by node name, for [`togen cost`](/togen/guides/cost-estimates/) to price. A node with no entry here is priced on its own defaults, or on the rates a [simulation](/togen/guides/simulation/) works out when `togen/simulation.json` exists; setting a key by hand overrides just that key for that node.

Each node type takes only the keys that mean something for it:

| Node type | Keys |
| --- | --- |
| `gateway` | `requests` |
| `function` | `invocations`, `durationMs` |
| `queue` | `messages` |
| `bucket` | `storageGb`, `egressGb`, `requests` |
| `service` | `egressGb`, `requests` |

`database` and `cache` take no usage key: neither has a traffic-based meter to price against.

The network's NAT gateway is not a node, so its usage lives under the name `network`:

```yaml
usage:
  network:
    natGb: 500
```

- `requests`, `invocations`, `messages`: a rate, `500/min`, `20k/day` or `2M/month`. The number takes an optional `k` or `M` suffix and the period is `min`, `hour`, `day` or `month`.
- `durationMs`: number, milliseconds one run of a function takes.
- `storageGb`, `egressGb`, `natGb`: number, gigabytes.

## Migrating from togen.json

Configuration used to live in `togen/togen.json`. It is still read, with a deprecation message on every run, but a project should move off it: `togen init --migrate` rewrites `togen/togen.json` as `togen.yml` at the project root. See [the CLI reference](/togen/reference/cli/#init) for the flag.
