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

`togen studio --port 3000` serves the canvas and its JSON API on 127.0.0.1 and opens a browser (`--no-open` if you would rather it did not). The studio is a top bar naming the project, its provider, region and environment, with the status pill and the Generate button on the right; a rail on the left with the views list and the palette; the canvas; and an inspector on the right that opens on selection. It is dark by default and `style.theme` in `togen.yml` picks `dark`, `light` or `system` (the OS setting); the toggle in the top bar overrides that for the session. Drag a type from the palette onto the canvas to add a node, and drag nodes around to arrange them: both save to `togen/`, and a project with no saved positions is laid out left to right on first load. Click a node to open the inspector on the right and set its name and properties; edits save shortly after you stop typing. Its Style tab shows the colour, icon and shape the node is drawn with, where each comes from (a node override, a kind override or the provider scheme) and the `togen.yml` snippets that would override them, each with a copy button. Drag from the right of one node to the left of another to connect them: one legal relation is applied straight away, several offer a small menu, and a pair the relation table has nothing for is refused with the reason. Click an edge to set a route's path and methods, and delete the selected node or edge from the inspector or with the delete key while the canvas has focus. Nodes and palette tiles carry the provider's own architecture icons (AWS, Google Cloud and Azure), bundled under each vendor's terms, which About in the rail footer lists. The studio starts with or without a project: in a directory with no `togen/`, `GET /api/project` answers 404 until `POST /api/project/init` creates one, from `{name, provider, region, environment}` as `togen init` would or from a bundled example (`{example: "aws-basic"}`; `GET /api/examples` lists them). It refuses with 409 when `togen/` is already there and leaves an existing `togen.yml` alone. The canvas also draws the network the resolver will create (the VPC, VNet or VPC network, and on Azure the resource group around everything) as a dashed boundary round the nodes that need it, labelled implicit because it is worked out from the view and never stored. Export in the top bar renders the open view to a PNG or JPEG in the browser from the same drawing the canvas shows, boundary, labels and icons included, at 1x, 2x or 3x, on the theme's ground, on white (which takes the light palette), or for PNG on nothing at all, with the view's name in the corner unless you turn it off; the dialog previews the picture, states its size in pixels, and saves the file through the browser as `<project>-<view>.png` or `.jpg`. The studio starts with or without a project. In a directory with no `togen/` it opens on a first-run screen that names the directory and offers three choices: a new sketch (project name, environment, provider with its palette, and region, then an empty canvas), one of the bundled examples, or importing existing Terraform, which is drawn but comes later. Underneath, `GET /api/project` answers 404 until `POST /api/project/init` creates one, from `{name, provider, region, environment}` as `togen init` would or from a bundled example (`{example: "aws-basic"}`; `GET /api/examples` lists them, `GET /api/workspace` names the directory).

Configuration is `togen.yml` at the root of the project, next to `togen/`. It holds `version`, the `targets` to generate, the `outDir` they land in, an optional `style` block (a `theme`, colours, icons and shapes per node type under `kinds` and per node name under `nodes`) and an optional `usage` block, the expected monthly usage per node name that `togen cost` prices on. `togen init` writes it with both blocks commented out. Every key has a default, so a project without the file generates the same as one with `targets: [hcl]` and `outDir: infra`. The studio reads it and never writes it: it shows what style applies to a selection and the YAML that would override it, for you to paste. Configuration used to live in `togen/togen.json`, which is still read with a deprecation message; `togen init --migrate` rewrites it as `togen.yml`.

`togen/views.json` names the views of a project: each has an id, a name and the node ids it shows, or `"*"` for all of them, and every project has `overview`. A project without the file has the overview alone, and neither `togen validate` nor `togen generate` reads it. `togen/layout.json` is version 2, with the positions and viewport of each view under `views` by view id; a version 1 file is read as the overview and written back as version 2 on the studio's next save. The API serves them at `/api/views`. In the studio the rail lists the views with how many nodes each shows; click one to open it, the plus button to add one, and the pencil to edit it in place of the inspector: rename it, tick the nodes it shows (grouped by type, with the rest dimmed on the canvas while you choose), or delete it, except the overview, which always shows everything. The canvas names the open view and counts its nodes, draws an edge only when both ends are in the view, and saves positions and the viewport per view, placing a node that has none by auto-layout. Undo and redo cover creating, renaming, deleting and changing the membership of a view.

