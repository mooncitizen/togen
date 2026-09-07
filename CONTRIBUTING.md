# Contributing

Thanks for looking. Togen is small and opinionated, so the fastest route to a
merged change is to open an issue first and agree the shape of it.

## Getting a working tree

Everything comes from the nix flake. Do not install Go, Node, pnpm, Terraform or
the linters any other way.

    nix develop

Or once, if you use direnv, so the shell enters it whenever you cd in:

    direnv allow

Tasks are `just` recipes and run from the repository root wherever you invoke
them. `just` on its own lists them.

## The loop

    just build       build the canvas and the binary
    just check       gofmt, go vet and the Go unit tests
    just studio      run the studio inside examples/aws-basic

## Before you push

All of these have to be green. CI runs the same ones.

    just check
    golangci-lint run ./...
    just acceptance
    just ui-check
    just ui-test

`just acceptance` needs terraform, which the nix shell provides. It generates
every example and runs `terraform validate` and `terraform fmt -check` over the
output, so a change that breaks a provider pairing fails here rather than in
someone's project.

If you changed how the studio looks, regenerate the screenshots and commit them:

    just screenshots

CI runs the same script and fails if a shot no longer captures anything, which
catches a renamed label or a restructured panel. It does not compare the images
byte for byte, because the same page rasterises differently on macOS and Linux,
so keeping them current is a human duty rather than a check.

## How changes land

Every change starts from an issue, happens on a branch named
`<type>/<issue>-<slug>`, and lands through a pull request that CI checks and a
maintainer squash-merges. Nothing is committed to `main` directly.

Branch types: `feature`, `fix`, `chore`, `docs`.

## Style

Comments are rare. Write one where the intent is not obvious from the code, and
nowhere else. No doc comment on every function, no parameter lists, no file
header banners. If a comment restates the code, delete it and find a better
name.

Commit messages get a short subject and a body that says what changed, why, and
anything a reviewer would otherwise have to ask.

## Recipes

    just check           gofmt, go vet and the Go unit tests. No Node needed.
    just fmt             fail if any Go file needs gofmt
    just test            the Go unit tests alone
    just lint            golangci-lint over every package
    just build           the canvas and then the binary that embeds it
    just build-cli       the binary alone, embedding whatever internal/server/dist holds
    just require-ui      fail if the canvas is not built; guards release builds
                         against the placeholder
    just generate        regenerate schema/ from the Go types and refresh the
                         embedded copies, examples included
    just refresh-prices  rebuild internal/cost/prices/aws.json from the AWS
                         Price List, or gcp.json with --provider gcp (needs
                         GCP_BILLING_API_KEY) or azure.json with --provider
                         azure. --check compares without writing; a weekly
                         workflow runs it
    just acceptance      generate every example and run terraform validate and
                         fmt -check on it
    just fonts           copy the IBM Plex faces the canvas embeds out of the
                         nix store
    just ui-deps         install the canvas's dependencies: the packages from
                         the lockfile and the fonts from nix
    just ui              build the canvas and copy it where the binary embeds it
    just ui-check        svelte-check over the canvas
    just ui-test         the canvas component tests, in the chromium the nix
                         shell provides
    just ui-dev          the canvas with hot reload, proxying /api to a studio
                         on port 3000. Start that with just studio
    just screenshots     regenerate docs/images from the real studio; depends
                         on a built binary
    just site-deps       install the documentation site's dependencies from
                         the lockfile
    just site            build the documentation site into site/dist
    just site-dev        the documentation site with hot reload
    just site-check      astro check over the documentation site
    just studio          run the studio inside an example project, for
                         instance `just studio aws-full 3001`
    just simulate        show the load an example's simulation puts on every
                         node and edge, for instance `just simulate aws-full --json`
    just cost            estimate an example's monthly cost, priced from its
                         simulation when it has one

`just` on its own lists every recipe. They run from the repository root
wherever you invoke them.

After `just refresh-prices`, accept the new numbers in the golden table with
`go test ./internal/cost -run Golden -update`, and the diff is the review.

## How it is put together

The Go side lives under `internal/`. `ir` is the schema and resource graph,
`resolve/<provider>` turns a project into resources and holds the cost matchers
beside them, `cost` owns the price snapshot and the estimate, `simulate` works
out the load a sketch's sources and bursts put on each node and edge, `emit/hcl`
prints the resources, `workspace` reads and writes the project files and runs
the pipeline, `server` is the studio API, and `cli` ties it together. The canvas
is `ui/`, a Svelte 5 app with its own `package.json`. The documentation site is
`site/`.

`togen studio` serves whatever is in `internal/server/dist/`, embedded at build
time. That directory is empty in a fresh checkout, so a plain `go build` gets
`internal/server/placeholder/` instead, a page that lists the endpoints. Run
`just ui` first, or `just build`, which does both.

## Node types and providers

`docs/catalogue/` holds one file per node type, saying what each provider's
resolver emits for it and why. A change to a resolver changes its catalogue file
in the same commit. The documentation site publishes those files directly, so
there is no second copy to update.

## Licence

By contributing you agree your work is licensed under the Apache License 2.0,
the licence in `LICENSE`.
