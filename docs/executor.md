# Executor

## Overview

The executor is a standalone service that consumes `TaskTriggerEvent` messages from RabbitMQ and runs tasks.
It is the only service that writes to the `execution_history` table.

**Default port:** none — the executor has no HTTP interface.
**Service boundary:** read-only access to `tasks`; full read/write access to `execution_history` — see [Architecture](architecture.md).

---

## How it works

1. **Consume** — the executor subscribes to `task.trigger.queue` with `prefetch=1`, receiving one message at a time.
2. **Dispatch** — the `task_type` field in the event is looked up in the runners map.
Unknown task types are nacked immediately, routing the message to `task.dlq`, without writing to `execution_history`.
3. **Record start** — a `running` row is written to `execution_history` (`started_at` set by the DB default) before the runner is invoked.
If this write fails, the message is nacked so the trigger isn't silently lost.
4. **Execute** — the matched runner is called with a `context.WithTimeout` derived from both `timeout_seconds` and the shutdown context.
5. **Record completion** — the row is updated to `success` (with `completed_at`, `duration_ms`, `output`) or `failed`/`timeout` (with `completed_at`, `duration_ms`, `error_message`, and `output` when the runner captured one).
This write uses its own short-lived context, independent of the consumer's shutdown context, so a SIGTERM arriving mid-task doesn't prevent the outcome from being recorded.
6. **Ack / Nack** — a successful run acks the message; any error nacks without requeue, routing to `task.dlq`.

Because the per-task timeout context (step 4) is derived from the shutdown context, a SIGTERM cancels in-flight requests immediately rather than waiting out the full task timeout — the in-flight attempt is then recorded as failed rather than left stuck at `running`.

---

## Runner dispatch

Runners are registered at construction time in a `map[task_type → TaskRunner]`.
The `TaskRunner` interface is:

```go
type TaskRunner interface {
    Run(ctx context.Context, config map[string]any) (Result, error)
}
```

`Result` carries the `StatusCode` and `Output` (response body, capped at 64 KB).

Adding a new task type requires only implementing `TaskRunner` and registering it in `defaultRunners()` — the dispatcher and consumer loop need no changes.

### Supported task types

| `task_type` | Runner | Notes |
|---|---|---|
| `http` | `runners.HTTPRunner` | See [HTTP task config](#http-task-config) |
| `command` | — | Out of scope — nacks to DLQ |
| `grpc` | — | Out of scope — nacks to DLQ |

---

## HTTP task config

The HTTP runner reads its configuration from `TaskTriggerEvent.TaskConfig`:

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `url` | string | YES | — | Request URL |
| `method` | string | NO | `GET` | HTTP method |
| `headers` | object | NO | — | Key/value pairs added to the request |
| `body` | string | NO | — | Request body |

**Example:**

```json
{
  "url": "https://api.example.com/webhook",
  "method": "POST",
  "headers": { "Authorization": "Bearer token123", "Content-Type": "application/json" },
  "body": "{\"event\": \"task.fired\"}"
}
```

Non-2xx responses are treated as failures.
The response body (up to 64 KB) and status code are captured in `Result`; the body is recorded as `execution_history.output` on both success and failure.

---

## Graceful shutdown

The executor handles `SIGTERM` and `SIGINT`.
On signal the consumer stops accepting new messages, the in-flight task's context is cancelled, and the process exits after the current message is acked or nacked.

---

## Configuration

| Variable | Default | Description |
|---|---|---|
| `DATABASE_*` | — | See [database.md](database.md) |
| `RABBITMQ_*` | — | See [broker.md](broker.md) |

---

## Running locally

```bash
docker compose up -d postgres rabbitmq
go run ./executor/cmd/
```

---

## Testing

Unit tests (no external dependencies):

```bash
cd src && go test -race ./executor/...
```

Integration tests (spins up PostgreSQL via testcontainers, exercises a full success and failure run through the executor and asserts the resulting `execution_history` row):

```bash
cd src && go test -tags integration -race ./executor/...
```

---

## Project structure

```
src/executor/
├── cmd/
│   └── main.go                        # Entry point — wires database, broker, executor
└── internal/
    ├── config/
    │   └── config.go                  # Env var loading (database + broker)
    ├── execution/
    │   └── executor.go                # Consumer loop, runner dispatch, execution_history recording
    ├── repository/
    │   ├── interface.go               # ExecutionRepository interface
    │   └── postgres.go                # Postgres implementation
    └── runners/
        ├── runner.go                  # TaskRunner interface, Result, ErrUnsupportedTaskType
        └── http.go                    # HTTP runner implementation
```
