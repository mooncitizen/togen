# queue

A message queue. One side puts work on it, the other side takes work off it, and neither has to be up at the same time as the other.

Properties: fifo, deadLetter, retentionDays.

## AWS

- `aws_sqs_queue.<name>` with `sqs_managed_sse_enabled`, so messages are encrypted at rest without anyone having to own a KMS key. `message_retention_seconds` is `retentionDays` in seconds, four days by default. `visibility_timeout_seconds` starts at 30 and a `consumes` edge from a function raises it, see below.
- `aws_sqs_queue.<name>_dlq` when `deadLetter` is set, which it is by default. The main queue gets a `redrive_policy` sending a message to it after five failed receives. The dead letter queue keeps messages for fourteen days, the SQS maximum, regardless of `retentionDays`: it is where someone looks days after the failure, and a poison message that expires before anyone reads it tells you nothing.
- Output `<name>_url`.

A queue does not need the VPC, so a project of nothing but queues and their publishers pays for no NAT gateway.

`maxReceiveCount` is 5 and cannot be configured yet. So is `batch_size` on the Lambda mapping, at 10.

### FIFO

`fifo` names the queue `<name>.fifo` and sets `fifo_queue`, because SQS requires the suffix. It also sets `content_based_deduplication`, so the SHA of the message body is the deduplication id and a publisher does not have to invent one.

The dead letter queue is named and flagged the same way. A FIFO queue can only redrive to another FIFO queue.

FIFO queues are ordered within a message group and much slower than standard queues. Take one only when the order actually matters.

### Edges

A `publishes` edge from a function or a service grants `sqs:SendMessage`, `sqs:GetQueueUrl` and `sqs:GetQueueAttributes` on the queue, and sets `<NAME>_URL` on the publisher, where `<NAME>` is the queue's name upper-snake-cased.

A `consumes` edge grants `sqs:ReceiveMessage`, `sqs:DeleteMessage`, `sqs:GetQueueAttributes` and `sqs:ChangeMessageVisibility`. Those four are what the Lambda poller uses, and a consumer that reads without deleting will reprocess everything forever.

From a function, `consumes` also creates `aws_lambda_event_source_mapping.<function>_<queue>`, so Lambda polls the queue and invokes the function with a batch. No environment variable, because the function is handed the messages rather than fetching them. The queue's `visibility_timeout_seconds` is raised to six times the function's timeout, AWS's own recommendation: Lambda refuses a mapping whose visibility timeout is below the function timeout, and a margin that tight redelivers a batch that is still being processed. A default 30 second function gives 180. Several consumers take the largest.

From a service, `consumes` sets `<NAME>_URL` on the task and leaves it to poll for itself. No mapping, and no change to the visibility timeout, because nothing knows how long the service takes over a message.

Cost: `togen cost` prices a queue at rest. SQS charges per request and nothing for a queue that sits there, so the node has a zero subtotal with the line `priced at rest, usage not set` under it, and `messages` is listed under not priced until the `usage` block in `togen.yml` sets it. The request meter is in the bundled snapshot ready for that: the `API Request` family of the `AWSQueueService` offer file for the `Standard` queue type, or `FIFO (first-in, first-out)` when `fifo` is set, on demand, in the project's region; FIFO is a quarter dearer. It is tiered and the snapshot holds the first tier's rate with the point where it ends. A message costs at least three requests (send, receive, delete), a Lambda consumer polling an empty queue costs requests too, and the dead letter queue is billed on the same meter. The matcher is `internal/resolve/aws/cost.go`.

## GCP

