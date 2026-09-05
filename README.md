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

`examples/aws-full` is the larger sketch: every node type, every relation and every property this milestone supports, all in one project. It is what `make acceptance` generates and validates alongside `aws-basic`, so a change that breaks a pairing shows up there rather than in someone's real project. It deploys a gateway in front of three functions and two services, two databases, two queues, two buckets and two caches, wired together with routes, calls, reads, writes, publishes and consumes.

## Develop

- `make check` runs `gofmt`, `go vet` and the unit tests.
- `make generate` regenerates the JSON schema from the Go types.
- `make acceptance` builds the binary, generates every example and runs `terraform validate` and `terraform fmt -check`. Needs terraform on the path, which the nix shell provides.

Everything lives under `internal/`. `ir` is the schema and resource graph, `resolve/aws` turns a project into resources, `emit/hcl` prints them, `cli` ties it together.
