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

Cost: `togen cost` prices a function at rest. Lambda charges nothing for a function that is not invoked, so the node has a zero subtotal with the line `priced at rest, usage not set` under it, and `requests` and `duration` are listed under not priced until the `usage` block in `togen.yml` sets them (`invocations` and `durationMs`, from ADR 0009). The two meters are in the bundled snapshot ready for that: the `AWS-Lambda-Requests` and `AWS-Lambda-Duration` groups of the `AWSLambda` offer file, on demand, in the project's region, for x86_64, which is what Lambda runs when `architectures` is not set; the arm64 meters are separate and a fifth cheaper. Duration is billed in GB-seconds, so the size's memory sets what a second of run time costs, which is why the summary shows it alongside the runtime. The duration meter is tiered and the snapshot holds the first tier's rate with the point where it ends. Ephemeral storage beyond the default, provisioned concurrency and the log group's ingestion are not priced. The matcher is `internal/resolve/aws/cost.go`, beside the memory table.

### Edges

A `calls` edge to another function grants `lambda:InvokeFunction` on that function's ARN and injects `<TARGET>_FUNCTION_NAME`. The invoke goes over the AWS API rather than the network, so the caller stays out of the VPC.

A `calls` edge to a service injects `<TARGET>_URL`, which is `http://<name>.<project>-<environment>.local:<port>` through Cloud Map private DNS. That name only resolves inside the VPC, so the function joins it: security group, `vpc_config` over the private subnets and the VPC access policy. The service's security group is opened on its container port from the function's.

## GCP

Not implemented yet. Planned: `google_cloudfunctions2_function` with a source bucket and a service account.

## Azure

Not implemented yet. Planned: `azurerm_linux_function_app` on a consumption plan with a storage account.
