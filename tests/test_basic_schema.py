# tests/test_basic_schema.py
import uuid
import pytest
import psycopg2

@pytest.fixture(scope="session")
def db_conn():
    dsn = "postgresql://postgres:postgres@localhost:5432/test_db"
    with psycopg2.connect(dsn) as conn:
        conn.autocommit = True
        yield conn

@pytest.fixture(autouse=True)
def fresh_schema(db_conn):
    cur = db_conn.cursor()
    cur.execute(open("src/database/migrations/000001_create_tasks_table.up.sql").read())
    cur.execute(open("src/database/migrations/000002_create_execution_history_table.up.sql").read())
    cur.execute(open("src/database/migrations/000003_add_timezone_to_tasks.up.sql").read())
    yield
    cur.execute(open("src/database/migrations/000003_add_timezone_to_tasks.down.sql").read())
    cur.execute(open("src/database/migrations/000002_create_execution_history_table.down.sql").read())
    cur.execute(open("src/database/migrations/000001_create_tasks_table.down.sql").read())
    cur.close()

def test_task_insert_softdelete_cascade(db_conn):
    cur = db_conn.cursor()

    # Insert a task with a concrete next_execution_time
    task_id = uuid.uuid4()
    cur.execute(
        """
        INSERT INTO tasks (id, name, schedule_type, schedule_config,
                           task_type, task_config, enabled, next_execution_time)
        VALUES (%s, 'Demo task', 'once', '{}'::jsonb,
                'http', '{}'::jsonb, true,
                now() + interval '10 minutes')
        """,
        (str(task_id),)          # <-- UUID converted to string
    )

    # Verify index usage
    cur.execute(
        """
        EXPLAIN (ANALYZE, BUFFERS)
        SELECT id FROM tasks
        WHERE enabled = true AND deleted_at IS NULL
        ORDER BY next_execution_time ASC
        LIMIT 1
        """
    )
    plan = "\n".join(row[0] for row in cur.fetchall())
    assert "Index Scan" in plan and "idx_tasks_next_execution" in plan

    # Soft‑delete the task
    cur.execute("UPDATE tasks SET deleted_at = now() WHERE id = %s", (str(task_id),))

    cur.execute(
        "SELECT COUNT(*) FROM tasks WHERE enabled = true AND deleted_at IS NULL"
    )
    assert cur.fetchone()[0] == 0

    # Hard‑delete cascade test
    hard_task_id = uuid.uuid4()
    cur.execute(
        """
        INSERT INTO tasks (id, name, schedule_type, schedule_config,
                           task_type, task_config, enabled)
        VALUES (%s, 'Hard task', 'once', '{}'::jsonb,
                'http', '{}'::jsonb, true)
        """,
        (str(hard_task_id),)
    )
    cur.execute(
        """
        INSERT INTO execution_history (task_id, status, started_at)
        VALUES (%s, 'running', now()),
               (%s, 'success', now() + interval '1 second')
        """,
        (str(hard_task_id), str(hard_task_id))
    )
    cur.execute(
        "SELECT COUNT(*) FROM execution_history WHERE task_id = %s",
        (str(hard_task_id),)
    )
    assert cur.fetchone()[0] == 2

    cur.execute("DELETE FROM tasks WHERE id = %s", (str(hard_task_id),))

    cur.execute(
        "SELECT COUNT(*) FROM execution_history WHERE task_id = %s",
        (str(hard_task_id),)
    )
    assert cur.fetchone()[0] == 0

    cur.close()


def test_timezone_column(db_conn):
    cur = db_conn.cursor()

    # NULL timezone (default) is accepted
    task_id = uuid.uuid4()
    cur.execute(
        """
        INSERT INTO tasks (id, name, schedule_type, schedule_config, task_type, task_config)
        VALUES (%s, 'tz-null', 'cron', '{}'::jsonb, 'http', '{}'::jsonb)
        """,
        (str(task_id),),
    )
    cur.execute("SELECT timezone FROM tasks WHERE id = %s", (str(task_id),))
    assert cur.fetchone()[0] is None

    # Named IANA timezone is stored and returned verbatim
    task_id2 = uuid.uuid4()
    cur.execute(
        """
        INSERT INTO tasks (id, name, schedule_type, schedule_config, task_type, task_config, timezone)
        VALUES (%s, 'tz-ny', 'cron', '{}'::jsonb, 'http', '{}'::jsonb, 'America/New_York')
        """,
        (str(task_id2),),
    )
    cur.execute("SELECT timezone FROM tasks WHERE id = %s", (str(task_id2),))
    assert cur.fetchone()[0] == "America/New_York"

    cur.close()
