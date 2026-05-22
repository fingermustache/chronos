> [!WARNING]
> 🚧 **This project is under active development and not to be used for production.**

## Project Overview: "Chronos" 
### Distributed Task Scheduler

A task scheduling system, demonstrating concurrency, distributed systems patterns, and modern Go practices.

Project currently being planned in this [GitHub Project](https://github.com/users/fingermustache/projects/1).

## Architecture Design (Under Review)

| Component | Purpose | Key Technologies | Go Features Showcased |
|-----------|---------|------------------|----------------------|
| **API Gateway** | REST API for task management, authentication, request routing | Gin/Chi, JWT, rate limiting | Middleware patterns, context propagation, graceful shutdown |
| **Scheduler Service** | Task scheduling engine, triggers execution at specified times | Time-based algorithms, distributed locking | Goroutines, channels, time.Ticker, sync primitives |
| **Executor Service** | Runs scheduled tasks, manages worker pools, handles retries | Worker pool pattern, circuit breaker | Concurrency patterns, context cancellation, error handling |
| **Notification Service** | Sends alerts on task success/failure via email/webhook | SMTP, HTTP client | Interfaces for multiple providers, retry logic |
| **Shared Database** | PostgreSQL for task definitions and execution history | PostgreSQL, migrations | database/sql, prepared statements, transactions |
| **Message Queue** | RabbitMQ or Redis Streams for inter-service communication | RabbitMQ/Redis | Producer-consumer patterns, buffered channels |
