---
title: Providers
description: What the bundled examples show about AWS, Google Cloud and Azure.
---

Togen's examples cover the same sketches on different providers, so what one provider does with a pairing can be read beside the other. Provider-specific resource behaviour for each node type is in the [node catalogue](/togen/reference/catalogue/); this page is about the examples themselves.

## AWS

`examples/aws-full` is the larger AWS sketch: every node type, every relation and every property this milestone supports, all in one project. It is what `just acceptance` generates and validates alongside `aws-basic`, so a change that breaks a pairing shows up there rather than in someone's real project. It deploys a gateway in front of three functions and two services, two databases, two queues, two buckets and two caches, wired together with routes, calls, reads, writes, publishes and consumes.

## Google Cloud

`examples/gcp-full` is the same sketch as `aws-full` on Google Cloud: the same nodes, properties and edges.

![The same sketch drawn on the GCP canvas](/togen/images/canvas-gcp.png)

`web` calling the private `admin` service is the one that shows Cloud Run's rule: the caller's egress all goes through the VPC, which brings a Cloud NAT with it. `just acceptance` validates it with the `google` and `random` providers, and `togen cost` reports no GCP prices bundled until someone with a Cloud Billing Catalog API key runs the refresh.

## Azure

`examples/azure-basic` is the first Azure sketch: a gateway routing to a function that reads a database, publishes to a queue and writes to a bucket. This becomes a resource group, a Linux Function App on a consumption plan with its storage account, a virtual network with a Postgres flexible server on a delegated subnet behind a private DNS zone, an administrator password from the `random` provider, a Service Bus namespace with a queue the app is a Data Sender on, a second storage account with a private container the app is a Blob Data Contributor on, and outputs carrying the app's URL, the server's FQDN, the namespace and the container's account. `just acceptance` validates it with the `azurerm` and `random` providers alongside the AWS examples, and it grows as the Azure resolver does.

![The same sketch drawn on the Azure canvas](/togen/images/canvas-azure.png)

`examples/azure-full` is `aws-full` on Azure: the same fourteen nodes and twenty-four edges. `diff examples/aws-full/togen/project.json examples/azure-full/togen/project.json` shows the provider, the region and one sketch difference, the MySQL version, which is `8.0.21` because that is the one Azure Database for MySQL offers.

The `events` queue carries `fifo` in both examples, and on Azure that is what moves the Service Bus namespace to the Standard tier, since Basic has no sessions.

`just acceptance` generates and validates it with the other five, and `togen cost` prices it: 912.79 USD a month in `uksouth`, most of it the zone redundant Postgres server and the large cache's two Standard C3 nodes.
