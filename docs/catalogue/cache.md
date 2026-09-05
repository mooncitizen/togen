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

Sizes: small `cache.t4g.micro`, medium `cache.t4g.medium`, large `cache.r7g.large`.

### Edges

`reads` and `writes` do the same thing. Redis has no read only mode worth modelling here, and a cache that a caller can read but not fill is not much of a cache.

Either one adds an ingress rule on the cache's security group allowing TCP 6379 from the caller's security group, and sets `<NAME>_HOST` and `<NAME>_PORT` on the caller, where `<NAME>` is the cache's name upper-snake-cased. Both edges between the same pair produce one rule and one pair of env vars.

There is no IAM statement, because ElastiCache Redis has no IAM to speak of. Access is the network and nothing else.

A cache is reached over the private network, so an edge to one pulls a function into the VPC exactly as a database edge does: the function gets a security group, a `vpc_config` over the private subnets, and the VPC access execution role. A project of functions and caches pays for the NAT gateway.

## GCP

Not implemented yet. Planned: `google_redis_instance` with `connect_mode` `PRIVATE_SERVICE_ACCESS` over the service networking connection the database already needs, sized by `memory_size_gb` rather than by node type.

## Azure

Not implemented yet. Planned: `azurerm_redis_cache`, where the size is a capacity, family and SKU that have to be a valid combination rather than three independent choices.
