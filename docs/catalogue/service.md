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

## GCP

Not implemented yet. Planned: `google_cloud_run_v2_service` with a service account, and for a public service a `google_cloud_run_v2_service_iam_member` granting `roles/run.invoker` to `allUsers` rather than a load balancer.

## Azure

Not implemented yet. Planned: `azurerm_container_app` on a container app environment, with an `ingress` block and `external_enabled` for a public service. Consumption profile only accepts fixed cpu and memory pairs, so sizes map to those.