`togen cost` prints a monthly estimate of the project from a snapshot of list prices bundled in the binary, so it works offline like everything else. Databases, services (Fargate tasks, and the load balancer of a public or routed one) and caches are priced on their fixed meters, and the implicit network brings the NAT gateway. What a node does is the `usage` block in `togen.yml`, keyed by node name, and each node type takes a fixed set of keys: a gateway `requests`; a function `invocations` and `durationMs`; a queue `messages`; a bucket `storageGb`, `egressGb` and `requests`; a service `egressGb`; and the implicit network, under `network`, `natGb`. The rates (`requests`, `invocations`, `messages`) are a number per period, `500/min`, `20k/day` or `2M/month`, normalised to a 730 hour month and rounded to a whole count; the rest are plain numbers, gigabytes a month or milliseconds a run. Each figure becomes a line at the snapshot's rate: Lambda requests and GB-seconds (invocations by run time by the size's memory), API Gateway requests, SQS requests at three a message, S3 GB-months, requests (a tenth as writes) and internet egress, a service's egress and its load balancer's capacity units on that egress, and the NAT gateway's data. A node with no entry is priced on defaults and says so: `priced on defaults, no usage set` under it, with the meters it did not price under `not priced`, so a zero is never mistaken for free. A line whose quantity outgrows the first price tier says so beneath it. A key that is not the node type's, or a node the project has not got, fails `togen validate`, `togen cost` and the studio's config check with the path (`usage.api.invocations: a gateway takes requests only`). Unit prices print to four places, or as many as a rate below a hundredth of a cent needs, so a Lambda request reads as `0.0000002`. A node type with no matcher yet is listed under `not priced` rather than silently counted as free, and the last line says which snapshot the prices came from. `--json` prints the same as a document. For `examples/aws-basic`, whose `togen.yml` sets usage for the gateway, one function, the bucket and the service:

```
api        gateway       HTTP API
  requests        21900000 requests  x    0.00000116 USD    25.40
                                                            25.40
orders     function      node, 512 MB, x86_64
  requests         2000000 requests  x     0.0000002 USD     0.40
  duration          300000 GB-s      x  0.0000166667 USD     5.00
                                                             5.40
orders-db  database      db.t4g.micro, postgres 17, single-AZ, 20 GB
  instance             730 h         x        0.0180 USD    13.14
  storage gp2           20 GB        x        0.1330 USD     2.66
                                                            15.80
web        service       0.25 vCPU, 0.5 GB, 1 task, public
  vcpu               182.5 vCPU-h    x        0.0466 USD     8.50
  memory               365 GB-h      x        0.0051 USD     1.87
  load balancer        730 h         x        0.0265 USD    19.32
  egress               100 GB        x        0.0900 USD     9.00
  capacity units       100 LCU-h     x        0.0084 USD     0.84
                                                            39.53
jobs       queue         standard
  priced on defaults, no usage set
                                                             0.00
worker     function      node, 512 MB, x86_64
  priced on defaults, no usage set
                                                             0.00
uploads    bucket        standard storage, versioned, private
  storage              200 GB        x        0.0240 USD     4.80
  egress                40 GB        x        0.0900 USD     3.60
                                                             8.40
sessions   cache         cache.t4g.micro, redis 7.1, 1 node
  node                 730 h         x        0.0180 USD    13.14
                                                            13.14
network    implicit VPC
  priced on defaults, no usage set
  nat gateway          730 h         x        0.0500 USD    36.50
                                                            36.50
not priced
  api        gateway       data transfer
  orders-db  database      backups beyond 20 GB
  jobs       queue         requests (3 per message)
  worker     function      requests
  worker     function      duration
  uploads    bucket        put requests (1 in 10)
  uploads    bucket        get requests (9 in 10)
  network    implicit VPC  nat gateway data

total  144.17 USD/month  eu-west-2, list prices from 2026-09-06, estimate not a quote
```

`examples/aws-full` is the larger sketch: every node type, every relation and every property this milestone supports, all in one project. It is what `just acceptance` generates and validates alongside `aws-basic`, so a change that breaks a pairing shows up there rather than in someone's real project. It deploys a gateway in front of three functions and two services, two databases, two queues, two buckets and two caches, wired together with routes, calls, reads, writes, publishes and consumes.

`examples/gcp-full` is the same sketch as `aws-full` on Google Cloud: the same nodes, properties and edges, so what one provider does with a pairing can be read beside the other. `web` calling the private `admin` service is the one that shows Cloud Run's rule: the caller's egress all goes through the VPC, which brings a Cloud NAT with it. `just acceptance` validates it with the `google` and `random` providers, and `togen cost` lists every node under not priced until GCP prices are bundled.

