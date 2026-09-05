# Togen tasks.
#
#   just build            build the canvas and the binary
#   just studio aws-full  run the studio inside an example
#   just                  list everything
#
# Run inside the Nix shell (`nix develop`), or once with `direnv allow` so the
# shell enters it automatically on cd. Recipes run from the repository root
# wherever you invoke them.

set shell := ["bash", "-euo", "pipefail", "-c"]

# Show available recipes.
default:
    @just --list

# gofmt, go vet and the Go unit tests. No Node needed.
check: fmt
    go vet ./...
    go test ./...

# Fail if any Go file needs gofmt.
fmt:
    #!/usr/bin/env bash
    set -euo pipefail
    out="$(gofmt -l .)"
    if [ -n "$out" ]; then echo "gofmt needed:"; echo "$out"; exit 1; fi

# The Go unit tests alone.
test:
    go test ./...

# golangci-lint over every package.
lint:
    golangci-lint run ./...

# Build the canvas and then the binary that embeds it.
build: ui build-cli

# Build the binary alone, embedding whatever internal/server/dist holds.
build-cli:
    go build -o bin/togen ./cmd/togen

# Regenerate schema/ from the Go types.
generate:
    go run ./internal/schema/cmd

# Generate every example and run terraform validate and fmt -check on it.
acceptance: build-cli
    bash scripts/acceptance.sh

# Install the canvas's dependencies from the lockfile.
ui-deps:
    pnpm --dir ui install --frozen-lockfile

# Build the canvas and copy it where the binary embeds it.
ui: ui-deps
    pnpm --dir ui build
    rm -rf internal/server/dist/*
    cp -R ui/dist/. internal/server/dist/

# svelte-check over the canvas.
ui-check: ui-deps
    pnpm --dir ui check

# The canvas component tests, in the chromium the nix shell provides.
ui-test: ui-deps
    pnpm --dir ui test

# Vite with hot reload, proxying /api to a studio running on port 3000.
ui-dev: ui-deps
    pnpm --dir ui dev

# Run the built studio inside an example project, for instance `just studio aws-full 3001`.
studio example='aws-basic' port='3000' *flags='':
    cd examples/{{example}} && ../../bin/togen studio --port {{port}} {{flags}}
