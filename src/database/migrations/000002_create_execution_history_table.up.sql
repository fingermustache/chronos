-- ------------------------------------------------------------
-- Migration:   000002_create_execution_history_table
-- Version:     2026-05-25
-- Purpose:     Create execution_history table with enum status,
--              constraints, and indexes.
-- ------------------------------------------------------------

-- Enum for status
CREATE TYPE task_status AS ENUM ('pending','running','success','failed','timeout');

-- Table
CREATE TABLE execution_history (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    task_id       UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    started_at    TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    completed_at  TIMESTAMPTZ,
    status        task_status NOT NULL,
    error_message TEXT,
    output        TEXT,
    retry_count   INTEGER DEFAULT 0 NOT NULL,
    duration_ms   INTEGER CHECK (duration_ms >= 0),      -- non‑negative
    metadata      JSONB,
    UNIQUE (task_id, started_at)                       -- one row per attempt
);

-- Indexes
CREATE INDEX idx_execution_history_task_id
    ON execution_history(task_id);
CREATE INDEX idx_execution_history_started_at
    ON execution_history(started_at DESC);
CREATE INDEX idx_execution_history_status
    ON execution_history(status);
CREATE INDEX idx_execution_history_task_started
    ON execution_history(task_id, started_at DESC);
CREATE INDEX idx_execution_history_task_status_started
    ON execution_history(task_id, status, started_at DESC);

-- Comments
COMMENT ON TABLE execution_history IS
    'Audit log of all task executions';
COMMENT ON COLUMN execution_history.duration_ms IS
    'Execution duration in milliseconds (non‑negative)';
COMMENT ON COLUMN execution_history.metadata IS
    'Additional execution metadata (e.g., trigger source, environment info)';
