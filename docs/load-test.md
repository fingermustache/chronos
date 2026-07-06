# Distributed Load Test

## Goal

Prove that `SELECT ... FOR UPDATE SKIP LOCKED` gives correct distributed claim semantics when multiple scheduler instances poll for due tasks concurrently.
The specific claims under test: no task is ever dispatched by two schedulers at once (duplicate execution), no due task is silently skipped (missed fire), and dispatch latency stays low under a realistic backlog.

## Setup

- Local `docker-compose` Postgres and RabbitMQ (see [database.md](database.md) and [broker.md](broker.md)).
- 1,000 tasks seeded directly into `tasks`, split across all three schedule types:
  - 340 `once` tasks, each already overdue by construction (`next_execution_time` set 1 minute in the past).
  - 330 `interval` tasks at a 20-second interval.
  - 330 `cron` tasks on `* * * * *` (every minute).
- **3 scheduler instances** running concurrently from the same binary, against the same database and broker, with `POLL_INTERVAL_SECONDS=1` and `CLAIM_BATCH_SIZE=50`.
  A small batch size relative to the backlog was chosen deliberately — it forces more polling rounds and more overlap between instances competing for the same due rows than a single large batch would.
- A raw AMQP consumer (bypassing the executor entirely) subscribed to `task.trigger.queue` and recorded every `TaskTriggerEvent` it received, tagged with the wall-clock time it was received.
  The executor was not run for this test; it exercises only the scheduler's claim/publish/advance path, which is what `FOR UPDATE SKIP LOCKED` protects.
- Recording window: 100 seconds, then all three scheduler processes were sent `SIGTERM` and shut down cleanly.

## Results

### Duplicate execution rate: 0

Across 1,990 total trigger events for 1,000 tasks:

| Schedule type | Tasks | Fired exactly as expected | Missed | Duplicated |
|---|---|---|---|---|
| `once` | 340 | 340 (exactly once each) | 0 | 0 |
| `interval` (20s) | 330 | 330 (exactly 3 times each) | 0 | 0 |
| `cron` (every minute) | 330 | 330 (exactly twice each) | 0 | 0 |

The `cron` tasks each fired twice: once as an immediate "catch-up" fire (they were already overdue at seed time), then again at the next real minute boundary.
A naive check first flagged these second fires as suspiciously close together (5-7 seconds apart), but cross-referencing wall-clock time showed all 330 second-fires landed within a 4.1-second spread of each other — i.e. they all fired at the same real cron tick, which is the expected behavior, not a race.

No task, of any schedule type, was ever claimed and published by more than one scheduler instance for the same due occurrence.

### Missed fires: 0

Every one of the 1,000 seeded tasks produced at least one trigger event during the test window.

### Dispatch latency

**Backlog drain** — all 1,000 pre-overdue tasks were claimed and published by the 3 competing schedulers within a **6.1-second spread** from first claim to last.

**Broker publish → consumer receive latency** (`received_at - triggered_at`, all 1,990 events):

| Percentile | Latency |
|---|---|
| min | 0.2 ms |
| p50 | 9.1 ms |
| p95 | 18.7 ms |
| p99 | 20.8 ms |
| max | 22.9 ms |

## Raw data

- [`load-test-data/tasks_snapshot.csv`](load-test-data/tasks_snapshot.csv) — the 1,000 seeded tasks as they existed in `tasks` immediately after the test window closed (`id`, `name`, `schedule_type`, `schedule_config`, `enabled`, `next_execution_time`, `created_at`, `updated_at`).
- [`load-test-data/trigger_events.csv`](load-test-data/trigger_events.csv) — every one of the 1,990 recorded `TaskTriggerEvent`s (`task_id`, `schedule_type`, `triggered_at`, `received_at`), sorted by receive time.

At the time of this snapshot the local database contained only this load test's data — `tasks` had exactly 1,000 rows (all `loadtest-*`) and `execution_history` was empty, since the executor was not part of this test.

## How to reproduce

1. `docker compose up -d db rabbitmq`
2. Run migrations: `migrate -path src/database/migrations -database "postgres://chronos:chronos@localhost:<DB_PORT>/chronos?sslmode=disable" up`
3. Seed 1,000 tasks directly into `tasks` (see the schema in [database.md](database.md) for `schedule_config` shapes per type), with `next_execution_time` in the past so they're immediately due.
4. Build and run 3 concurrent copies of `scheduler/cmd` against the same database and broker, with a small `CLAIM_BATCH_SIZE`.
5. Subscribe a consumer to `task.trigger.queue` and record `task_id` + receive timestamp for every message.
6. Cross-check: every task fired the expected number of times, no two events for the same task landed implausibly close together relative to its own schedule, and no task never fired at all.
