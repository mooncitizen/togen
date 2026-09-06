# service

A long running container. Always on, listening on a port, restarted when it dies.

Properties: image, port, size, minReplicas, maxReplicas, public, env.

## AWS

- `aws_ecs_cluster` named after the project and environment, one per project, created by the first service. Container Insights is on, because a cluster with no metrics is not worth having.
- `aws_cloudwatch_log_group` at `/ecs/<service name>` with 30 day retention, created explicitly so the retention is set rather than unlimited.
- Two roles, because ECS uses them at different moments. `aws_iam_role.<name>_execution` is what the agent assumes to pull the image and write logs, and carries `AmazonECSTaskExecutionRolePolicy`. `aws_iam_role.<name>` is the task role, which is what the code inside the container assumes, and is where edge permissions land. Keeping them apart means a compromised container cannot use the pull credentials.
- `aws_security_group` with an allow-all egress rule. A service always runs in the VPC, so unlike a function it gets one whether or not any edge asks. The first node that needs the private network creates the VPC, with a single NAT gateway that costs roughly 30 US dollars a month before data charges.
- `aws_ecs_task_definition` on Fargate with `network_mode = "awsvpc"`. The container definition carries the image, the port mapping, the environment and an `awslogs` driver pointed at the log group. Environment variables come from the node's `env` plus whatever edges add, sorted by name so the JSON does not churn between runs.
- `aws_ecs_service` with `launch_type = "FARGATE"`, `desired_count` from `minReplicas`, running on the private subnets with no public IP.

`maxReplicas` is read and then ignored. It is there for the autoscaling policies a later milestone will add, and until then the service sits at `minReplicas`.

### Public services

Only a service with `public` set gets a load balancer of its own. An ALB is about 20 US dollars a month before traffic, so a service that is only called from inside the VPC does not pay for one, and its callers reach it over the private network. A private service that a gateway routes to is the exception: it gets the same load balancer with `internal = true`, on the private subnets, reachable only through the gateway's VPC link rather than from the internet. It is still not given a URL output, because the gateway is the way in. See [gateway](gateway.md).

When `public` is set the resolver adds `aws_lb` on the public subnets, `aws_lb_target_group` with `target_type = "ip"` because Fargate tasks are addressed by IP rather than by instance, `aws_lb_listener` on port 80 forwarding to that group, and an `aws_security_group` for the load balancer that accepts port 80 from anywhere. The service's own security group then accepts the container port from the load balancer's, so nothing else in the VPC can reach it. The service depends on the listener, since ECS refuses to register targets against a group no listener uses. The URL is emitted as an output named `<name>_url`.

The health check path is `/` and cannot be configured yet. A service that answers 404 there will never come into service, so either serve something at the root or wait for the health check property.

HTTP only for now. HTTPS needs a certificate, and a certificate needs a domain the user owns, which is a decision Togen does not make for them yet.

Sizes are Fargate cpu and memory pairs: small 256/512, medium 1024/2048, large 2048/4096. Fargate only accepts fixed pairs, so these are not free choices.

### Edges

A `reads` or `writes` edge to a database opens the engine port on the database's security group from the service's, injects the host, port, name and secret ARN as environment variables, and puts a `secretsmanager:GetSecretValue` statement on the task role. Statements from all edges are collected into one `aws_iam_role_policy`.

A `calls` edge to a service needs a stable private address, and a private service has no load balancer to give one, so the target is registered in Cloud Map. The first call in the project creates `aws_service_discovery_private_dns_namespace.main` at `<project>-<environment>.local`, one per project. Each called service then gets an `aws_service_discovery_service` named after the node, and a `service_registries` block on its `aws_ecs_service` so ECS registers and deregisters task IPs as they come and go. The block also tells Cloud Map that ECS reports health, so a stopped task drops out of DNS. The target's security group is opened on its container port from the caller's and the caller gets `<TARGET>_URL`, which is `http://<name>.<project>-<environment>.local:<port>`.

