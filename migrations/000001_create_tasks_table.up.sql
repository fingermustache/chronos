-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Tasks table
CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    schedule_type VARCHAR(20) NOT NULL CHECK (schedule_type IN ('cron', 'interval', 'once')),
    schedule_config JSONB NOT NULL,
    task_type VARCHAR(20) NOT NULL CHECK (task_type IN ('http', 'command', 'grpc')),
    task_config JSONB NOT NULL,
    enabled BOOLEAN DEFAULT true NOT NULL,
    max_retries INTEGER DEFAULT 3 NOT NULL CHECK (max_retries >= 0),
    timeout_seconds INTEGER DEFAULT 300 NOT NULL CHECK (timeout_seconds > 0),
    next_execution_time TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Indexes for performance
CREATE INDEX idx_tasks_enabled ON tasks(enabled) WHERE deleted_at IS NULL;
CREATE INDEX idx_tasks_next_execution ON tasks(next_execution_time) WHERE enabled = true AND deleted_at IS NULL;
CREATE INDEX idx_tasks_schedule_type ON tasks(schedule_type) WHERE deleted_at IS NULL;
CREATE INDEX idx_tasks_deleted_at ON tasks(deleted_at);

-- Updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger for tasks table
CREATE TRIGGER update_tasks_updated_at BEFORE UPDATE ON tasks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Add comments for documentation
COMMENT ON TABLE tasks IS 'Stores scheduled task definitions';
COMMENT ON COLUMN tasks.schedule_config IS 'JSON: {cron_expr: "0 * * * *"} or {interval_seconds: 3600} or {run_at: "2026-05-21T10:00:00Z"}';
COMMENT ON COLUMN tasks.task_config IS 'JSON configuration specific to task type (e.g., HTTP: {url, method, headers, body})';
COMMENT ON COLUMN tasks.next_execution_time IS 'Calculated next execution time for scheduler optimization';
