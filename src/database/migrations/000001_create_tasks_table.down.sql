-- ------------------------------------------------------------
-- Migration:   000001_create_tasks_table (down)
-- Version:     2026-05-25
-- Purpose:     Remove the `tasks` table, its trigger, and its
--              dedicated trigger function.  Do NOT drop the
--              shared uuid-ossp extension.
-- ------------------------------------------------------------

DROP TRIGGER IF EXISTS tasks_set_updated_at ON tasks;
DROP FUNCTION IF EXISTS tasks_set_updated_at();
DROP INDEX IF EXISTS idx_tasks_name_active_unique;
DROP TABLE IF EXISTS tasks;
