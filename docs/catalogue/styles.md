# styles

The look every node type has when `togen.yml` says nothing: a colour, an icon and a shape per type, chosen per provider so a diagram looks like the cloud it targets. The table lives in `internal/style`, keyed by provider, and `just generate` publishes it as `schema/styles.json` for the canvas. `style.kinds` in `togen.yml` overrides a type and `style.nodes` one node; the order is node, then kind, then this scheme (ADR 0007).

Colours are the category colours each vendor uses in its own architecture icon set, so a queue is the same pink in Togen as in an AWS deck. Icon ids are `<provider>/<service>` and name an icon in that set; the files live under `ui/icons/<provider>/` with each set's terms in a `NOTICE.md` beside them, and the studio's about panel shows the same notices. The icons are the vendors' own drawings, used as their terms permit, in architecture diagrams. Databases and caches are drawn as a `cylinder`, everything else as a `card`. Each provider also has an accent colour and a colour for the network boundary the resolver creates implicitly.

`icon` in `togen.yml` accepts any id below or a path relative to `togen.yml` starting with `./` or `../`.

## AWS

Icons from the [AWS Architecture Icons](https://aws.amazon.com/architecture/icons/). Accent `#ED7100`. The implicit VPC is drawn in `#8C4FFF`.

| Type | Resource | Colour | Icon | Shape |
| --- | --- | --- | --- | --- |
| service | ECS on Fargate | `#ED7100` | `aws/ecs` | card |
| function | Lambda | `#ED7100` | `aws/lambda` | card |
| database | RDS | `#C925D1` | `aws/rds` | cylinder |
| gateway | API Gateway | `#8C4FFF` | `aws/api-gateway` | card |
| queue | SQS | `#E7157B` | `aws/sqs` | card |
| bucket | S3 | `#7AA116` | `aws/s3` | card |
| cache | ElastiCache | `#C925D1` | `aws/elasticache` | cylinder |

## GCP

Icons from the [Google Cloud architecture icons](https://cloud.google.com/icons). Accent `#4285F4`. The implicit VPC network is drawn in `#4285F4`.

| Type | Resource | Colour | Icon | Shape |
| --- | --- | --- | --- | --- |
| service | Cloud Run | `#34A853` | `gcp/cloud-run` | card |
| function | Cloud Functions | `#34A853` | `gcp/cloud-functions` | card |
| database | Cloud SQL | `#4285F4` | `gcp/cloud-sql` | cylinder |
| gateway | API Gateway | `#4285F4` | `gcp/api-gateway` | card |
| queue | Pub/Sub | `#F9AB00` | `gcp/pubsub` | card |
| bucket | Cloud Storage | `#EA4335` | `gcp/cloud-storage` | card |
| cache | Memorystore | `#4285F4` | `gcp/memorystore` | cylinder |

## Azure

Icons from the [Azure architecture icons](https://learn.microsoft.com/en-us/azure/architecture/icons/). Accent `#0078D4`. The implicit VNet and the resource group around everything are both drawn in `#0078D4`.

| Type | Resource | Colour | Icon | Shape |
| --- | --- | --- | --- | --- |
| service | Container Apps | `#0078D4` | `azure/container-apps` | card |
| function | Functions | `#0078D4` | `azure/functions` | card |
| database | Database for PostgreSQL | `#7B3FE4` | `azure/database-postgresql` | cylinder |
| gateway | API Management | `#0078D4` | `azure/api-management` | card |
| queue | Service Bus | `#FFB900` | `azure/service-bus` | card |
| bucket | Blob Storage | `#6BB700` | `azure/storage-blob` | card |
| cache | Cache for Redis | `#7B3FE4` | `azure/cache-redis` | cylinder |
