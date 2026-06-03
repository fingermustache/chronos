-- src/database/seed.sql
TRUNCATE tasks CASCADE;

INSERT INTO tasks (
    id, name, schedule_type, schedule_config,
    task_type, task_config, max_retries, timeout_seconds
) VALUES
(
    gen_random_uuid(), 'Health Check',
    'cron', '{"expression": "* * * * *"}',
    'http', '{"url": "http://localhost:8080/health", "method": "GET"}',
    3, 30
),
(
    gen_random_uuid(), 'Cleanup Job',
    'interval', '{"seconds": 3600}',
    'command', '{"command": "/usr/bin/bash", "args": ["-c", "echo cleanup"]}',
    1, 60
),
(
    gen_random_uuid(), 'One-off Migration',
    'once', '{"run_at": "2026-12-01T00:00:00Z"}',
    'grpc', '{"address": "localhost:50051", "service": "Worker", "method": "Run"}',
    0, 120
);
