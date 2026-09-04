# function

A serverless function. Short lived, invoked by a gateway route or a queue.

Properties: runtime, handler, size, timeoutSeconds, env.

## AWS

- `aws_iam_role` assumable by Lambda, with `AWSLambdaBasicExecutionRole` attached.
- `aws_cloudwatch_log_group` at `/aws/lambda/<function name>` with 30 day retention, created explicitly so the retention is set rather than unlimited.
- `aws_lambda_function`. The deployment package path is a variable `<name>_package` defaulting to `functions/<name>.zip`, so the generated code validates without a build step and the user wires their own packaging. `source_code_hash` is `filebase64sha256` of that variable, so Terraform redeploys whenever the zip on disk changes.
- Environment variables from the node's `env` plus whatever edges add.
- If any edge gives the function access to something in the VPC, it gets `aws_security_group` with an allow-all egress rule, `vpc_config` over the private subnets, and `AWSLambdaVPCAccessExecutionRole`. The first node that needs the private network creates the VPC, with a single NAT gateway that costs roughly 30 US dollars a month before data charges.
- If any edge grants permissions, they are collected into one `aws_iam_role_policy` on the role.

Runtimes: node `nodejs22.x`, python `python3.12`, go `provided.al2023` with handler `bootstrap`.

Sizes are memory: small 512 MB, medium 1024 MB, large 2048 MB.

## GCP

Not implemented yet. Planned: `google_cloudfunctions2_function` with a source bucket and a service account.

## Azure

Not implemented yet. Planned: `azurerm_linux_function_app` on a consumption plan with a storage account.
