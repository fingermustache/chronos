-- ------------------------------------------------------------
-- Migration:   000002_create_execution_history_table.down (down)
-- Version:     2026-05-25
-- Purpose:     Drop the `execution_history` table and all its
--              associated indexes. This provides a clean rollback
--              for migration 000002.
-- ------------------------------------------------------------

DROP INDEX IF EXISTS idx_execution_history_task_id;
DROP INDEX IF EXISTS idx_execution_history_started_at;
DROP INDEX IF EXISTS idx_execution_history_status;
DROP INDEX IF EXISTS idx_execution_history_task_started;
DROP INDEX IF EXISTS idx_execution_history_task_status_started;
DROP TABLE IF EXISTS execution_history;
DROP TYPE IF EXISTS task_status;
