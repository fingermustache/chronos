# 3. Test Strategy

Date: 2026-06-18

## Status

Accepted

## Context

The Chronos codebase is a Go monorepo organised around three independently
deployable services (api-gateway, scheduler, executor) with shared code in
pkg/. As the surface area of each service grows — starting with the task CRUD
implementation in Phase 1 — we need a consistent, documented approach to
testing that:

- Gives fast feedback during local development
- Catches regressions at the database boundary without requiring a running
  environment
- Avoids duplicating test effort across layers
- Stays consistent across all three services so any engineer can read a test
  file in an unfamiliar service and immediately understand what it is doing
  and why

The codebase already contains implicit conventions — testcontainers for
repository tests, httptest for handler tests, a shared testutil package — but
these have not been written down. This ADR formalises what is already in
practice and extends it to cover the service layer introduced in Phase 1.

## Decision

We adopt a three-layer test strategy aligned to the three code layers in each
service: handler, service, and repository. Each layer has a distinct scope,
tooling, and build tag.

### Layer 1 — Handler tests (unit)

**Location:** `internal/handler/*_test.go`  
**Build tag:** none (runs with plain `go test ./...`)  
**Scope:** HTTP concerns only — request parsing, response status codes,
response envelope shape, and correct delegation to the service layer.  
**Approach:** The service layer is replaced with a hand-rolled mock that
implements the service interface. No database is involved. Tests use
`net/http/httptest` and a real Chi router wired identically to `server.go` so
that routing bugs are caught alongside handler bugs.  
**What is not tested here:** Business logic, validation rules, and database
behaviour. Those belong in the layers below.

### Layer 2 — Service tests (unit)

**Location:** `internal/service/*_test.go`  
**Build tag:** none (runs with plain `go test ./...`)  
**Scope:** Business logic — validation, default application, error
translation, and correct delegation to the repository layer.  
**Approach:** The repository layer is replaced with a hand-rolled mock that
implements the repository interface. No database is involved. Each test
exercises one behaviour in isolation (e.g. a missing required field, a
negative retry count, cursor resolution).  
**What is not tested here:** HTTP concerns and SQL correctness. Those belong
in the layers above and below.

### Layer 3 — Repository tests (integration)

**Location:** `internal/repository/*_test.go`  
**Build tag:** `//go:build integration`  
**Scope:** SQL correctness — queries, constraints, soft-delete behaviour,
pagination, and anything that requires a real database to verify.  
**Approach:** A throwaway PostgreSQL 16 container is started via
testcontainers-go. All migrations are applied before the test suite runs.
A single container is shared across all tests in the package via `TestMain`
and `testutil.NewTestDBWithTeardown`. Each test function calls
`testutil.Truncate` at the start to reset relevant tables, giving full
isolation without the cost of spinning up a new container per test.  
**What is not tested here:** HTTP concerns and business logic. Those belong
in the layers above.

### Shared conventions

**Test helpers** that are used across more than one service live in
`pkg/testutil/`. Helpers that are specific to a single service live alongside
the tests in that service's package. Helpers are never exported from
`internal/` packages.

**Hand-rolled mocks** are preferred over mock generation tools (e.g. mockery,
gomock). The interfaces in this codebase are small and stable enough that
generated mocks add more indirection than they remove. If an interface grows
beyond five methods, we revisit this.

**Table-driven tests** are used when multiple inputs exercise the same code
path with different outcomes (e.g. validation rules). A flat set of named
test functions is used when each test exercises a meaningfully different
scenario (e.g. the HTTP handler tests).

**Integration tests are excluded from the default test run.** Running
`go test ./...` executes only unit tests. Integration tests require Docker
and are run explicitly:

```bash
go test -tags integration ./...
