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

Cost: `togen cost` prices a gateway at rest. An HTTP API has no hourly charge, so the node has a zero subtotal with the line `priced at rest, usage not set` under it, and `requests` and `data transfer` are listed under not priced until the `usage` block in `togen.yml` sets `requests` (from ADR 0009). The request meter is in the bundled snapshot ready for that: the `API Calls` family of the `AmazonApiGateway` offer file with the `ApiGatewayHttpRequest` usage type, on demand, in the project's region, which is the HTTP API's rate and about a third of the REST API's. It is tiered and the snapshot holds the first tier's rate with the point where it ends. Data transfer out is an EC2 charge rather than the gateway's and stays unpriced. The VPC link an HTTP API uses to reach a service has no charge of its own; the load balancer behind it is the service's. The matcher is `internal/resolve/aws/cost.go`.

## GCP

Not implemented yet. The real API Gateway product is beta and needs an OpenAPI document, so the plan is to expose the Cloud Run URL directly with a public invoker binding.

## Azure

Not implemented yet. API Management takes hours to provision, so the plan is Container Apps ingress with `external_enabled`.
