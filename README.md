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

`togen studio --port 3000` serves the canvas and its JSON API on 127.0.0.1 and opens a browser (`--no-open` if you would rather it did not). The studio is a top bar naming the project, its provider, region and environment, with the status pill and the Generate button on the right; a rail on the left with the views list and the palette; the canvas; and an inspector on the right that opens on selection. It is dark by default and `style.theme` in `togen.yml` picks `dark`, `light` or `system` (the OS setting); the toggle in the top bar overrides that for the session. Drag a type from the palette onto the canvas to add a node, and drag nodes around to arrange them: both save to `togen/`, and a project with no saved positions is laid out left to right on first load. Click a node to open the inspector on the right and set its name and properties; edits save shortly after you stop typing. Drag from the right of one node to the left of another to connect them: one legal relation is applied straight away, several offer a small menu, and a pair the relation table has nothing for is refused with the reason. Click an edge to set a route's path and methods, and delete the selected node or edge from the inspector or with the delete key while the canvas has focus. Nodes and palette tiles carry the provider's own architecture icons (AWS, Google Cloud and Azure), bundled under each vendor's terms, which About in the rail footer lists.

Configuration is `togen.yml` at the root of the project, next to `togen/`. It holds `version`, the `targets` to generate, the `outDir` they land in, and an optional `style` block: a `theme`, colours, icons and shapes per node type under `kinds` and per node name under `nodes`. `togen init` writes it with the style block commented out. Every key has a default, so a project without the file generates the same as one with `targets: [hcl]` and `outDir: infra`. The studio reads it and never writes it: it shows what style applies to a selection and the YAML that would override it, for you to paste. Configuration used to live in `togen/togen.json`, which is still read with a deprecation message; `togen init --migrate` rewrites it as `togen.yml`.

`togen/views.json` names the views of a project: each has an id, a name and the node ids it shows, or `"*"` for all of them, and every project has `overview`. A project without the file has the overview alone, and neither `togen validate` nor `togen generate` reads it. `togen/layout.json` is version 2, with the positions and viewport of each view under `views` by view id; a version 1 file is read as the overview and written back as version 2 on the studio's next save. The studio draws the overview and keeps the other views' entries through a save; the API serves them at `/api/views`.

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

The canvas imports `schema/project.schema.json` at build time, so the palette lists whatever node types the schema has and the inspector builds each node's form from that type's properties: toggles, selects, numbers, text and a key value editor for `env`, with the schema description as help text and the default as the placeholder. Database versions come from `schema/engines.json` for the project's provider, and `schema/styles.json` is the built-in style per provider, the colour, icon id and shape of every node type, listed in `docs/catalogue/styles.md`. Only what you set is written, so a field left empty stays out of the file. Connections consult `schema/relations.json`, and every change is validated in the browser with ajv against the same schema and the same rules as the CLI: the top bar counts the problems and expands to the lines the CLI would print, and the node each one names gets a red badge, or the edge a red stroke. It talks to the studio over `/api` and reloads when the websocket says a file changed on disk. Generate writes the target files and lists them in the bar, or marks the nodes and edges the studio refused. Undo and redo, the buttons or Mod+Z and Shift+Mod+Z outside a text field, cover changes to the project, not the layout.

The Go side lives under `internal/`. `ir` is the schema and resource graph, `resolve/aws` turns a project into resources, `emit/hcl` prints them, `workspace` reads and writes the project files, validates `togen.yml` against `schema/togen.schema.json` and runs the pipeline, `server` is the studio API, and `cli` ties it together. The canvas is `ui/`, a Svelte 5 app with its own `package.json`.

`togen studio` serves whatever is in `internal/server/dist/`, embedded at build time. That directory is empty in a fresh checkout, so a plain `go build` gets `internal/server/placeholder/` instead, a page that lists the endpoints.
