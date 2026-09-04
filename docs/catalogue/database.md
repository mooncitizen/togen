# database

A managed relational database that compute nodes read from and write to. Postgres or MySQL. Private, encrypted, credentials owned by the platform.

Properties: engine, version, size, storageGb, highAvailability.

## AWS

- `aws_db_subnet_group` over the private subnets of the implicit VPC. The first node that needs the private network creates that VPC, with a single NAT gateway that costs roughly 30 US dollars a month before data charges. Subnets are spread over the first two availability zones returned by the `aws_availability_zones` data source, so the network adapts to whichever region the project targets rather than hardcoding zone names.
- `aws_security_group` with no rules of its own. Ingress rules come from `reads` and `writes` edges.
- `aws_db_instance` with `manage_master_user_password = true`, so RDS creates and rotates the secret and nothing sensitive lives in state or in the generated code. Storage encrypted, not publicly accessible, seven day backups. `skip_final_snapshot` and `deletion_protection = false` because this is a scaffold and `terraform destroy` should work.
- Output `<name>_endpoint`.

Sizes: small `db.t4g.micro`, medium `db.t4g.medium`, large `db.r6g.large`. `highAvailability` sets `multi_az`.

Engine versions allowed: postgres 17, 16, 15. MySQL 8.4, 8.0. Default is the first. The table lives in `ir.Engines`, keyed by provider, and `make generate` publishes it as `schema/engines.json` for the editor. `ValidateProject` rejects a version the provider does not list, and the AWS resolver checks again on the way through.

A `reads` or `writes` edge from a function adds an ingress rule from the function's security group on the engine port, an IAM statement allowing `secretsmanager:GetSecretValue` on the managed secret, and the env vars `<NAME>_HOST`, `<NAME>_PORT`, `<NAME>_NAME`, `<NAME>_SECRET_ARN` on the function. The function is placed in the VPC.

## GCP

Not implemented yet. Planned: `google_sql_database_instance` with private IP over a service networking connection, plus database and user.

## Azure

Not implemented yet. Planned: `azurerm_postgresql_flexible_server` or `azurerm_mysql_flexible_server` plus a database.
