# bucket

Object storage. Somewhere to put files that outlive the process that wrote them: uploads, exports, static assets.

Properties: versioning, public.

## AWS

- `aws_s3_bucket.<name>` with `bucket_prefix` rather than `bucket`. S3 names are global, so a fixed name collides the second time anyone generates the same sketch, in another account or another environment. Terraform appends a unique suffix to the prefix instead, which is why the generated name is an output rather than something you can predict. `force_destroy = true` because this is a scaffold and `terraform destroy` should work without emptying the bucket by hand.
- `aws_s3_bucket_versioning.<name>` when `versioning` is on, which it is by default. Off emits nothing at all: a new bucket is unversioned already, and `Suspended` on a bucket that was never versioned is a state nobody asked for.
- `aws_s3_bucket_server_side_encryption_configuration.<name>` with `AES256`, so objects are encrypted at rest without anyone having to own a KMS key. Same trade as the queue's `sqs_managed_sse_enabled`.
- `aws_s3_bucket_public_access_block.<name>` with all four flags on for a private bucket.
- Output `<name>_bucket`, the generated name.

Every resource after the bucket refers to it by `id`, so Terraform orders them behind it without a `depends_on`.

### public

`public` does not touch the ACL flags. `block_public_acls` and `ignore_public_acls` stay on either way, because ACLs are the old way of sharing a bucket, they are off by default on new buckets, and nothing here needs them. What changes is `block_public_policy` and `restrict_public_buckets`, which come off so a policy is allowed to grant anonymous access.

That policy is `aws_s3_bucket_policy.<name>`, one statement allowing `s3:GetObject` to `*` on `<arn>/*`. Read of the objects, not of the listing, so the bucket is not a public directory. It `depends_on` the public access block: AWS rejects a public policy while the block is still in force, and without the dependency Terraform is free to apply them the wrong way round.

A public bucket is world readable by anyone who knows the name. Take it for assets, not for anything with a customer in it.

### Edges

A `reads` edge from a function or a service grants `s3:GetObject` on `<arn>/*` and `s3:ListBucket` on the bucket itself. Two statements because the object actions and the bucket actions take different resources, and code that lists a prefix fails confusingly without the second one.

A `writes` edge grants `s3:PutObject` and `s3:DeleteObject` on `<arn>/*`. Overwriting is a put, so a writer needs no more than this. Delete is there because a writer that cannot tidy up leaves the bucket to grow forever.

Both set `<NAME>_BUCKET` on the caller, where `<NAME>` is the bucket's name upper-snake-cased. That is the generated name, which the code has no other way of knowing.

A bucket is reached over the public S3 endpoint, so neither edge pulls the caller into the VPC and neither creates a security group. A project of functions and buckets pays for no NAT gateway.

Cost: `togen cost` prices a bucket at rest. S3 charges nothing for a bucket that holds nothing, so the node has a zero subtotal with the line `priced at rest, usage not set` under it, and `storage`, `requests` and `egress` are listed under not priced until the `usage` block in `togen.yml` sets them. Versioning and public access change nothing on their own; old versions are storage like any other. The S3 Standard meters are in the bundled snapshot ready for that: GB-months from the `Storage` family with the `General Purpose` class and `Standard` volume type, and the PUT, COPY, POST and LIST (tier 1) and GET (tier 2) request prices from the `API Request` family, on demand, in the project's region. Storage is tiered and the snapshot holds the first tier's rate with the point where it ends. Egress is an EC2 data transfer charge rather than the bucket's and stays unpriced. The matcher is `internal/resolve/aws/cost.go`.

## GCP

Not implemented yet. Planned: `google_storage_bucket` with uniform bucket level access, versioning on the `versioning` block, and public reads through an IAM member granting `roles/storage.objectViewer` to `allUsers` rather than a bucket policy.

## Azure

Not implemented yet. Planned: `azurerm_storage_account` and an `azurerm_storage_container` per bucket node, with `container_access_type` set to `blob` for a public one and `private` otherwise.
