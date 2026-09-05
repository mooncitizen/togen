# Node catalogue

One file per node type. Each says what the node means, what each provider's resolver emits for it, and why the opinions are what they are. When a resolver changes what it emits, the file changes in the same commit.

Providers not yet implemented for a node type say so rather than guessing.

[styles.md](styles.md) is the one file that is not a node type: the colour, icon and shape each provider draws every type with, and which icon set the ids come from.

## Cost

`togen cost` estimates the monthly bill of `togen/project.json` without touching the network. Each node type's file has a Cost paragraph per provider saying which meters it is priced on and what is left out; a node type without one is listed under `not priced` in the output rather than priced at zero. `togen cost --json` prints the same estimate as a document for the studio, and `GET /api/cost` serves it.

Prices come from a snapshot bundled in the binary, `internal/cost/prices/aws.json`: the AWS Price List bulk offer files trimmed to the SKUs the matchers name, one entry per SKU per region for the regions the studio offers. `just refresh-prices` rebuilds it (about 4.5 GB of downloads, streamed, nothing kept on disk) and fails if any matcher matches nothing, or more than one SKU, in any region. The snapshot records the date it was taken and the offer file versions; every estimate prints that date, and one older than 90 days adds a warning line. A weekly workflow runs `just refresh-prices --check`, which fetches and compares without writing, so a price that moved shows up as a failed check rather than a stale number.

The estimate is from on-demand list prices in USD for the project's region, over a 730 hour month, and is never a quote. Free tier, reserved instances and savings plans, support plans, tax, cross-region and internet data transfer are never included. Charges a matcher knows about but cannot size from the project, such as NAT gateway data or backups beyond the allocated storage, are named in the output as not priced.
