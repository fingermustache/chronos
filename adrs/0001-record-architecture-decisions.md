# 1. Use Service-Oriented Architecture with Bounded Contexts

Date: 2026-05-21

## Status

Accepted

## Context

Chronos requires separation between task management, scheduling, execution, and notifications. Need to balance distributed systems best practices with maintainability for a personal project.

## Decision

Implement **Service-Oriented Architecture with Bounded Contexts**:
- **Shared PostgreSQL database** with enforced service boundaries
- **Event-driven communication** via RabbitMQ
- **Four services** with strict data access rules

### Service Boundaries

| Service | Database Access | Responsibility |
|---------|----------------|----------------|
| **API Gateway** | `tasks` table (R/W) | Task CRUD, publish lifecycle events |
| **Scheduler** | `tasks` table (read + `next_execution_time` only) | Poll due tasks, publish trigger events |
| **Executor** | `execution_history` table (R/W) | Execute tasks, record results |
| **Notification** | None (event-driven only) | Send alerts on completion |

## Consequences

### Positive
- Clear separation of concerns with independent scaling
- Simple to implement (shared DB) while demonstrating distributed systems thinking
- Easy to evolve into separate databases if needed
- Portfolio demonstrates SOA and bounded context patterns

### Negative
- Shared database is single point of failure
- Potential database bottleneck under high load

## Alternatives Considered

**Monolithic:** Rejected - doesn't show distributed systems skills  
**Microservices with separate DBs:** Rejected - too complex for personal project  
**Serverless:** Rejected - vendor lock-in, cold start issues
