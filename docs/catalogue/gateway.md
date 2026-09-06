# gateway

The public HTTP entry point. A project has at most one. Routes are edges from the gateway to a function or service, each with a path and methods.

Properties: none.

## AWS

- `aws_apigatewayv2_api` with `protocol_type = "HTTP"`.
- `aws_apigatewayv2_stage` named `$default` with `auto_deploy = true`, so route changes go live on apply.
- Output `<name>_url`.

A `routes` edge to a function adds an `AWS_PROXY` integration with payload format 2.0, one `aws_apigatewayv2_route` per method with key `<METHOD> <path>`, and an `aws_lambda_permission` letting the API invoke the function. The permission is scoped to the whole API rather than one route, which is the same thing while a function has a single integration.

### Routes to a service

A service listens on the private network, so the API needs a way in. The first routed service in a project creates `aws_apigatewayv2_vpc_link.main` on the private subnets, with its own security group and an allow-all egress rule. One link serves every routed service, because a link takes a few minutes to create and costs nothing to share.

The route then targets an `HTTP_PROXY` integration with `connection_type = "VPC_LINK"` and payload format 1.0, whose `integration_uri` is the service's `aws_lb_listener` ARN, and the load balancer's security group is opened on port 80 from the link's. A service that is not `public` has no load balancer of its own, so being routed gives it an internal one on the private subnets. Everything else about it matches a public service's, described under [service](service.md).

Routing stays in API Gateway. Paths and methods are already routes there, so listener rules on the load balancer would be a second place to say the same thing, and a project that routes to both a function and a service would have no single URL to hand out. The gateway is the entry point in every case: `<name>_url` is the API endpoint whether the project routes to functions, services or both, and a public service keeps its own `<name>_url` as a second, direct way in.

The path is passed through to the service as it stands. A route on `/web` arrives at the container as `/web`, so the service has to serve that prefix.

Cost: `togen cost` prices a gateway on one usage key, `requests`, a rate. An HTTP API has no hourly charge, so a gateway with no usage entry has a zero subtotal with the line `priced on defaults, no usage set` under it and `requests` under not priced. With an entry, the requests a month (500/min is 21,900,000) become the `requests` line on the `API Calls` family of the `AmazonApiGateway` offer file with the `ApiGatewayHttpRequest` usage type, on demand, in the project's region, which is the HTTP API's rate and about a third of the REST API's. It is tiered and the snapshot holds the first tier's rate with the point where it ends (300,000,000 requests); a quantity past that is priced at the first tier's rate and the line says so. Data transfer out through the gateway is not the gateway's meter and stays under not priced either way. The VPC link an HTTP API uses to reach a service has no charge of its own; the load balancer behind it is the service's. The matcher is `internal/resolve/aws/cost.go`.

## GCP

Nothing. A gateway node creates no resources. Cloud Run serves each function on its own HTTPS URL, and the real API Gateway product is beta and wants a hand-written OpenAPI document, so the entry point is the target's URL (ADR 0010).

A `routes` edge to a function or a service adds a `google_cloud_run_v2_service_iam_member` granting `roles/run.invoker` to `allUsers` on the target's Cloud Run service, and an output `<gateway>_<target>_url` carrying that service's URL. The binding goes on the Cloud Run service rather than the function because a 2nd gen function answers HTTP through Cloud Run, which checks `run.invoker`; `roles/cloudfunctions.invoker` through `google_cloudfunctions2_function_iam_member` would leave the URL closed. Several routes to one target share one binding and one output, and a service that is already `public` gets the output alone.

There is no single gateway URL. Two routed targets are two entry points, `api_orders_url` and `api_web_url`, and a caller has to know which to use.

The path and methods are not enforced. Cloud Run does not route by path, so the target receives every request to its URL and a route on `/orders` means the target handles `/orders` itself. Two edges claiming the same method and path on one gateway are still refused, as on AWS, because the sketch says two things answer one route.

## Azure

No resources (ADR 0010). API Management takes three hours to create even on the Consumption tier, so the gateway is the routed targets' own public hostnames, and a gateway node on its own produces nothing at all.

A `routes` edge to a function adds an output `<gateway>_<target>_url`, `https://` plus the function app's `default_hostname`. A route to a service adds the same output with the Container App's ingress FQDN, and makes that ingress external whatever the service's `public` says, since a gateway that hands out an internal hostname would route to nothing. There is one output per routed target, named for both ends, whether the gateway routes to one target or several, so the name does not change when a second route is added. The same route key on two edges is refused as on AWS, so a project stays valid across providers.

The routing itself is the target's job. Azure Functions declares HTTP triggers, their routes and their methods in the code, under the `/api` prefix by default, and nothing in the infrastructure can add a route to a running app; a Container App serves whatever its container serves. The edge's path and methods are therefore written into the target's app settings or container env as `<GATEWAY>_ROUTES`, a comma separated list of `<METHOD> <path>` such as `GET /orders,POST /orders`, accumulated across every edge from that gateway to that target, so the code can read what the sketch expects it to serve.

Cost: `togen cost` prices a gateway at zero, because there is nothing to meter. Azure has no gateway resource (ADR 0010): the routed targets answer on their own hostnames, so the summary reads `no resource, routes go to the targets' own URLs`, the subtotal is `0.00`, and nothing appears under not priced. A gateway takes no usage keys on Azure; `requests` in the `usage` block still validates, since the key belongs to the node type rather than the provider, and prices nothing here. What the requests cost is charged to whatever serves them, a function's executions or a service's `Standard Requests` meter. The matcher is `internal/resolve/azure/cost.go`.
