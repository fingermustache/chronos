# tests/test_basic_schema.py
import uuid
import pytest

def make_task(conn, task_id=None, name="Test task"):
    task_id = task_id or uuid.uuid4()
    cur = conn.cursor()
    cur.execute(
        """
        INSERT INTO tasks (id, name, schedule_type, schedule_config, task_type, task_config, enabled)
        VALUES (%s, %s, 'once', '{}'::jsonb, 'http', '{}'::jsonb, true)
        """,
        (str(task_id), name)
    )
    cur.close()
    return task_id


def test_task_insert(conn):
    task_id = make_task(conn)
    cur = conn.cursor()
    cur.execute("SELECT id FROM tasks WHERE id = %s", (str(task_id),))
    assert cur.fetchone() is not None


def test_soft_delete_excludes_from_active_query(conn):
    task_id = make_task(conn)
    cur = conn.cursor()
    cur.execute("UPDATE tasks SET deleted_at = now() WHERE id = %s", (str(task_id),))
    cur.execute("SELECT COUNT(*) FROM tasks WHERE id = %s AND deleted_at IS NULL", (str(task_id),))
    assert cur.fetchone()[0] == 0


def test_hard_delete_cascades_to_execution_history(conn):
    task_id = make_task(conn)
    cur = conn.cursor()
    cur.execute(
        "INSERT INTO execution_history (task_id, status) VALUES (%s, 'running'), (%s, 'success')",
        (str(task_id), str(task_id))
    )
    cur.execute("DELETE FROM tasks WHERE id = %s", (str(task_id),))
    cur.execute("SELECT COUNT(*) FROM execution_history WHERE task_id = %s", (str(task_id),))
    assert cur.fetchone()[0] == 0


def test_next_execution_index_is_used(conn):
    task_id = make_task(conn)
    cur = conn.cursor()
    cur.execute(
        "UPDATE tasks SET next_execution_time = now() + interval '10 minutes' WHERE id = %s",
        (str(task_id),)
    )
    cur.execute(
        """
        EXPLAIN SELECT id FROM tasks
        WHERE enabled = true AND deleted_at IS NULL
        ORDER BY next_execution_time ASC
        LIMIT 1
        """
    )
    plan = "\n".join(row[0] for row in cur.fetchall())
    assert "idx_tasks_next_execution" in plan


def test_unique_name_allows_reuse_after_soft_delete(conn):
    task_id = make_task(conn, name="unique-task")
    cur = conn.cursor()
    cur.execute("UPDATE tasks SET deleted_at = now() WHERE id = %s", (str(task_id),))
    # Same name should be insertable after soft delete
    make_task(conn, name="unique-task")
    cur.execute("SELECT COUNT(*) FROM tasks WHERE name = 'unique-task'")
    assert cur.fetchone()[0] == 2


def test_duplicate_active_name_is_rejected(conn):
    make_task(conn, name="dupe-task")
    with pytest.raises(Exception, match="unique"):
        make_task(conn, name="dupe-task")
