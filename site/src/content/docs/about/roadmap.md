---
title: Roadmap
description: What Togen does today, what it does not, and what is deliberately absent.
---

This page is a status report, checked against the code rather than written from memory. No dates: things land when they are ready.

## What is there

Sketching, validating, generating and estimating cost work end to end for all three providers, AWS, GCP and Azure, with the node types and edges documented in [nodes and edges](/togen/guides/nodes-and-edges/). [Simulation](/togen/guides/simulation/) derives load from sources and bursts and feeds it into the cost estimate. The studio (`togen studio`) covers sketching, styling, views, cost and simulation as one interface over the same files the CLI reads and writes.

## Generation targets

Only one is implemented: `internal/emit` has a single package, `hcl`, and it is the only entry in `internal/workspace`'s emitter table. `togen.yml`'s `targets` key accepts a list, and `togen generate --target` documents `hcl`, `pulumi` or `cdktf`, but only `hcl` currently emits anything; asking for the other two fails rather than silently falling back.

## Cost estimates by provider

`togen cost` bundles real list prices for AWS and Azure. Run against `examples/azure-full`, it prices compute, storage and cache down to a total in USD a month, with a `not priced` section for meters like egress and backups that are not modelled yet. AWS is priced the same way; see [cost estimates](/togen/guides/cost-estimates/) for the AWS worked example and what its lines mean.

GCP is not priced yet. Run against `examples/gcp-full`, every node comes back `no gcp prices are bundled yet` and the total is `0.00 USD/month`. The resolver builds GCP resources correctly, `togen generate` and `togen validate` both work against a GCP project; it is only the cost matcher in `internal/resolve/gcp/cost.go` that has not been written.

## What is deliberately absent

Terraform import, bringing existing cloud resources into a Togen sketch, does not exist and is not planned as a near-term feature. Togen generates a starting repository; reconciling it with resources that already exist is a different problem with different failure modes, and folding it in would blur what a sketch means.

There is no telemetry, no account, and no network call at generate time; see [why Togen](/togen/about/why-togen/) for why that is a design choice rather than a gap.

## Everything else

For the file formats and API surface as they exist today, see the [reference](/togen/reference/cli/) section; it is kept in step with the code, not with this page.
