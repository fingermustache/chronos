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

**Distributed load test (done)** — 1,000 tasks across all three schedule types, 3 concurrent scheduler instances.
Zero duplicate executions and zero missed fires across 1,990 trigger events, proving `SELECT FOR UPDATE SKIP LOCKED` holds under concurrent load; p99 dispatch latency 20.8ms.
Full methodology and raw data in [docs/load-test.md](docs/load-test.md).
A Grafana dashboard visualising scheduler lag over time is still planned.

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
