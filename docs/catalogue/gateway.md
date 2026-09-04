# gateway

The public HTTP entry point. A project has at most one. Routes are edges from the gateway to a function or service, each with a path and methods.

Properties: none.

## AWS

- `aws_apigatewayv2_api` with `protocol_type = "HTTP"`.
- `aws_apigatewayv2_stage` named `$default` with `auto_deploy = true`, so route changes go live on apply.
- Output `<name>_url`.

A `routes` edge to a function adds an `AWS_PROXY` integration with payload format 2.0, one `aws_apigatewayv2_route` per method with key `<METHOD> <path>`, and an `aws_lambda_permission` letting the API invoke the function. The permission is scoped to the whole API rather than one route, which is the same thing while a function has a single integration.

Routes to a service are not implemented yet. Planned: the service's load balancer with listener rules.

## GCP

Not implemented yet. The real API Gateway product is beta and needs an OpenAPI document, so the plan is to expose the Cloud Run URL directly with a public invoker binding.

## Azure

Not implemented yet. API Management takes hours to provision, so the plan is Container Apps ingress with `external_enabled`.
