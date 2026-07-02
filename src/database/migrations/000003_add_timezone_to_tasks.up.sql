ALTER TABLE tasks
    ADD COLUMN timezone VARCHAR(64) DEFAULT NULL;

COMMENT ON COLUMN tasks.timezone IS
    'IANA timezone name (e.g. America/New_York). NULL means UTC. Affects cron schedule evaluation only.';
