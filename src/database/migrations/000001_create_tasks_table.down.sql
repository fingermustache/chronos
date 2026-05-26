-- ------------------------------------------------------------
-- Migration:   000001_create_tasks_table (down)
-- Version:     2026-05-25
-- Purpose:     Drop the `tasks` table, its trigger, function,
--              and the UUID extension – clean rollback.
-- ------------------------------------------------------------

DROP TRIGGER IF EXISTS update_tasks_updated_at ON tasks;
DROP FUNCTION IF EXISTS update_updated_at_column();
DROP TABLE IF EXISTS tasks;
DROP EXTENSION IF EXISTS "uuid-ossp";