A public service is called by that same private name. Sending internal traffic out to the load balancer and back would cross the internet and pay for the trip, for a service the caller can already reach directly.

A `calls` edge to a function grants `lambda:InvokeFunction` on the target and injects `<TARGET>_FUNCTION_NAME`. Nothing is opened on the network, because Lambda is invoked over the AWS API.

Cost: `togen cost` prices a service on the Fargate meters from the bundled ECS list prices: vCPU-hours and GB-hours for the size's cpu and memory pair, times `minReplicas`, over a 730 hour month, since the service sits at `minReplicas` until autoscaling arrives. The task definition sets no `runtime_platform`, so the tasks run on x86 and those are the rates used, not the cheaper ARM ones. A public service adds 730 hours of its application load balancer to the same block. A private service that a gateway routes to gets the same load balancer, but the node alone cannot tell, so its hours appear as a separate `implicit ALB` block named after the service, beside the network. A private service nobody routes to has no load balancer line, which is the point of not giving it one. Load balancer capacity units, which is where traffic is charged, are named as not priced. The first node that needs the VPC also brings the NAT gateway's hourly charge under `network`. The matcher is `internal/resolve/aws/cost.go`, beside the size table, so a change to the pairs and its price consequence are one diff.

## GCP

- `google_service_account` with `account_id` `<project>-<environment>-<name>`. The service runs as it, and it is what `calls` edges grant to. Account ids are capped at 30 characters, so a long project, environment and name combination is refused with a message saying which to shorten.
- `google_cloud_run_v2_service` in the project's region with `deletion_protection` off, so `terraform destroy` works without a second edit. The template runs as the service account, carries a `scaling` block with `min_instance_count` from `minReplicas` and `max_instance_count` from `maxReplicas`, and one container with the `image`, a `ports` block for `port`, cpu and memory limits from the size, and an `env` block per variable: the node's `env` first, sorted by name, then whatever edges add. Cloud Run scales the service itself, so unlike AWS both replica bounds take effect. A service with `minReplicas` of 1 or more is billed for that many idle instances.
- `ingress` is `INGRESS_TRAFFIC_INTERNAL_ONLY` unless `public`, when it is `INGRESS_TRAFFIC_ALL`. There is no load balancer either way. Cloud Run gives every service an HTTPS URL of its own.
- If any edge gives the service access to something on the private network, the template gains a `vpc_access` block naming the implicit network's connector with `PRIVATE_RANGES_ONLY` egress, and the first node that needs the network creates it. A `reads` or `writes` edge to a database does that. A service with no such edge stays off the connector.

Sizes are Cloud Run cpu and memory limits: small 1 cpu and 512Mi, medium 1 cpu and 1Gi, large 2 cpu and 2Gi. Cloud Run wants whole vCPUs above 1 and at least 512Mi per vCPU beyond the first, so these are the pairs that line up with the function's memory table.

The generated code does not enable APIs. The GCP project needs Cloud Run enabled before the first apply, and the image has to be somewhere Cloud Run can pull from (Artifact Registry, or a public registry).

### Public services

`public` adds a `google_cloud_run_v2_service_iam_member` granting `roles/run.invoker` to `allUsers` on the service, and an output `<name>_url` carrying the service's URL. A private service has no binding and no output; its URL still reaches callers inside the project through the `calls` edge.

### Edges

A `routes` edge from the gateway opens the service to the internet and outputs its URL as `<gateway>_<name>_url`, the same shape as a routed function; see [gateway](gateway.md). A public service already answers to `allUsers`, so a route to one adds the output alone rather than a second binding on the same service.

A `reads` or `writes` edge to a database sets the connection details on the container and puts the service on the connector, described under [database](database.md).

