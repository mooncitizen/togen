---
title: Configuration
description: What togen.yml is for.
---

Configuration is `togen.yml`, at the root of the project, next to `togen/`. It holds:

- `version`
- the `targets` to generate
- the `outDir` they land in
- an optional `style` block: a `theme`, and colours, icons and shapes per node type under `kinds` and per node name under `nodes`
- an optional `usage` block: the expected monthly usage per node name, which [`togen cost`](/togen/guides/cost-estimates/) prices on

`togen init` writes it with both optional blocks commented out. Every key has a default, so a project without the file generates the same as one with `targets: [hcl]` and `outDir: infra`.

The studio reads `togen.yml` and never writes it: it shows what style applies to a selection and the YAML that would override it, for you to paste in yourself.

Configuration used to live in `togen/togen.json`, which is still read, with a deprecation message. `togen init --migrate` rewrites it as `togen.yml`.

For every key, its type, and its default, see [togen.yml](/togen/reference/togen-yml/).
