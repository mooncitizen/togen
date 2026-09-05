# Togen

Sketch a system as boxes and edges, get a starting infrastructure repo. Local, deterministic, no accounts needed to generate.

Status: milestone one. AWS, Terraform HCL, seven node types.

## Try it

```bash
nix develop
just build
cd examples/aws-basic
../../bin/togen generate
```

Output lands in `infra/hcl/`. `terraform init -backend=false && terraform validate` in there should pass.

`terraform plan` needs the deployment package for each function on disk at `functions/<name>.zip` (the path is a variable, so point it wherever your build puts it). `terraform validate` does not.

`togen studio --port 3000` serves the canvas and its JSON API on 127.0.0.1 and opens a browser (`--no-open` if you would rather it did not). Drag a type from the palette onto the canvas to add a node, and drag nodes around to arrange them: both save to `togen/`, and a project with no saved positions is laid out left to right on first load. Click a node to open the inspector on the right and set its name and properties; edits save shortly after you stop typing. Edge drawing lands later.

`examples/aws-full` is the larger sketch: every node type, every relation and every property this milestone supports, all in one project. It is what `just acceptance` generates and validates alongside `aws-basic`, so a change that breaks a pairing shows up there rather than in someone's real project. It deploys a gateway in front of three functions and two services, two databases, two queues, two buckets and two caches, wired together with routes, calls, reads, writes, publishes and consumes.

## Develop

- `just check` runs `gofmt`, `go vet` and the unit tests. No Node needed.
- `just generate` regenerates the JSON schema from the Go types.
- `just acceptance` builds the binary, generates every example and runs `terraform validate` and `terraform fmt -check`. Needs terraform on the path, which the nix shell provides.
- `just ui` builds the canvas and copies it into `internal/server/dist/`, which is what `just build` embeds.
- `just ui-test` runs the component tests in a headless chromium that the nix shell provides.
- `just ui-check` runs `svelte-check` over the canvas.
- `just ui-dev` serves the canvas with hot reload and proxies `/api` to a studio running on port 3000; start that one with `just studio`, or `just studio aws-full 3001` for the other example on another port.
- `just` on its own lists every recipe. They run from the repository root wherever you are.

The canvas imports `schema/project.schema.json` at build time, so the palette lists whatever node types the schema has and the inspector builds each node's form from that type's properties: toggles, selects, numbers, text and a key value editor for `env`, with the schema description as help text and the default as the placeholder. Database versions come from `schema/engines.json` for the project's provider. Only what you set is written, so a field left empty stays out of the file. It talks to the studio over `/api` and reloads when the websocket says a file changed on disk.

The Go side lives under `internal/`. `ir` is the schema and resource graph, `resolve/aws` turns a project into resources, `emit/hcl` prints them, `workspace` reads and writes the `togen/` files and runs the pipeline, `server` is the studio API, and `cli` ties it together. The canvas is `ui/`, a Svelte 5 app with its own `package.json`.

`togen studio` serves whatever is in `internal/server/dist/`, embedded at build time. That directory is empty in a fresh checkout, so a plain `go build` gets `internal/server/placeholder/` instead, a page that lists the endpoints.
