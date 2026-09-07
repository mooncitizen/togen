---
title: Why Togen
description: The idea behind Togen, and what it deliberately is not.
---

Most infrastructure starts as a picture before it starts as code: boxes for the pieces, arrows for how they talk. Togen takes that picture seriously. You sketch a system as nodes and edges, a gateway, some functions, a database, a queue, and Togen resolves it into a provider's own idea of that system, the roles, the security groups, the environment variables an edge implies, and emits it as Terraform you own from the moment it lands on disk.

The sketch is not the infrastructure. It is an intermediate description that a resolver turns into a resource graph, and the resource graph is what gets printed as HCL. That middle step is what lets the same sketch mean something coherent on AWS, GCP or Azure: a `calls` edge from a function to a service is a Cloud Map DNS registration and a security group rule on AWS, a `roles/run.invoker` grant and a Cloud Run URL on GCP. You draw the relationship once; the provider's own shape for it is Togen's job, not yours.

Everything runs on your machine. `togen generate` reads a file and writes files. `togen studio` opens a local server on `127.0.0.1` and nothing else. There is no account, no telemetry, no call out to a service to resolve a sketch or price it: [cost estimates](/togen/guides/cost-estimates/) come from a bundled snapshot of list prices, not a live lookup.

## What it is not

Togen is not a deployment tool. It does not run `terraform apply`, hold state, or touch a cloud account. What it produces is a starting Terraform repository, reviewed and applied the way any other Terraform is.

It is not a runtime. Nothing Togen writes depends on Togen being present afterwards, beyond the one-line comment naming where the file came from.

It is not a wrapper you keep installed. Generate once, delete the sketch, and the output still applies. See [how it is built](/togen/about/design/) for the shape of the two-stage compilation that makes that true, and [the roadmap](/togen/about/roadmap/) for what is not built yet.
