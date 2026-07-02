# Broker

Chronos uses RabbitMQ as the event bus between the scheduler and executor.
The `pkg/broker` package owns all broker primitives: connection management, topology setup, publisher, and consumer.

## Topology

```
task.trigger (direct exchange)
  └── task.trigger.queue  [dead-letters → task.dlx]
task.dlx (fanout exchange)
  └── task.dlq
```

| Resource | Type | Durable | Purpose |
|---|---|---|---|
| `task.trigger` | direct exchange | yes | Entry point; scheduler publishes here |
| `task.trigger.queue` | queue | yes | Executor subscribes; DLX configured |
| `task.dlx` | fanout exchange | yes | Receives rejected messages |
| `task.dlq` | queue | yes | Dead-letter storage for inspection/replay |

All declarations are idempotent — every service calls `broker.SetupTopology` on startup.

## Message Schema

Every trigger event carries a `TaskTriggerEvent`:

```json
{
  "task_id":         "550e8400-e29b-41d4-a716-446655440000",
  "schedule_type":   "cron",
  "task_type":       "http",
  "task_config":     { "url": "https://api.example.com/job", "method": "POST" },
  "max_retries":     3,
  "timeout_seconds": 30,
  "triggered_at":    "2026-01-01T12:00:00Z"
}
```

| Field | Type | Description |
|---|---|---|
| `task_id` | UUID | Identifies the task in the database |
| `schedule_type` | string | `cron`, `interval`, or `once` |
| `task_type` | string | Executor handler type (`http`, …) |
| `task_config` | object | Handler-specific configuration |
| `max_retries` | int | Maximum retry attempts on failure |
| `timeout_seconds` | int | Per-attempt execution deadline |
| `triggered_at` | RFC3339 | Scheduler claim time; used for latency metrics |

The executor carries everything it needs in the message body and does not query the database for task configuration.

## Reliability

**Publisher confirms** — the scheduler waits for a broker confirm before advancing `next_execution_time`.
This prevents task skips if the broker is temporarily unavailable.

**Persistent delivery** — messages survive a RabbitMQ restart (`delivery_mode=2`).

**QoS prefetch=1** — each executor instance receives one message at a time, ensuring fair dispatch across horizontally scaled executors.

**Dead-letter queue** — a consumer that returns an error causes the message to be nacked without requeue.
RabbitMQ routes it to `task.dlq` automatically via `task.dlx`.
The `x-death` header on the dead-lettered message records the originating queue and reason.

## Local Development

RabbitMQ is included in `docker-compose.yaml`:

```bash
docker compose up -d rabbitmq
```

The management UI is available at `http://localhost:15672` (credentials: `chronos` / `chronos`).

| Variable | Default | Description |
|---|---|---|
| `RABBITMQ_URL` | — | Full AMQP URL; takes precedence over individual fields |
| `RABBITMQ_HOST` | `localhost` | Broker host |
| `RABBITMQ_PORT` | `5672` | AMQP port |
| `RABBITMQ_USER` | `chronos` | Username |
| `RABBITMQ_PASSWORD` | `chronos` | Password |
| `RABBITMQ_VHOST` | `/` | Virtual host |

## Integration Tests

The `pkg/broker` integration tests spin up an ephemeral RabbitMQ container via Testcontainers:

```bash
cd src && go test -tags integration ./pkg/broker/...
```

Tests cover:

- Publish → consume round-trip with field assertion
- Nack routing to DLQ with polling verification
