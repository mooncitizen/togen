# cache

An in-memory key value store on the private network. Somewhere to put sessions, rate limit counters, or the answer to a query nobody wants to run twice. Redis, and only Redis: a cache node is a cache, not a choice of engine.

A cache is not durable storage. Everything in it can be gone after a failover or a restart, so nothing that matters should live there alone.

Properties: size.

## AWS

- `aws_elasticache_subnet_group.<name>` over the private subnets of the implicit VPC. The first node that needs the private network creates that VPC, with a single NAT gateway that costs roughly 30 US dollars a month before data charges.
- `aws_security_group.<name>` with no rules of its own. Ingress rules come from `reads` and `writes` edges.
- `aws_elasticache_replication_group.<name>` running Redis 7.1 with `num_cache_clusters = 1` on port 6379. One node, so `automatic_failover_enabled` is off: failing over wants a second cluster in another availability zone, and a scaffold should not be paying for one before anybody has asked. Turning it on later is a property away, and a cache that loses its contents is doing what a cache does.
- `at_rest_encryption_enabled` and `transit_encryption_enabled` are both on. Transit encryption is the one that shows up in your code: the endpoint speaks TLS, so a client has to be told to use it. `rediss://` rather than `redis://` for a URL, `tls: {}` for node-redis, `ssl=True` for redis-py. A plain connection is refused rather than downgraded, which is a confusing five minutes if you were not expecting it.
- No auth token. Encryption in transit allows one, but a token is a secret that has to be generated, stored and handed to the caller, and there is nowhere to put it that is better than the security group. The security group is the boundary: only the callers with an edge can reach the port at all.
- `apply_immediately = true`, so a change to the node type or the version happens on apply rather than waiting for the next maintenance window. Fine for a scaffold, worth a second thought once real traffic is on it.
- Output `<name>_endpoint`, the primary endpoint address.

`replication_group_id` is capped at 40 characters, which is the tightest limit AWS puts on anything Togen names. The generated name is `<project>-<environment>-<node>`, so a long project name leaves little room for the cache. The resolver reports it against the node rather than letting AWS reject the apply.

Sizes: small `cache.t4g.micro`, medium `cache.t4g.medium`, large `cache.r7g.large`. In me-south-1 and me-central-1, which offer no t4g cache nodes, small and medium are `cache.t3.micro` and `cache.t3.medium`.

### Edges

`reads` and `writes` do the same thing. Redis has no read only mode worth modelling here, and a cache that a caller can read but not fill is not much of a cache.

Either one adds an ingress rule on the cache's security group allowing TCP 6379 from the caller's security group, and sets `<NAME>_HOST` and `<NAME>_PORT` on the caller, where `<NAME>` is the cache's name upper-snake-cased. Both edges between the same pair produce one rule and one pair of env vars.

There is no IAM statement, because ElastiCache Redis has no IAM to speak of. Access is the network and nothing else.

A cache is reached over the private network, so an edge to one pulls a function into the VPC exactly as a database edge does: the function gets a security group, a `vpc_config` over the private subnets, and the VPC access execution role. A project of functions and caches pays for the NAT gateway.

Cost: `togen cost` prices a cache as one meter from the bundled ElastiCache list prices: 730 hours a month of the `Cache Instance` SKU for the size's node type, Redis, on demand, in the project's region, times the one node that `num_cache_clusters` sets, so turning failover on later doubles it. The plain node hour is the one picked, not the extended support rates AWS lists for the same node type. Backups are off, so nothing is left unpriced beyond the data transfer the catalogue README rules out. The first node that needs the VPC also brings the NAT gateway's hourly charge under `network`. The matcher is `internal/resolve/aws/cost.go`, beside the size table and the node count, so a change to either and its price consequence are one diff.

## GCP

Not implemented yet. Planned: `google_redis_instance` with `connect_mode` `PRIVATE_SERVICE_ACCESS` over the service networking connection the database already needs, sized by `memory_size_gb` rather than by node type.

## Azure

- `azurerm_redis_cache.<name>` in the resource group every Azure project gets, named `<project>-<environment>-<name>`. No network: the cache answers on its public hostname, and only a caller holding the access key gets in. Private endpoints are not modelled yet.
- The non-SSL port is off and `minimum_tls_version` is `1.2`, so the endpoint speaks TLS on 6380 and nothing else. As on AWS the client has to be told: `rediss://`, `tls: {}` for node-redis, `ssl=True` for redis-py.
- `redis_version` is `6`. Version 4 is retired and the provider will not create a new cache on it.
- Output `<name>_hostname`.

The name is capped at 63 characters, which the resolver reports against the node rather than letting Azure reject the apply.

Sizes are one valid `capacity`, `family` and `sku_name` triple each, because the provider accepts C0 to C6 and P1 to P5 and nothing in between. Small is Basic C0 (250 MB, one node, no SLA). Medium is Standard C1 (1 GB) and large Standard C3 (6 GB), both with a replica. Premium and the P family are not offered: they bring clustering and virtual network injection, which are out of scope here.

### Edges

`reads` and `writes` do the same thing, as on AWS. Either one sets `<NAME>_HOST`, `<NAME>_PORT` and `<NAME>_PASSWORD` on the caller, where `<NAME>` is the cache's name upper-snake-cased: the hostname, the SSL port, and the primary access key. On a function they are app settings, on a service container env. Both edges between the same pair set the three once.

The port is the resource's `ssl_port` attribute rather than a literal, so it is a number Terraform converts to a string in the settings.

The access key sits in the app settings until the secrets story lands, exactly as the database password does. There is no role assignment: Azure Cache for Redis has no data plane role to grant, access is the key.
