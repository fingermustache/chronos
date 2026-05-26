-- ------------------------------------------------------------
-- Migration:   000001_create_tasks_table (up)
-- Version:     2026-05-25
-- Purpose:     Create the `tasks` table, a namespaced trigger
--              function, and the trigger that updates `updated_at`.
-- ------------------------------------------------------------

-- Enable the UUID extension (leave it in place for the whole DB)
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Enum types for static lists
CREATE TYPE schedule_type_enum AS ENUM ('cron','interval','once');
CREATE TYPE task_type_enum     AS ENUM ('http','command','grpc');

-- Tasks table
CREATE TABLE tasks (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name             VARCHAR(255) NOT NULL,
    description      TEXT,
    schedule_type    schedule_type_enum NOT NULL,
    schedule_config  JSONB NOT NULL DEFAULT '{}'::jsonb,
    task_type        task_type_enum NOT NULL,
    task_config      JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled          BOOLEAN DEFAULT true NOT NULL,
    max_retries      INTEGER DEFAULT 3 NOT NULL CHECK (max_retries >= 0),
    timeout_seconds  INTEGER DEFAULT 300 NOT NULL
                     CHECK (timeout_seconds > 0 AND timeout_seconds <= 600),  -- ≤ 10 min
    next_execution_time TIMESTAMPTZ,
    created_at       TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at       TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    deleted_at       TIMESTAMPTZ,
    UNIQUE (name, deleted_at)
);

-- Indexes (partial to ignore soft‑deleted rows)
CREATE INDEX idx_tasks_enabled
    ON tasks(enabled)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_tasks_next_execution
    ON tasks(next_execution_time)
    WHERE enabled = true AND deleted_at IS NULL;

CREATE INDEX idx_tasks_schedule_type
    ON tasks(schedule_type)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_tasks_deleted_at
    ON tasks(deleted_at);

-- Namespaced trigger function (schema‑wide, not product‑specific)
CREATE OR REPLACE FUNCTION tasks_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger that fires before any UPDATE on tasks
CREATE TRIGGER tasks_set_updated_at
BEFORE UPDATE ON tasks
FOR EACH ROW EXECUTE FUNCTION tasks_set_updated_at();

-- Documentation comments
COMMENT ON TABLE tasks IS 'Stores scheduled task definitions';
COMMENT ON COLUMN tasks.schedule_config IS
    'JSON: {cron_expr:"0 * * * *"} or {interval_seconds:3600} or {run_at:"2026-05-21T10:00:00Z"}';
COMMENT ON COLUMN tasks.task_config IS
    'JSON specific to task type (e.g., HTTP: {url, method, headers, body})';
COMMENT ON COLUMN tasks.next_execution_time IS
    'Calculated next execution time for scheduler optimization';
