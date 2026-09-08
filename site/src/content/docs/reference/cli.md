---
title: CLI
description: Every togen command and every flag, with defaults and an example.
---

Every command reads and writes files under the current directory. Most of them also make one request a day to check for a new release; see [upgrading](/togen/getting-started/upgrading/) for that check and for what `togen upgrade` does.

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

## version

Print the version, how it was built and how it was installed.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--json` | `false` | print the version as JSON |

```bash
togen version
```

See [upgrading](/togen/getting-started/upgrading/) for what each field means.

## upgrade

Replace this binary with the latest release.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--check` | `false` | report the latest release without installing it |
| `--yes` | `false` | do not ask before replacing the binary |
| `--version` | (empty, falls back to the latest release) | install this tag instead |

```bash
togen upgrade
```

What happens depends on how togen was installed: a Homebrew or Nix install refuses and names the command to use instead. See [upgrading](/togen/getting-started/upgrading/) for the full behaviour and the daily check that suggests it.
