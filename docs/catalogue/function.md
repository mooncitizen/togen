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

- `google_service_account` with `account_id` `<project>-<environment>-<name>`. The function runs as it, and it is what later edges grant roles to. Account ids are capped at 30 characters, so a long project, environment and name combination is refused with a message saying which to shorten.
- `google_storage_bucket` for function sources, one per project, created by whichever function resolves first. Bucket names are global, so it is `<gcp project id>-<project>-<environment>-functions` with the project id from the `project` variable. `uniform_bucket_level_access` and `force_destroy` are on: it holds nothing but zips Terraform uploaded.
- `google_storage_bucket_object` `<name>.zip` in that bucket, uploaded from the variable `<name>_package`, which defaults to `functions/<name>.zip` as on AWS. The generated code validates without the file; plan and apply need it.
- `google_cloudfunctions2_function` in the project's region. `build_config` names the runtime, the entry point and the source object, with the object's `generation` alongside it: a new zip is a new generation, so Terraform rebuilds the function the way `source_code_hash` redeploys a Lambda. `service_config` carries the memory, `timeout_seconds` from `timeoutSeconds`, the service account, and the node's `env` as `environment_variables` plus whatever edges add.
- If any edge gives the function access to something on the private network, `service_config` gains `vpc_connector` set to the implicit network's connector, with `PRIVATE_RANGES_ONLY` egress so only private addresses go through it, and the first node that needs the network creates it. A `reads` or `writes` edge to a database does that; a function with no such edge stays off the connector and no network is created for it.

Runtimes: node `nodejs22`, python `python313`, go `go126`. The entry point is the part of `handler` after the last dot, so `index.handler` gives `handler`. Cloud Functions looks the function up by name in the source's main module (`index.js` for Node, `main.py` for Python, the package for Go) rather than by file, so the file part has nothing to say. A handler that ends in a dot names no function and is refused.

Sizes are memory: small 512Mi, medium 1Gi, large 2Gi.

The generated code does not enable APIs. The GCP project needs Cloud Functions, Cloud Build, Cloud Run and Artifact Registry enabled before the first apply.

### Edges

A `routes` edge from the gateway opens the function to the internet and outputs its URL, described under [gateway](gateway.md). A `reads` or `writes` edge to a database sets the connection details on the function and puts it on the connector, described under [database](database.md). Other edges are not supported by the GCP resolver yet.

## Azure

- `azurerm_service_plan` with `os_type = "Linux"` and `sku_name = "Y1"`, the consumption plan: billed per execution, nothing when idle. One per function, as each AWS function has its own role and log group. A plan is free, so sharing one would save a resource and gain nothing.
- `azurerm_storage_account`, Standard tier with LRS replication, which the Functions host needs for its own state and for the content share the consumption plan runs from. Its name is the project, environment and node name with the hyphens dropped, cut to the 24 character limit (the project first, since the node is what tells accounts apart), because storage account names are lowercase alphanumerics and globally unique. The function app reaches it with the account's primary access key, which is the way the provider documents for the consumption plan.
- `azurerm_linux_function_app` in the resource group every Azure project gets, with `https_only`, a system assigned identity for the role assignments edges will add, and the runtime in `site_config.application_stack`.
- `app_settings` from the node's `env`, plus `AzureFunctionsJobHost__functionTimeout` carrying `timeoutSeconds` as `hh:mm:ss`. That is the documented way to set a `host.json` value from outside the package, so the timeout applies without editing the code's own `host.json`. The consumption plan stops a function after ten minutes, so a `timeoutSeconds` above 600 is refused rather than written and ignored.

Runtimes: node `node_version = "22"`, python `python_version = "3.12"`, go `use_custom_runtime = true`. Azure Functions has no Go worker, so a Go function runs as a custom handler: an executable the code's `host.json` names under `customHandler.description.defaultExecutablePath`, invoked over HTTP by the host.

`handler` maps to nothing. On Azure the entry point is declared in the code (the function definitions in the package, or `host.json` for a custom handler), not in the infrastructure. `size` maps to nothing either: Y1 has one memory tier and scales by instance count, so small, medium and large produce the same plan.

The consumption plan does not join the virtual network, so a function never asks for one. Code is deployed separately (`func azure functionapp publish`, or a zip deploy); the generated Terraform does not carry a package the way the AWS output does, so there is no package variable.

### Edges

A `routes` edge from a gateway makes the app's default hostname the gateway URL for this function and records the routes as an app setting, described under [gateway](gateway.md). Other edges are not implemented yet.
