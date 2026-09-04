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

## GCP

Not implemented yet. Planned: `google_pubsub_topic` with a `google_pubsub_subscription` per consumer, a dead letter topic on the subscription's `dead_letter_policy`, and the ordering key rather than a separate FIFO queue type.

## Azure

Not implemented yet. Planned: `azurerm_servicebus_namespace` shared by the project and an `azurerm_servicebus_queue` per node, with `requires_session` for FIFO and the built in dead letter queue rather than a second resource.
