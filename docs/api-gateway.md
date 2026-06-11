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
| 6 | **Validation** | Enforces `Content-Type: application/json` on `POST`, `PUT`, and `PATCH` requests. Returns `415 Unsupported Media Type` on violations. |
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

No protected routes are implemented yet — the route group and auth middleware
are in place ready for Phase 2 task endpoints.

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
    │   └── health.go         # GET /health handler
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
