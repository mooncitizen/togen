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

## Node types and providers

`docs/catalogue/` holds one file per node type, saying what each provider's
resolver emits for it and why. A change to a resolver changes its catalogue file
in the same commit. The documentation site publishes those files directly, so
there is no second copy to update.

## Licence

By contributing you agree your work is licensed under the Apache License 2.0,
the licence in `LICENSE`.
