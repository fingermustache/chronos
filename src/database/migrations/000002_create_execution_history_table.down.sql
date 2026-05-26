-- ------------------------------------------------------------
-- Migration:   000002_create_execution_history_table.down (down)
-- Version:     2026-05-25
-- Purpose:     Drop the `execution_history` table and all its
--              associated indexes. This provides a clean rollback
--              for migration 000002.
-- ------------------------------------------------------------

DROP TABLE IF EXISTS execution_history;
