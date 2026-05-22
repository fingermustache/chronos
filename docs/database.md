# Database Schema

## Overview

Chronos uses PostgreSQL to store task definitions and execution history. The schema consists of two main tables with a one-to-many relationship.

---

## Tables

### tasks

Stores scheduled task definitions and configuration.

| Column | Type | Description |
|--------|------|-------------|
| id | UUID | Primary key |
| name | VARCHAR(255) | Task name |
| description | TEXT | Optional description |
| schedule_type | VARCHAR(20) | `cron`, `interval`, or `once` |
| schedule_config | JSONB | Schedule configuration (cron expression, interval seconds, or run_at timestamp) |
| task_type | VARCHAR(20) | `http`, `command`, or `grpc` |
| task_config | JSONB | Task execution parameters (URL, command, etc.) |
| enabled | BOOLEAN | Whether task is active (default: true) |
| max_retries | INTEGER | Maximum retry attempts (default: 3) |
| timeout_seconds | INTEGER | Execution timeout (default: 300) |
| next_execution_time | TIMESTAMPTZ | Next scheduled run time |
| created_at | TIMESTAMPTZ | Creation timestamp |
| updated_at | TIMESTAMPTZ | Last update timestamp (auto-updated) |
| deleted_at | TIMESTAMPTZ | Soft delete timestamp (NULL = active) |

**Indexes:**
- `idx_tasks_next_execution` on `next_execution_time` (for scheduler queries)
- `idx_tasks_enabled` on `enabled`

---

### execution_history

Audit log of task executions.

| Column | Type | Description |
|--------|------|-------------|
| id | UUID | Primary key |
| task_id | UUID | Foreign key → tasks.id |
| started_at | TIMESTAMPTZ | Execution start time |
| completed_at | TIMESTAMPTZ | Execution end time (NULL if running) |
| status | VARCHAR(20) | `pending`, `running`, `success`, `failed`, or `timeout` |
| error_message | TEXT | Error details if failed |
| output | TEXT | Execution output/result |
| retry_count | INTEGER | Retry attempt number (0 = first attempt) |
| duration_ms | INTEGER | Execution duration in milliseconds |
| metadata | JSONB | Additional execution context |

**Indexes:**
- `idx_execution_history_task_id` on `task_id`
- `idx_execution_history_started_at` on `started_at`
- `idx_execution_history_task_started` on `(task_id, started_at DESC)`

---

## Relationships

- One task can have many execution records
- Foreign key: `execution_history.task_id` references `tasks.id`
- On delete: CASCADE (deleting a task removes its execution history)