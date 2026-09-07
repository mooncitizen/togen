---
title: Install
description: Get the togen binary onto your machine.
---

Togen is a single binary. There is no account, no service to sign up to, and nothing it needs to phone home to.

## Release binary

Once a release is tagged, GitHub Releases carries a tarball for each platform, named `togen_<version>_<os>_<arch>.tar.gz`, alongside a `checksums.txt`. Download the one for your platform, verify it against the checksums, and extract it:

```bash
tar xzf togen_<version>_<os>_<arch>.tar.gz
```

Put the `togen` binary it contains on your `PATH`. This is the build to use: it embeds the studio's canvas, so `togen studio` serves the real thing.

## go install

```bash
go install github.com/mooncitizen/togen/cmd/togen@latest
```

This builds the CLI, but not the canvas. The studio's UI lives under `internal/server/dist/`, which is built separately by pnpm and is not checked into the repository, so a plain `go build` (or `go install`) has nothing to embed there. `togen studio` still runs: it serves a placeholder page listing the API endpoints instead of the canvas. Use a release binary, or build from source with `just ui` first, if you want the canvas.

## Nix

Every checkout has a `flake.nix`. Run commands through it and you get Go, Node, pnpm and Terraform pinned to the versions the project uses, with nothing installed outside nix:

```bash
git clone git@github.com:mooncitizen/togen.git
cd togen
nix develop
```

## From source

Inside the nix shell, `just build` builds the canvas and the binary that embeds it:

```bash
nix develop
just build
```

The binary lands at `bin/togen`.

## Check it worked

```bash
togen --version
```
