# Togen

Sketch a system as boxes and edges, get a starting infrastructure repo. Local, deterministic, no accounts needed to generate.

Status: milestone one. AWS, Terraform HCL, seven node types.

## Try it

```bash
nix develop
make build
cd examples/aws-basic
../../bin/togen generate
```

Output lands in `infra/hcl/`. `terraform init -backend=false && terraform validate` in there should pass.

`terraform plan` needs the deployment package for each function on disk at `functions/<name>.zip` (the path is a variable, so point it wherever your build puts it). `terraform validate` does not.

`togen studio --port 3000` serves the canvas and its JSON API on 127.0.0.1 and opens a browser (`--no-open` if you would rather it did not). The canvas is a blank Svelte Flow board for now; the palette, inspector and editing land later.

`examples/aws-full` is the larger sketch: every node type, every relation and every property this milestone supports, all in one project. It is what `make acceptance` generates and validates alongside `aws-basic`, so a change that breaks a pairing shows up there rather than in someone's real project. It deploys a gateway in front of three functions and two services, two databases, two queues, two buckets and two caches, wired together with routes, calls, reads, writes, publishes and consumes.

## Develop

- `make check` runs `gofmt`, `go vet` and the unit tests. No Node needed.
- `make generate` regenerates the JSON schema from the Go types.
- `make acceptance` builds the binary, generates every example and runs `terraform validate` and `terraform fmt -check`. Needs terraform on the path, which the nix shell provides.
- `make ui` builds the canvas and copies it into `internal/server/dist/`, which is what `make build` embeds.
- `make ui-test` runs the component tests in a headless chromium that the nix shell provides.
- `make ui-check` runs `svelte-check` over the canvas.
- `pnpm --dir ui dev` serves the canvas with hot reload and proxies `/api` to a `togen studio --port 3000` running alongside it.

The Go side lives under `internal/`. `ir` is the schema and resource graph, `resolve/aws` turns a project into resources, `emit/hcl` prints them, `workspace` reads and writes the `togen/` files and runs the pipeline, `server` is the studio API, and `cli` ties it together. The canvas is `ui/`, a Svelte 5 app with its own `package.json`.

`togen studio` serves whatever is in `internal/server/dist/`, embedded at build time. That directory is empty in a fresh checkout, so a plain `go build` gets `internal/server/placeholder/` instead, a page that lists the endpoints.