- `google_pubsub_topic.<name>` named `<project>-<environment>-<name>`. Pub/Sub has no queue as such: a topic takes what publishers send, and each consumer gets a subscription of its own on it, so `retentionDays` and `fifo` land on the subscriptions rather than the topic. Retention is `message_retention_duration` in seconds, four days by default; Pub/Sub allows up to 31 days and the property tops out at 14, so nothing is capped.
- `google_pubsub_topic.<name>_dead_letter` when `deadLetter` is set, which it is by default, with a `google_pubsub_subscription.<name>_dead_letter` on it that keeps messages for 31 days, the Pub/Sub maximum, regardless of `retentionDays`. A topic with no subscription drops what it is given, so without that subscription the dead letters would go nowhere. Pub/Sub forwards a dead letter as its service agent (`service-<project number>@gcp-sa-pubsub.iam.gserviceaccount.com`, found through `data.google_project.current`), which is granted `roles/pubsub.publisher` on the dead letter topic here and `roles/pubsub.subscriber` on each subscription that redrives to it.
- Output `<name>_topic`, the topic's name.

A queue does not need the VPC or the connector.

`max_delivery_attempts` is 5 and cannot be configured yet.

### FIFO

Pub/Sub orders messages per ordering key rather than per topic, so `fifo` sets `enable_message_ordering` on each consumer's subscription and the publisher has to put an ordering key on every message it wants ordered. Messages without a key are delivered in any order. There is no deduplication: Pub/Sub delivers at least once, and a consumer has to tolerate a repeat.

### Edges

A `publishes` edge from a function or a service grants the publisher's service account `roles/pubsub.publisher` on the topic (`google_pubsub_topic_iam_member.<queue>_from_<publisher>`) and sets `<NAME>_TOPIC` on the publisher, where `<NAME>` is the queue's name upper-snake-cased and the value is the topic's short name. The client libraries take a short name and fill in the project they run in.

A `consumes` edge delivers to the consumer by push. Both shapes run as the consumer's own service account, so there is no account to create for the trigger or the subscription, and the consumer's account is granted `roles/run.invoker` on the consumer's own Cloud Run service (`google_cloud_run_v2_service_iam_member.<consumer>_<queue>`) so the push is let in. Cloud Run counts a Pub/Sub push and an Eventarc delivery from the same project as internal traffic, so a private consumer stays private.

From a function, `consumes` creates `google_eventarc_trigger.<function>_<queue>` on the topic's `messagePublished` event, targeting the function's Cloud Run service and running as the function's account, which is also granted `roles/eventarc.eventReceiver` on the project (`google_project_iam_member.<function>_event_receiver`, once per function). The function receives the message as a CloudEvent. Eventarc creates and owns the subscription behind the trigger, so `fifo`, `retentionDays` and `deadLetter` do not reach a function consumer: it gets Eventarc's own retry policy, and a message that keeps failing is retried until Eventarc gives up rather than parked on the dead letter topic. That is the trade for the CloudEvent shape Google documents for 2nd gen functions. No environment variable, because the function is handed the messages rather than fetching them.

From a service, `consumes` creates `google_pubsub_subscription.<service>_<queue>` with a `push_config` pointing at the service's URL and an `oidc_token` minted for the service's account, which the service should verify. The subscription carries the queue's retention and ordering, and a `dead_letter_policy` sending a message to the dead letter topic after five failed deliveries when the queue has one. Pub/Sub mints the token as its service agent, which is granted `roles/iam.serviceAccountTokenCreator` on the service's account (`google_service_account_iam_member.<service>_token_creator`, once per service). No environment variable either: the message arrives as an HTTP POST with the Pub/Sub envelope, and a 2xx response acknowledges it. The acknowledgement deadline is Pub/Sub's default of ten seconds, so a service that takes longer over a message will see it again.

Pull subscriptions for services and topic schemas are not supported yet.

The generated code does not enable APIs. The GCP project needs Pub/Sub and, for a function consumer, Eventarc enabled before the first apply.

Cost: `togen cost` has no GCP prices yet and reports the queue as not priced.

## Azure

Not implemented yet. Planned: `azurerm_servicebus_namespace` shared by the project and an `azurerm_servicebus_queue` per node, with `requires_session` for FIFO and the built in dead letter queue rather than a second resource.
