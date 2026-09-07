---
title: Quickstart
description: Generate Terraform from a bundled example in under a minute.
---

The fastest way to see what Togen does is to generate one of the bundled examples.

## Generate the bundled example

```bash
nix develop
just build
cd examples/aws-basic
../../bin/togen generate
```

Output lands in `infra/hcl/`. `terraform init -backend=false && terraform validate` in there should pass.

`terraform plan` needs the deployment package for each function on disk at `functions/<name>.zip` (the path is a variable, so point it wherever your build puts it). `terraform validate` does not.

## Start a project of your own

```bash
togen init --provider aws --name my-project
```

This writes `togen/` and `togen.yml` in the current directory. Every key in `togen.yml` has a default, so a project with nothing set generates the same as one with `targets: [hcl]` and `outDir: infra`. See [Configuration](/togen/guides/configuration/) for what the file holds.

## Open the studio

```bash
togen studio --port 3000
```

This serves the canvas and its JSON API on 127.0.0.1 and opens a browser (`--no-open` if you would rather it did not). From here you sketch the system as boxes and edges rather than editing `togen/project.json` by hand. See [Your first project](/togen/getting-started/your-first-project/) for what you see on first run, and [The studio](/togen/guides/the-studio/) for a tour of the interface.
