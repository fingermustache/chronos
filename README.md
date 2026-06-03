> [!WARNING]
> 🚧 **This project is under active development and not to be used for production.**

## Project Overview: "Chronos" 
### Distributed Task Scheduler

A task scheduling system, demonstrating concurrency, distributed systems patterns, and modern Go practices.

Project currently being planned in this [GitHub Project](https://github.com/users/fingermustache/projects/1).

## Architecture Design (Under Review)
- [Architecture Design Record](https://github.com/fingermustache/chronos/blob/main/adrs/0001-record-architecture-decisions.md)
- [Documentation](https://github.com/fingermustache/chronos/blob/main/docs/architecture.md)

## Database setup

```bash
# first time setup
make db/start
make db/migrate
make db/seed

# check it worked
psql postgres://chronos:chronos@localhost:5432/chronos -c "SELECT id, name, task_type FROM tasks;"

# full reset back to clean seeded state anytime
make db/reset
