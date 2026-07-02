# ADR 0005: Broker Topology and Message Schema

## Status

Accepted

## Context

Chronos uses RabbitMQ as an event bus between the scheduler and executor (ADR 0001).
The scheduler claims due tasks from PostgreSQL and must hand them off to the executor reliably — at-least-once delivery, with failed tasks routed to a dead-letter queue for visibility and replay.

Three questions to answer:

1. **Exchange type** — direct vs. fanout vs. topic.
2. **Dead-letter strategy** — what happens when a consumer rejects a message.
3. **Message schema** — what fields the executor needs to execute a task without querying the database.

## Decision

### Topology

```
task.trigger (direct exchange)
  └── task.trigger.queue  [x-dead-letter-exchange: task.dlx]
task.dlx (fanout exchange)
  └── task.dlq
```

- **`task.trigger`** — direct exchange; routing key `task.trigger`.
  Direct is sufficient because there is a single consumer group (executor instances).
  Topic exchange would add complexity with no benefit at this stage.
- **`task.trigger.queue`** — durable, with `x-dead-letter-exchange: task.dlx`.
  Messages that are nacked without requeue are routed to the DLX automatically by RabbitMQ.
- **`task.dlx`** — fanout exchange.
  Fanout allows future consumers (alerting, audit) to bind without changing the DLQ itself.
- **`task.dlq`** — durable queue bound to `task.dlx`.
  Operators inspect and replay messages from the DLQ.
  No TTL is set on the DLQ so messages are retained until explicitly acknowledged or purged.

All declarations are idempotent — safe to call on every service startup.

### Publisher confirms

The scheduler enables publisher confirms (`ch.Confirm`) and waits for each individual confirm (`PublishWithDeferredConfirmWithContext + .Wait()`).
This guarantees the broker has persisted the message before `next_execution_time` is advanced.
Persistent delivery mode (`DeliveryMode: 2`) ensures messages survive a RabbitMQ restart.

### Consumer QoS

Each executor channel sets `Qos(prefetch=1)`.
This implements fair dispatch: an executor only receives the next message after it has acknowledged the previous one, preventing a slow executor from accumulating a backlog while others sit idle.

### Message schema

```json
{
  "task_id":         "uuid",
  "schedule_type":   "cron | interval | once",
  "task_type":       "http | ...",
  "task_config":     { ... },
  "max_retries":     3,
  "timeout_seconds": 30,
  "triggered_at":    "2026-01-01T00:00:00Z"
}
```

The executor carries everything it needs to execute the task in the message body.
It does not query the database to look up the task — this avoids a round-trip and decouples the executor from the API gateway's schema.
`triggered_at` is set by the scheduler at claim time and is used for latency observability.

### Alternatives considered

**Topic exchange instead of direct** — rejected.
Topic routing (`task.#`) would allow partitioning by task type (e.g. `task.http`, `task.grpc`) in the future.
The added complexity is not justified yet; the topology can be migrated when a second executor type is needed.

**Per-message TTL on DLQ** — rejected.
Automatic expiry hides failures.
Operators must explicitly review and replay dead-lettered messages.

**Database as the message store (outbox pattern)** — considered as an alternative to RabbitMQ entirely.
ADR 0001 already chose RabbitMQ for decoupling.
The transactional outbox pattern would add a poller and schema complexity with no benefit given that the scheduler already uses DB polling.

**Separate DLQ per queue** — unnecessary at this scale.
A shared `task.dlq` is sufficient; the `x-death` header added by RabbitMQ records the origin queue.

## Consequences

- All services call `SetupTopology` on startup; topology is declared idempotently.
- The executor never queries the database for task configuration — `task_config` is carried in the event.
- Dead-lettered messages accumulate in `task.dlq` until operators intervene.
- `triggered_at` enables scheduler-to-executor latency metrics.
- Adding a new task type only requires a new handler in the executor; no topology changes.
