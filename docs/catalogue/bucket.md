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

Cost: `togen cost` prices a bucket on three usage keys, `storageGb`, `egressGb` and `requests` (a rate). S3 charges nothing for a bucket that holds nothing, so a bucket with no usage entry has a zero subtotal with the line `priced on defaults, no usage set` under it and its meters under not priced. With an entry, `storageGb` is the `storage` line in GB-months, `egressGb` the `egress` line, and `requests` is split a tenth writes and nine tenths reads, since one figure has to cover two meters and most buckets are read far more than written: the lines are labelled `put requests (1 in 10)` and `get requests (9 in 10)` so the split is on the page. Versioning and public access change nothing on their own; old versions are storage like any other. The meters are S3 Standard: GB-months from the `Storage` family with the `General Purpose` class and `Standard` volume type, the PUT, COPY, POST and LIST (tier 1) and GET (tier 2) request prices from the `API Request` family, on demand, in the project's region, and internet egress from the `AWSDataTransfer` offer file (`AWS Outbound` to `External`, priced from the region it leaves). Storage and egress are tiered and the snapshot holds the first tier's rate with the point where it ends (51,200 GB-months, 10,240 GB); a quantity past that is priced at the first tier's rate and the line says so. The global free allowance of 100 GB egress a month is a row of its own and is not applied. The matcher is `internal/resolve/aws/cost.go`.

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

- `azurerm_storage_account.<name>`, one per bucket, named by the same rule as a function's account: project, environment and node name run together, lowercase alphanumerics only, cut to 24 characters with the project trimmed first. Standard tier, LRS, `min_tls_version = "TLS1_2"`, and a `blob_properties` block with `versioning_enabled` set from `versioning`. Azure keeps versioning as a flag on the account rather than a separate resource, so off is written as `false` rather than left out, and turning it off later is a plan the account can apply.
- `azurerm_storage_container.<name>` in that account, named `<project>-<env>-<name>`, with `container_access_type = "private"`.
- Outputs `<name>_account` and `<name>_container`. The account name is the one a client needs and the truncation rule makes it hard to guess.

One account per bucket rather than one per project because the edges grant on the account: two containers in one account would be readable by every caller granted either one.

A bucket needs no resource group of its own and no network. It joins the project's resource group and is reached over the account's public blob endpoint, `https://<account>.blob.core.windows.net`, which the code builds from `<NAME>_ACCOUNT`.

### public

`public` sets `allow_nested_items_to_be_public = true` on the account and `container_access_type = "blob"` on the container. `blob` is anonymous read of the blobs by name and not of the listing, the same shape as the AWS policy; `container` would open the listing too and nothing asks for that. A private bucket writes the account flag as `false` explicitly, so the result does not depend on which provider major changed the default.

### Edges

A `reads` edge from a function or a service is an `azurerm_role_assignment` of Storage Blob Data Reader to the caller's system-assigned identity, a `writes` edge one of Storage Blob Data Contributor. Contributor covers read, write and delete, so a writer on Azure can also read, which is wider than the AWS put and delete; there is no built-in role for writing without reading, and a custom role is more than this stage wants.

Both are scoped to the storage account, not the container. The account holds this one container and nothing else, so the grant reaches the same blobs either way, and the account's `id` is a resource manager id in every version of the provider where the container's has changed shape between majors. The assignment is named `<caller>_<bucket>_read` or `_write` and carries `principal_type = "ServicePrincipal"`, so it does not wait on directory replication of a fresh identity.

Both set `<NAME>_ACCOUNT` and `<NAME>_CONTAINER` on the caller, as app settings on a function app and container env on a container app. The caller authenticates with its managed identity (`DefaultAzureCredential` in the SDKs) and needs no key or connection string, which is why neither is exported.

Neither edge asks for the virtual network, so a project of functions and buckets creates none.

Not yet: lifecycle management and static websites.
