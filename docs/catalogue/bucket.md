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

- `google_storage_bucket.<name>` named `<gcp project id>-<project>-<environment>-<name>`, in the project's region, with `uniform_bucket_level_access` on so access is IAM alone and no object carries an ACL of its own. Cloud Storage names are global, and a GCP project id is itself globally unique, so it goes in front (from `var.project`, the same way the functions' source bucket is named) and the name is predictable before the first apply: no random suffix, no lookup of an output. Project, environment and node names are already lowercase, which bucket names require. Names are capped at 63 characters and a project id can be 30, so the rest is held to 32 characters and a longer one is refused at generate time with the same message as the other name limits.
- `force_destroy = false`, unlike the AWS bucket. Destroying a project with a bucket that still holds objects fails until the bucket is emptied by hand. That is deliberate: a bucket is where the data lives, and the scaffold's convenience should not extend to deleting it.
- `versioning { enabled = true }` when `versioning` is on, which it is by default. Off emits no block at all, the same as on AWS: a new bucket is unversioned already.
- Output `<name>_bucket`, the bucket's name.

A bucket does not need the VPC or the connector.

### public

`public` adds `google_storage_bucket_iam_member.<name>_public` granting `roles/storage.objectViewer` to `allUsers`. That is read of the objects and of the listing, which is what the role carries; there is no way to allow one and not the other with uniform access. An organisation policy enforcing public access prevention overrides it, and the apply then fails on the member rather than silently leaving the bucket private.

A public bucket is world readable by anyone who knows the name, and the name is predictable. Take it for assets, not for anything with a customer in it.

### Edges

A `reads` edge from a function or a service grants the caller's service account `roles/storage.objectViewer` on the bucket (`google_storage_bucket_iam_member.<bucket>_reads_from_<caller>`): get and list, the two things the AWS edge grants as separate statements.

A `writes` edge grants `roles/storage.objectUser` (`google_storage_bucket_iam_member.<bucket>_writes_from_<caller>`): create, overwrite and delete, and also get and list, because the role carries them and there is no narrower predefined role that can delete. A writer on GCP can therefore read, where on AWS it cannot. `roles/storage.objectCreator` alone cannot overwrite an object, which is what most writers do.

Both set `<NAME>_BUCKET` on the caller, where `<NAME>` is the bucket's name upper-snake-cased and the value is the bucket's name, so the code does not have to repeat the naming rule.

Storage answers on its public endpoint, so neither edge pulls the caller onto the connector.

Lifecycle rules and CORS are not supported yet.

Cost: `togen cost` has no GCP prices yet and reports the bucket as not priced.

## Azure

Not implemented yet. Planned: `azurerm_storage_account` and an `azurerm_storage_container` per bucket node, with `container_access_type` set to `blob` for a public one and `private` otherwise.
