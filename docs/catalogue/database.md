# database

A managed relational database that compute nodes read from and write to. Postgres or MySQL. Private, encrypted, credentials owned by the platform.

Properties: engine, version, size, storageGb, highAvailability.

## AWS

- `aws_db_subnet_group` over the private subnets of the implicit VPC. The first node that needs the private network creates that VPC, with a single NAT gateway that costs roughly 30 US dollars a month before data charges. Subnets are spread over the first two availability zones returned by the `aws_availability_zones` data source, so the network adapts to whichever region the project targets rather than hardcoding zone names.
- `aws_security_group` with no rules of its own. Ingress rules come from `reads` and `writes` edges.
- `aws_db_instance` with `manage_master_user_password = true`, so RDS creates and rotates the secret and nothing sensitive lives in state or in the generated code. Storage encrypted, not publicly accessible, seven day backups. `skip_final_snapshot` and `deletion_protection = false` because this is a scaffold and `terraform destroy` should work.
- Output `<name>_endpoint`.

Sizes: small `db.t4g.micro`, medium `db.t4g.medium`, large `db.r6g.large`. `highAvailability` sets `multi_az`.

Engine versions allowed: postgres 17, 16, 15. MySQL 8.4, 8.0. Default is the first. The table lives in `ir.Engines`, keyed by provider, and `just generate` publishes it as `schema/engines.json` for the editor. `ValidateProject` rejects a version the provider does not list, and the AWS resolver checks again on the way through.

A `reads` or `writes` edge from a function adds an ingress rule from the function's security group on the engine port, an IAM statement allowing `secretsmanager:GetSecretValue` on the managed secret, and the env vars `<NAME>_HOST`, `<NAME>_PORT`, `<NAME>_NAME`, `<NAME>_SECRET_ARN` on the function. The function is placed in the VPC.

Cost: `togen cost` prices a database as two meters from the bundled RDS list prices. The instance is 730 hours a month of the `Database Instance` SKU for the size's instance class, the engine, `Single-AZ` or `Multi-AZ` from `highAvailability`, on demand, in the project's region. Storage is `storageGb` GB-months of `General Purpose` (gp2), which is what `aws_db_instance` provisions when no `storage_type` is set, with the same engine and deployment option, so Multi-AZ doubles both meters as AWS bills them. The engine version does not change the price and is shown only in the summary. Backups beyond the allocated size, I/O, snapshot exports and data transfer are not priced, and the output says so. The first node that needs the VPC also brings the NAT gateway's hourly charge under `network`; the data it processes is not priced. The matcher is `internal/resolve/aws/cost.go`, beside the size table, so a change to the sizes and its price consequence are one diff.

## GCP

- `google_sql_database_instance` named `<project>-<environment>-<name>` in the project's region, with `database_version` `POSTGRES_<n>` or `MYSQL_<n>` from the engine table. It has a private address only: `ip_configuration` turns `ipv4_enabled` off and points `private_network` at the implicit VPC, and the instance carries an explicit `depends_on` on the service networking connection, because Terraform cannot see from its arguments that the peering has to exist first. The first node that needs the private network creates the VPC, its subnet, the VPC access connector, the reserved range and the peering. `settings.edition` is `ENTERPRISE`, said explicitly because Cloud SQL defaults Postgres 16 and later to Enterprise Plus, which refuses the shared-core and custom tiers below. `deletion_protection = false`, as on AWS, so `terraform destroy` works.
- `google_sql_database` named after the node with hyphens as underscores, on that instance.
- `random_password`, 32 characters with special characters off, and a `google_sql_user` `app` that takes it. The password lives in Terraform state. On AWS `manage_master_user_password` keeps it in Secrets Manager instead; ADR 0010 defers Secret Manager here until the secrets story is designed. The `random` provider (`hashicorp/random ~> 3.6`) joins `required_providers` the first time a node needs it, once however many databases there are.
- Output `<name>_connection_name`, the `project:region:instance` string the Cloud SQL Auth Proxy takes.

Sizes: small `db-f1-micro` (shared core, 0.6 GB, outside the Cloud SQL SLA the way the AWS micro is burstable), medium `db-custom-2-7680` (2 vCPU, 7.5 GB), large `db-custom-4-15360` (4 vCPU, 15 GB). Custom tiers are `db-custom-<vcpu>-<mb>` with memory a multiple of 256 MB between 0.9 and 6.5 GB per vCPU, and these two are the standard shapes Google documents. `storageGb` is `disk_size` with `disk_autoresize` off, so the size in the sketch is the size on disk and a later apply does not try to shrink a disk Cloud SQL grew. `highAvailability` sets `availability_type` to `REGIONAL` rather than `ZONAL`.

Engine versions allowed: postgres 17, 16, 15 (`POSTGRES_17` and so on), MySQL 8.4, 8.0 (`MYSQL_8_4`, `MYSQL_8_0`). Default is the first. Cloud SQL also offers Postgres 18 and MySQL 9.7 (checked 2026-09-06); they are left out so the table matches the AWS one. `ValidateProject` rejects a version the provider does not list, and the GCP resolver checks again on the way through.

Cloud SQL keeps a deleted instance's name for about a week, so a destroy followed by an apply with the same names waits that long or needs a different environment name.

A `reads` or `writes` edge from a function sets `<NAME>_HOST` (the private address), `<NAME>_PORT`, `<NAME>_NAME`, `<NAME>_USER` and `<NAME>_PASSWORD` on the function and puts it on the VPC access connector. There is no port to open and no role to grant: the instance answers anything on the VPC, and access is the user and password. Reads and writes wire the same way, since one user owns the database. A service will be wired the same way once the GCP resolver has one: a Cloud Run service exports the same handle as a function, so the edge already takes it.

The generated code does not enable APIs. The GCP project needs the Cloud SQL Admin, Service Networking and Serverless VPC Access APIs enabled before the first apply.

Cost: `togen cost` does not price GCP yet, so a database is listed under `not priced`.

## Azure

Not implemented yet. Planned: `azurerm_postgresql_flexible_server` or `azurerm_mysql_flexible_server` plus a database.
