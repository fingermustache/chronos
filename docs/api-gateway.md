# API Gateway

## Overview

The API Gateway is the entry point for all client requests to Chronos. It handles
request validation, authentication, rate limiting, and routes traffic to the
appropriate internal handlers.

**Framework:** [Chi v5](https://github.com/go-chi/chi)  
**Default port:** `8080` (configurable via `PORT` environment variable)  
**Service boundary:** Full read/write access to the `tasks` table — see [Architecture](architecture.md)

---

## Middleware Stack

Every request passes through the following middleware in order:

| Order | Middleware | What it does |
|---|---|---|
| 1 | **Recovery** | Catches panics so a single bad request cannot crash the server. Logs the panic value and stack trace, returns `500`. Outermost so it protects all subsequent middleware. |
| 2 | **RequestID** | Stamps every request with a unique ID. Reads `X-Request-ID` from the incoming request if present (preserves upstream trace IDs), otherwise generates one with `crypto/rand`. Threads the ID through `context.Context` and echoes it in the response header. |
| 3 | **Logger** | Logs method, path, status code, and wall-clock duration for every request using `log/slog`. Captures the status code via a wrapped `ResponseWriter`. |
| 4 | **CORS** | Sets `Access-Control-Allow-*` headers on every response. Returns `204 No Content` immediately for `OPTIONS` preflight requests so they never reach route handlers. |
| 5 | **RateLimiter** | Limits requests per IP per minute (default: 60 RPM, configurable via `RATE_LIMIT_RPM`). Uses an in-memory sliding window with `sync.Mutex`. Respects `X-Forwarded-For` and `X-Real-IP` for clients behind a load balancer. Returns `429 Too Many Requests` with a `Retry-After: 60` header when the limit is exceeded. |
| 6 | **Validation** | Enforces `Content-Type: application/json` on `POST`, `PUT`, and `PATCH` requests with a body. Returns `415 Unsupported Media Type` on content-type violations. Enforces a 1 MiB body size limit; returns `413 Request Entity Too Large` when exceeded. |
| 7 | **Auth** | Validates Bearer token format on all protected routes. Phase 1: structural check only (verifies `Authorization: Bearer <token>` shape). Phase 2: will verify the JWT signature. |

Auth is scoped to the **protected route group only** — public routes like `/health`
bypass it entirely at the router level.

---

## Routes

### Public

| Method | Path | Handler | Auth required |
|---|---|---|---|
| `GET` | `/health` | `handler.Health` | No |

### Protected

All protected routes require `Authorization: Bearer <token>`.

| Method | Path | Handler | Description |
|---|---|---|---|
| `POST` | `/tasks` | `handler.Create` | Create a new task |
| `GET` | `/tasks` | `handler.List` | List tasks with cursor-based pagination (`limit`, `cursor` query params) |
| `GET` | `/tasks/{id}` | `handler.GetByID` | Get a single task by UUID |
| `PUT` | `/tasks/{id}` | `handler.Update` | Update one or more fields on an existing task |
| `DELETE` | `/tasks/{id}` | `handler.Delete` | Soft-delete a task |

---

## Schedule Types

Every task requires a `schedule_type` and a matching `schedule_config` object.
The API validates both fields on create and on any update that supplies `schedule_config`
(which must always be accompanied by `schedule_type`).

### `cron`

Runs on a standard five-field cron schedule (minute hour dom month dow).

```json
{
  "schedule_type": "cron",
  "schedule_config": { "expression": "0 9 * * 1-5" }
}
```

- `expression` — required, non-empty string.
- Must be a syntactically and semantically valid five-field cron expression.
- Second-field and year-field extensions are not accepted.

### `interval`

Runs repeatedly at a fixed interval expressed as a whole number of seconds.

```json
{
  "schedule_type": "interval",
  "schedule_config": { "seconds": 3600 }
}
```

- `seconds` — required, positive integer (e.g. `300` = every 5 minutes, `86400` = every 24 hours).
- Zero and negative values are rejected.

### `once`

Runs exactly once at the specified timestamp.

```json
{
  "schedule_type": "once",
  "schedule_config": { "run_at": "2026-09-01T09:00:00Z" }
}
```

- `run_at` — required, RFC3339 timestamp string.
- Must be in the future at the time of the request.

### `next_execution_time`

All tasks include a `next_execution_time` field in API responses (RFC3339, UTC).
It is computed and persisted by the API gateway at create time and recalculated on any update that changes `schedule_type` or `schedule_config`.

| `schedule_type` | How `next_execution_time` is set |
|---|---|
| `cron` | Next occurrence of the expression after the request timestamp |
| `interval` | Request timestamp + `seconds` |
| `once` | Exactly the `run_at` value |

The scheduler polls this field to determine which tasks are due.
After a task fires, the scheduler updates `next_execution_time` for `interval` tasks and sets `enabled = false` for `once` tasks.

---

## Configuration

All configuration is loaded from environment variables via `config.Load()`.
Defaults are safe for local development.

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Port the HTTP server listens on |
| `RATE_LIMIT_RPM` | `60` | Max requests per IP per minute |

---

## Graceful Shutdown

The server handles `SIGTERM` (Kubernetes/Docker) and `SIGINT` (Ctrl+C).

On signal:
1. Stops accepting new connections
2. Waits up to **30 seconds** for in-flight requests to complete
3. Forces close if the deadline is exceeded
4. Logs `server stopped cleanly` on success

Startup errors (e.g. port already in use) are surfaced immediately via a
dedicated error channel and cause the process to exit with code `1`.

---

## HTTP Server Timeouts

Configured in `server.New()` to prevent slow clients from holding connections open.

| Timeout | Value | Description |
|---|---|---|
| `ReadTimeout` | `15s` | Time to read the full request including body |
| `WriteTimeout` | `15s` | Time to write the full response |
| `IdleTimeout` | `60s` | Keep-alive connection idle time |

---

## Request Tracing

Every request carries a unique ID through its full lifecycle:
Client request
- RequestID middleware sets X-Request-ID in context
- Logger reads it from context when logging
- Recovery reads it from context when logging panics
- Auth reads it from context when logging auth events
- Rate limiter reads it from context when logging 429s

## Project Structure

```
src/api-gateway/
├── cmd/
│   └── main.go               # Entry point — wires config, server, shutdown
└── internal/
    ├── config/
    │   └── config.go         # Env var loading with typed defaults
    ├── handler/
    │   ├── health.go         # GET /health handler
    │   └── task.go           # Task CRUD handlers (create, list, get, update, delete)
    ├── middleware/
    │   ├── auth.go           # Bearer token validation (Phase 1 stub)
    │   ├── cors.go           # Cross-origin headers + preflight handling
    │   ├── logging.go        # Structured request/response logging
    │   ├── ratelimit.go      # IP-based rate limiting
    │   ├── recovery.go       # Panic recovery
    │   ├── tracing.go        # Request ID generation and context threading
    │   └── validation.go     # Content-Type enforcement
    └── server/
        └── server.go         # Router wiring, middleware chain, HTTP server config
```

---

## Running Locally

```bash
cd src
go run ./api-gateway/cmd/main.go
```

With custom config:

```bash
PORT=9090 RATE_LIMIT_RPM=100 go run ./api-gateway/cmd/main.go
```

Check the health endpoint:

```bash
curl http://localhost:8080/health
# {"status":"ok","timestamp":"2026-06-11T10:00:00Z"}
```
