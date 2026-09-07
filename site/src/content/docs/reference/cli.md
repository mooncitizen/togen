---
title: CLI
description: Every togen command and every flag, with defaults and an example.
---

`togen --version` prints the build's version and exits. Every other command reads and writes files under the current directory; none of them talk to the network.

## init

Create `togen/` and `togen.yml` in the current directory.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--provider` | `aws` | `aws`, `gcp` or `azure` |
| `--name` | (empty) | project name |
| `--migrate` | `false` | rewrite an old `togen/togen.json` as `togen.yml` |

```bash
togen init --provider aws --name shop
```

`--migrate` is the way off the legacy configuration file. See [togen.yml](/togen/reference/togen-yml/) for what replaced it.

## validate

Check `togen/project.json`.

```bash
togen validate
```

No flags. Exits non-zero and prints one line per problem if the project does not resolve.

## generate

Generate infrastructure code from `togen/project.json`.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--target` | (empty, falls back to `togen.yml`) | `hcl`, `pulumi` or `cdktf` |
| `--out` | (empty, falls back to `togen.yml`) | output directory |
| `--force` | `false` | replace the output directory even if it has files Togen did not write |

```bash
togen generate --target hcl --out infra
```

Only `hcl` is implemented; see [the roadmap](/togen/about/roadmap/) for the other two targets.

## cost

Estimate the monthly cost of `togen/project.json` from bundled list prices.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--json` | `false` | print the estimate as JSON |

```bash
togen cost
```

See [cost estimates](/togen/guides/cost-estimates/) for what each line means and which provider has prices bundled.

## simulate

Show the load `togen/simulation.json` puts on every node and edge.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--json` | `false` | print the rates as JSON |

```bash
togen simulate
```

A project with no `togen/simulation.json` prints a line saying there is no load to simulate rather than a table of zeros. See [simulation](/togen/guides/simulation/) for sources, bursts and fan-out.

## studio

Serve the canvas on 127.0.0.1.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--port` | `3000` | port to listen on, `0` for any free port |
| `--no-open` | `false` | do not open a browser |

```bash
togen studio --port 3000
```

See [the studio](/togen/guides/the-studio/) for a tour, and [the studio API](/togen/reference/api/) for scripting against it directly.
