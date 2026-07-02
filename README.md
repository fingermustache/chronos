> [!WARNING]
> 🚧 **This project is under active development and not to be used for production.**

## Project Overview: "Chronos" 
### Distributed Task Scheduler

A task scheduling system, demonstrating concurrency, distributed systems patterns, and modern Go practices.

Project currently being planned in this [GitHub Project](https://github.com/users/fingermustache/projects/1).

## Architecture Design (Under Review)
- [Architecture Design Record](https://github.com/fingermustache/chronos/blob/main/adrs/0001-record-architecture-decisions.md)
- [Documentation](https://github.com/fingermustache/chronos/blob/main/docs/architecture.md)

## Use Cases

Planned demos to validate the system end-to-end once all phases are complete.

**Infrastructure maintenance** — rotate API keys, trigger nightly Postgres backups, and purge expired sessions on cron schedules.
Exercises the `cron` schedule type against real external side effects.

**SaaS workflow** — expire unpaid orders after a fixed interval, generate monthly invoices on a cron expression, and send one-time confirmation emails at a scheduled timestamp.
Exercises all three schedule types (`cron`, `interval`, `once`) together in a single domain.

**Distributed load test** — spin up 1,000 tasks across all three schedule types and run multiple scheduler instances simultaneously.
Measures dispatch latency, missed fires, and duplicate execution rate to prove `SELECT FOR UPDATE SKIP LOCKED` holds under concurrent load.
Results visualised in a Grafana dashboard showing scheduler lag and execution history over time.

---

## Database setup
cp .env.example .env

```bash
# first time setup
make db/start
make db/migrate
make db/seed

# check it worked
psql postgres://chronos:chronos@localhost:{port}/chronos -c "SELECT id, name, task_type FROM tasks;"

# full reset back to clean seeded state anytime
make db/reset