`examples/azure-basic` is the first Azure sketch: a gateway routing to a function that reads a database, publishes to a queue and writes to a bucket, which becomes a resource group, a Linux Function App on a consumption plan with its storage account, a virtual network with a Postgres flexible server on a delegated subnet behind a private DNS zone, an administrator password from the `random` provider, a Service Bus namespace with a queue the app is a Data Sender on, a second storage account with a private container the app is a Blob Data Contributor on, and outputs carrying the app's URL, the server's FQDN, the namespace and the container's account. `just acceptance` validates it with the `azurerm` and `random` providers alongside the AWS examples, and it grows as the Azure resolver does.

`examples/azure-full` is `aws-full` on Azure: the same fourteen nodes and twenty-four edges, with the three differences the Azure resolver asks for. The MySQL version is `8.0.21`, the one Azure Database for MySQL offers, the `fifo` queue moves the Service Bus namespace to the Standard tier, since Basic has no sessions, and the mailer calls the admin service rather than the worker, because a `calls` URL on Azure is a Terraform reference to the target and the AWS loop of calls would be a cycle. `just acceptance` generates and validates it with the other four, and `togen cost` runs on it with every node under `not priced` until the Azure matchers land.

## Develop

- `just check` runs `gofmt`, `go vet` and the unit tests. No Node needed.
- `just generate` regenerates the JSON schema and the data files under `schema/` from the Go types, and refreshes the copies the binary embeds, the examples included.
- `just acceptance` builds the binary, generates every example, runs `togen cost` in it, and runs `terraform validate` and `terraform fmt -check`. Needs terraform on the path, which the nix shell provides.
- `just refresh-prices` rebuilds `internal/cost/prices/aws.json` from the AWS Price List, streaming about 4.5 GB, and fails if a matcher no longer picks exactly one SKU in some region. `--check` compares without writing; a weekly workflow runs it. After a refresh, `go test ./internal/cost -run Golden -update` accepts the new numbers in the golden table and the diff is the review.
- `just ui` builds the canvas and copies it into `internal/server/dist/`, which is what `just build` embeds.
- `just ui-test` runs the component tests in a headless chromium that the nix shell provides.
- `just ui-check` runs `svelte-check` over the canvas.
- `just ui-dev` serves the canvas with hot reload and proxies `/api` to a studio running on port 3000; start that one with `just studio`, or `just studio aws-full 3001` for the other example on another port.
- `just` on its own lists every recipe. They run from the repository root wherever you are.

The canvas imports `schema/project.schema.json` at build time, so the palette lists whatever node types the schema has and the inspector builds each node's form from that type's properties: toggles, selects, numbers, text and a key value editor for `env`, with the schema description as help text and the default as the placeholder. Database versions come from `schema/engines.json` for the project's provider, and `schema/styles.json` is the built-in style per provider, the colour, icon id and shape of every node type, listed in `docs/catalogue/styles.md`. Only what you set is written, so a field left empty stays out of the file. Connections consult `schema/relations.json`, and every change is validated in the browser with ajv against the same schema and the same rules as the CLI: the top bar counts the problems and expands to the lines the CLI would print, and the node each one names gets a red badge, or the edge a red stroke. It talks to the studio over `/api` and reloads when the websocket says a file changed on disk. Generate writes the target files and lists them in the bar, or marks the nodes and edges the studio refused. Undo and redo, the buttons or Mod+Z and Shift+Mod+Z outside a text field, cover changes to the project, not the layout. Database versions come from `schema/engines.json` for the project's provider, and `schema/regions.json` lists each provider's regions with their display names, the default first.

The Go side lives under `internal/`. `ir` is the schema and resource graph, `resolve/aws` turns a project into resources and holds the cost matchers beside them, `cost` owns the price snapshot, the lookup and the estimate, `emit/hcl` prints the resources, `workspace` reads and writes the project files, validates `togen.yml` against `schema/togen.schema.json` and runs the pipeline, `server` is the studio API, and `cli` ties it together. The canvas is `ui/`, a Svelte 5 app with its own `package.json`.

`togen studio` serves whatever is in `internal/server/dist/`, embedded at build time. That directory is empty in a fresh checkout, so a plain `go build` gets `internal/server/placeholder/` instead, a page that lists the endpoints.
