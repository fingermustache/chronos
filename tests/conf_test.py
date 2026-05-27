# tests/conftest.py
import os
import pytest
import psycopg2

DB_DSN = os.getenv("TEST_DATABASE_URL", "postgresql://postgres:postgres@localhost:5432/test_db")
MIGRATIONS_DIR = os.path.join(os.path.dirname(__file__), "../src/database/migrations")

def read_migration(filename):
    return open(os.path.join(MIGRATIONS_DIR, filename)).read()

@pytest.fixture(scope="session")
def db_conn():
    conn = psycopg2.connect(DB_DSN)
    conn.autocommit = True

    cur = conn.cursor()
    cur.execute(read_migration("000001_create_tasks_table.up.sql"))
    cur.execute(read_migration("000002_create_execution_history_table.up.sql"))
    cur.close()

    yield conn

    cur = conn.cursor()
    cur.execute(read_migration("000002_create_execution_history_table.down.sql"))
    cur.execute(read_migration("000001_create_tasks_table.down.sql"))
    cur.close()
    conn.close()

@pytest.fixture
def conn(db_conn):
    """Each test gets its own transaction, rolled back on teardown."""
    db_conn.autocommit = False
    yield db_conn
    db_conn.rollback()
    db_conn.autocommit = True