A `calls` edge to a service grants the caller's service account `roles/run.invoker` on the target and sets `<TARGET>_URL` on the caller. A `calls` edge to a function does the same on the function's Cloud Run service, since a 2nd gen function answers HTTP through Cloud Run and that is where `run.invoker` is checked. Callers are functions or services, and each caller gets its own binding named `<target>_from_<caller>`. The caller still has to send an identity token for the target's URL with each request (the Google auth libraries fetch one from the metadata server), because Cloud Run checks the token, not the network.

Two things to know before relying on that. Cloud Run only counts a request from another Cloud Run service as internal when the caller routes all of its egress through the VPC, and the resolver puts callers on the connector with `PRIVATE_RANGES_ONLY` egress, which sends `run.app` traffic over the public path. So a call to a private service is refused at the ingress today, and the target has to be `public` (its binding still matters: `allUsers` opens the URL, the caller's token is what an application should check) until the resolver routes caller egress through the VPC. And the URL is Terraform's reference to the target, so two services that call each other are a cycle Terraform refuses; a `calls` loop needs one side to find the other some other way.

Cost: `togen cost` has no GCP prices yet and reports the service as not priced.

## Azure

- `azurerm_log_analytics_workspace.main`, `PerGB2018` with 30 day retention, one per project, created by the first service. Container Apps sends its console and system logs there, and 30 days matches the AWS log group.
- `azurerm_container_app_environment.main`, one per project, created by the same first service. It sits on the implicit virtual network's `apps` subnet (`10.0.0.0/23`, delegated to `Microsoft.App/environments`), so the apps in it can reach the flexible servers on their private zones, and it declares the `Consumption` workload profile, which bills per second of cpu and memory while a replica runs and nothing when scaled to zero.
- `azurerm_container_app` in the resource group every Azure project gets, in that environment, on the `Consumption` profile, with `revision_mode = "Single"` and a system assigned identity for the role assignments edges will add. The template carries `min_replicas` and `max_replicas` from the node and one container: the image, the cpu and memory pair for the size, and the node's `env` plus whatever edges add, as `env` blocks in the order they were set. Container Apps scales between the two counts on HTTP concurrency out of the box, so unlike AWS `maxReplicas` does something. The ingress has `target_port` from `port`, `external_enabled` from `public`, and sends all traffic to the latest revision, since revisions and traffic splitting are not in the catalogue.

Sizes are the pairs the Consumption profile accepts: small 0.25 cpu and 0.5Gi, medium 0.5 and 1Gi, large 1 and 2Gi. Anything else is refused by Azure at apply, so these are not free choices.

A public service gets an output `<name>_url`, `https://` plus the ingress FQDN. A private service's ingress is internal: the same FQDN answers only from inside the environment, and there is no output.

HTTPS comes for free on the environment's own domain, which is why the URL is not `http://` as on AWS.

### Edges

A `routes` edge from a gateway makes the ingress external whatever `public` says, because on Azure the gateway is nothing but the target's own hostname (ADR 0010) and a route to an ingress the internet cannot reach would be a route to nothing. The gateway URL output is `<gateway>_<name>_url`, the same ingress FQDN, and the routes go in as `<GATEWAY>_ROUTES` in the container env, described under [gateway](gateway.md).

A `reads` or `writes` edge to a database sets `<NAME>_HOST`, `<NAME>_PORT`, `<NAME>_NAME`, `<NAME>_USER` and `<NAME>_PASSWORD` in the container env, described under [database](database.md). The environment already sits on the network the server answers from, so a service reaches it where a consumption plan function cannot.

A `calls` edge to a service sets `<TARGET>_URL` on the caller, `https://` plus the target's ingress FQDN. For a private service that is the internal FQDN, which a container app in the same environment resolves and a function app on the consumption plan does not, so a function calling a private service is marked as needing the network the same way the database edge marks it. A public service is called by its public FQDN. A `calls` edge to a function sets `<TARGET>_URL` to `https://` plus the function app's default hostname. Nothing else is created: everything on Azure answers HTTPS on its own name.

Cost is not priced yet.
