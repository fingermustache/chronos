# Scheduler

The scheduler is a standalone service that continuously polls PostgreSQL for due tasks, claims them safely in a distributed environment, publishes a trigger event per task to RabbitMQ, and advances (or disables) each task's schedule.

## How it works

On every poll tick the scheduler opens a single database transaction and executes:

1. **Claim due tasks** — `SELECT … FOR UPDATE SKIP LOCKED LIMIT <batch_size>` against `tasks` where `enabled = true` and `next_execution_time <= NOW()`.
SKIP LOCKED means multiple scheduler instances can run concurrently without claiming the same task twice.
2. **Publish** — for each claimed task, a `TaskTriggerEvent` is published to the RabbitMQ exchange.
3. **Advance schedule** — still inside the same transaction:
   - `interval` tasks: `next_execution_time` is set to `NOW() + interval_seconds`.
   - `cron` tasks: `next_execution_time` is set to the next occurrence after `NOW()` as computed by the cron expression.
   - `once` tasks: `enabled` is set to `false`; the task never fires again.

Because publish and schedule advance share one transaction, a broker failure rolls back the claim — the task will be retried on the next tick rather than silently dropped.

## Graceful shutdown

`Run()` listens for `SIGTERM` / `SIGINT`.
When a signal arrives the current tick runs to completion before the process exits.

## Configuration

| Env var | Default | Description |
|---|---|---|
| `POLL_INTERVAL_SECONDS` | `1` | How often (in seconds) the scheduler polls for due tasks. |
| `CLAIM_BATCH_SIZE` | `100` | Maximum tasks claimed per tick. |
| `DATABASE_*` | — | See [database.md](database.md). |
| `RABBITMQ_*` | — | See [broker.md](broker.md). |

## Running locally

```bash
docker compose up -d postgres rabbitmq
go run ./scheduler/cmd/
```

## Testing

Unit tests (no external dependencies):

```bash
go test -race ./scheduler/...
```

Integration tests (spins up PostgreSQL and RabbitMQ via testcontainers):

```bash
go test -tags integration -race ./scheduler/...
```
