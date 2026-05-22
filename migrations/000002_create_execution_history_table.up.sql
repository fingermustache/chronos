-- Execution history table
CREATE TABLE execution_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    started_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    completed_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(20) NOT NULL CHECK (status IN ('pending', 'running', 'success', 'failed', 'timeout')),
    error_message TEXT,
    output TEXT,
    retry_count INTEGER DEFAULT 0 NOT NULL,
    duration_ms INTEGER,
    metadata JSONB
);

-- Indexes for queries
CREATE INDEX idx_execution_history_task_id ON execution_history(task_id);
CREATE INDEX idx_execution_history_started_at ON execution_history(started_at DESC);
CREATE INDEX idx_execution_history_status ON execution_history(status);
CREATE INDEX idx_execution_history_task_started ON execution_history(task_id, started_at DESC);

-- Composite index for common query pattern
CREATE INDEX idx_execution_history_task_status_started ON execution_history(task_id, status, started_at DESC);

-- Add comments
COMMENT ON TABLE execution_history IS 'Audit log of all task executions';
COMMENT ON COLUMN execution_history.duration_ms IS 'Execution duration in milliseconds';
COMMENT ON COLUMN execution_history.metadata IS 'Additional execution metadata (e.g., trigger source, environment info)';
