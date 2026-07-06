# Architecture

## Overview

Chronos uses a **Service-Oriented Architecture (SOA) with Bounded Contexts**. While services share a single PostgreSQL database, each service has clearly defined boundaries and responsibilities, accessing only the data it owns.

This design balances pragmatism with architectural best practices: it's simple to implement and maintain, yet demonstrates distributed systems thinking and could easily evolve into separate databases if needed.

---

## Architecture Pattern

**Type:** Service-Oriented Architecture with Bounded Contexts  
**Database:** Shared PostgreSQL with enforced service boundaries  
**Communication:** Event-driven via RabbitMQ message queue

---

## Service Boundaries

Each service has a clearly defined bounded context with specific database access rules:

### API Gateway
**Responsibility:** Task management and user-facing API  
**Database Access:**
- Full read/write access to `tasks` table
- No access to `execution_history` table

**Operations:**
- Create, read, update, delete tasks
- Validate task configurations
- Publish task lifecycle events

---

### Scheduler Service
**Responsibility:** Time-based task scheduling and execution triggering  
**Database Access:**
- Read-only access to `tasks` table
- No access to `execution_history` table

**Operations:**
- Poll for due tasks
- Calculate next execution times
- Maintain in-memory scheduling state
- Publish execution trigger events

---

### Executor Service
**Responsibility:** Task execution and result tracking  
**Database Access:**
- No direct access to `tasks` table (receives task data via events)
- Full read/write access to `execution_history` table

**Operations:**
- Execute tasks (HTTP, command, gRPC)
- Manage worker pools
- Handle retries and timeouts
- Record execution results
- Publish completion events

---

**Operations:**
- Listen to execution completion events
- Send notifications via email, webhook, Slack
- Handle delivery retries
