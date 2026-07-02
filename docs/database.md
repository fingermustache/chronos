# Database

## Overview

Chronos uses PostgreSQL to store task definitions and execution history. The schema consists of two tables with a one-to-many relationship, managed by [golang-migrate](https://github.com/golang-migrate/migrate).

---

## Schema

### `tasks`

Stores scheduled task definitions and configuration.

| Column | Type | Nullable | Default | Description |
|---|---|---|---|---|
| `id` | `UUID` | NO | `gen_random_uuid()` | Primary key |
| `name` | `VARCHAR(255)` | NO | — | Task name, unique |
| `description` | `TEXT` | YES | `NULL` | Optional description |
| `schedule_type` | `VARCHAR(20)` | NO | — | `cron`, `interval`, or `once` |
| `schedule_config` | `JSONB` | NO | `'{}'` | Schedule parameters — see [Schedule Config](#schedule-config) |
| `task_type` | `VARCHAR(20)` | NO | — | `http`, `command`, or `grpc` |
| `task_config` | `JSONB` | NO | `'{}'` | Execution parameters — see [Task Config](#task-config) |
| `enabled` | `BOOLEAN` | NO | `true` | Whether the task is active |
| `max_retries` | `INTEGER` | NO | `3` | Maximum retry attempts on failure |
| `timeout_seconds` | `INTEGER` | NO | `300` | Execution timeout in seconds — must be between 1 and 600 (enforced by `CHECK` constraint and API validation) |
| `timezone` | `VARCHAR(64)` | YES | `NULL` | IANA timezone name (e.g. `America/New_York`). `NULL` means UTC. Affects cron schedule evaluation only |
| `next_execution_time` | `TIMESTAMPTZ` | YES | `NULL` | Next scheduled run — always stored as UTC, calculated in the task's timezone |
| `created_at` | `TIMESTAMPTZ` | NO | `NOW()` | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | NO | `NOW()` | Last update — auto-updated via trigger |
| `deleted_at` | `TIMESTAMPTZ` | YES | `NULL` | Soft delete timestamp — `NULL` means active |

**Indexes:**

| Index | Columns | Type | Notes |
|---|---|---|---|
| `idx_tasks_next_execution_time` | `next_execution_time` | Partial B-tree | `WHERE deleted_at IS NULL AND enabled = true` — scheduler hot path |
| `idx_tasks_enabled` | `enabled` | B-tree | Filters active tasks |

---

### `execution_history`

Append-only audit log of every task execution attempt. Only the executor
service writes to this table.

| Column | Type | Nullable | Default | Description |
|---|---|---|---|---|
| `id` | `UUID` | NO | `gen_random_uuid()` | Primary key |
| `task_id` | `UUID` | NO | — | Foreign key → `tasks.id` |
| `started_at` | `TIMESTAMPTZ` | NO | `NOW()` | Execution start time |
| `completed_at` | `TIMESTAMPTZ` | YES | `NULL` | End time — `NULL` while running |
| `status` | `VARCHAR(20)` | NO | `pending` | `pending`, `running`, `succeeded`, `failed`, or `timeout` |
| `error_message` | `TEXT` | YES | `NULL` | Error detail if `status = failed` |
| `output` | `TEXT` | YES | `NULL` | Stdout/response body from execution |
| `retry_count` | `INTEGER` | NO | `0` | Attempt number — `0` is the first attempt |
| `duration_ms` | `BIGINT` | YES | `NULL` | Wall-clock duration in milliseconds |
| `metadata` | `JSONB` | NO | `'{}'` | Arbitrary execution context (headers, env, etc.) |

> **Note:** `duration_ms` uses `BIGINT` rather than `INTEGER` to safely
> represent long-running tasks. `INTEGER` overflows at ~24.8 days.

**Indexes:**

| Index | Columns | Notes |
|---|---|---|
| `idx_execution_history_task_id` | `task_id` | FK lookup |
| `idx_execution_history_started_at` | `started_at` | Time-range queries |
| `idx_execution_history_task_started` | `(task_id, started_at DESC)` | Paginated history per task |

---

## Relationships

- One `tasks` row → many `execution_history` rows
- `execution_history.task_id` references `tasks.id` with `ON DELETE CASCADE`
- Deleting a task (hard delete) removes all its execution history

---

## Soft Deletes

Tasks use soft deletion. The `deleted_at` column is `NULL` for active records
and set to the deletion timestamp when a task is deleted via the API.

**Implications for queries:**

- All repository queries filter `WHERE deleted_at IS NULL` by default
- The `GetDueTasks` query in the scheduler uses the partial index which
  already excludes soft-deleted rows
- Hard deletes are not exposed via the API — `deleted_at` is the only
  deletion mechanism
- Execution history for soft-deleted tasks is preserved for audit purposes

---

## JSONB Config Shapes

### Schedule Config

| `schedule_type` | Expected keys | Example |
|---|---|---|
| `cron` | `expression` | `{"expression": "0 * * * *"}` |
| `interval` | `seconds` | `{"seconds": 3600}` |
| `once` | `run_at` | `{"run_at": "2026-06-02T09:00:00Z"}` |

### Task Config

| `task_type` | Expected keys | Example |
|---|---|---|
| `http` | `url`, `method`, `headers`, `body` | `{"url": "https://example.com/hook", "method": "POST"}` |
| `command` | `command`, `args`, `env` | `{"command": "/usr/bin/python3", "args": ["/scripts/job.py"]}` |
| `grpc` | `address`, `service`, `method`, `payload` | `{"address": "svc:50051", "service": "Worker", "method": "Run"}` |

---

## Migrations

Migrations live in `src/database/migrations/` and follow the
`golang-migrate` naming convention:

```
000001_create_tasks_table.up.sql
000001_create_tasks_table.down.sql
000002_create_execution_history_table.up.sql
000002_create_execution_history_table.down.sql
000003_add_timezone_to_tasks.up.sql
000003_add_timezone_to_tasks.down.sql
```

To apply migrations manually:

```bash
migrate -path src/database/migrations \
        -database "postgres://chronos:chronos@localhost:5432/chronos?sslmode=disable" \
        up
```

In tests, migrations are applied automatically by `pkg/testutil.NewTestDB()`
and `pkg/testutil.NewTestDBWithTeardown()`.

---

## Connection Pool

Configured in `pkg/database.DefaultConfig()`. Values are tuned for a single
service instance — if multiple services share a pool, reduce `MaxOpenConns`
proportionally.

| Setting | Default | Rationale |
|---|---|---|
| `MaxOpenConns` | `25` | Prevents overwhelming Postgres default `max_connections = 100` across services |
| `MaxIdleConns` | `5` | Keeps a small warm pool without holding excess connections |
| `ConnMaxLifetime` | `5m` | Rotates connections to prevent stale state behind a load balancer |
| `ConnMaxIdleTime` | `2m` | Reclaims idle connections under low traffic |

---

## Transaction Boundaries

`pkg/database.DB.WithTx()` wraps operations that must succeed or fail
atomically. Current usage:

| Operation | Transactional | Reason |
|---|---|---|
| Create task | NO | Single insert |
| Update task | NO | Single update |
| Delete task (soft) | NO | Single update |
| Create execution + mark task scheduled | YES | Cross-table write — both must succeed |
| Update execution status | NO | Single update |
