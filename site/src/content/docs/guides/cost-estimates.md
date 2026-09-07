---
title: Cost estimates
description: What togen cost prices, and where the numbers come from.
---

`togen cost` prints a monthly estimate of the project from a snapshot of list prices bundled in the binary, so it works offline like everything else.

## What gets priced

On AWS, the databases, services (Fargate tasks, and the load balancer of a public or routed one) and caches are priced on their fixed meters, and the implicit network brings the NAT gateway. On Azure it is the flexible servers' compute and storage, the Container Apps vCPU and memory of the replicas at `minReplicas`, and the cache's node hours; there is no NAT gateway and the virtual network is free, so the one implicit charge is the Service Bus Standard base unit a `fifo` queue brings.

## Usage keys

What a node does is the `usage` block in `togen.yml`, keyed by node name. Each node type takes a fixed set of keys:

- a gateway: `requests`
- a function: `invocations` and `durationMs`
- a queue: `messages`
- a bucket: `storageGb`, `egressGb` and `requests`
- a service: `egressGb` and `requests`
- the implicit network, under `network`: `natGb`

A key a provider's matchers have no meter for prices nothing there: a service's `requests` is a Container Apps meter on Azure and nothing on AWS, and a gateway's `requests` the other way round.

The rates (`requests`, `invocations`, `messages`) are a number per period, `500/min`, `20k/day` or `2M/month`, normalised to a 730 hour month and rounded to a whole count. The rest are plain numbers: gigabytes a month, or milliseconds a run.

## How a figure becomes a line

Each figure becomes a line at the snapshot's rate:

- Lambda requests and GB-seconds (invocations by run time by the size's memory)
- API Gateway requests
- SQS requests at three a message
- S3 GB-months, requests (a tenth as writes), and internet egress
- a service's egress and its load balancer's capacity units on that egress
- the NAT gateway's data

On Azure: Functions executions and GB-seconds, Service Bus operations at three a message, blob GB-months and operations on the same tenth split, Container Apps requests, and the Bandwidth egress meter.

A node with no usage entry is priced on defaults and says so: `priced on defaults, no usage set` under it, with the meters it did not price listed under `not priced`, so a zero is never mistaken for free. A line whose quantity outgrows the first price tier says so beneath it.

## Validation

A key that is not the node type's, or a node the project has not got, fails `togen validate`, `togen cost` and the studio's config check, with the path: `usage.api.invocations: a gateway takes requests only`.

## Reading the numbers

Unit prices print to four places, or as many as a rate below a hundredth of a cent needs, so a Lambda request reads as `0.0000002`. A node type with no matcher yet is listed under `not priced` rather than silently counted as free, and the last line says which snapshot the prices came from.

All three providers have matchers for every node type, but only the AWS and Azure snapshots are bundled: the GCP price list needs a Cloud Billing Catalog API key to build, so until someone with one runs the refresh, a GCP project prices nothing and the last line says no GCP prices are bundled yet.

`--json` prints the same as a document.

## A worked example

![The cost panel in the studio, showing a project's estimate](/togen/images/cost-panel.png)

`examples/aws-basic`, whose `togen.yml` sets usage for the gateway, one function, the bucket and the service, prices at 144.17 USD a month:

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

The last line is always a reminder: this is an estimate against list prices, not a quote.
