# Go Backend Boot Sequence

Based on: https://github.com/sriniously/go-boilerplate

> Observability: **OpenTelemetry** throughout (not New Relic)

> Like CODING_SEQUENCE.md, this order is language-agnostic and doubles as
> the guideline for a future Rust rewrite — steps 3–4 in particular were
> generalized after building the Go db layer as a driver-swappable
> singleton (MongoDB active, Postgres sketched) rather than a
> Postgres-only pool.

---

## Startup Order

### 1. Env / Config
- Load `.env` via `godotenv` autoload (fires at import time)
- Parse env vars with `koanf` → unmarshal into typed `Config` struct
- Validate all fields with `go-playground/validator` — fail fast if missing

### 2. Logger + OpenTelemetry
- Init OTLP exporter + tracer provider with service name and env from config
- `defer tp.Shutdown(ctx)`
- Create structured logger (zerolog) wired to otel log bridge or stdout JSON
- All subsequent steps log through this

### 3. DB Migrations (SQL backends only)
- Run `tern` (or equivalent) migrations over a raw connection, not the pool
- Done before the pool opens so schema is ready before any service touches DB
- Skip in local env if desired — run manually via task command
- Document/schemaless stores (e.g. MongoDB) have no migration chain — the
  equivalent is an idempotent `ensureIndexes` call, which can run as part of
  step 4 instead of as its own boot phase

### 4. Database Connect
- Call the db package's singleton `Init(ctx, cfg, log)` (see
  CODING_SEQUENCE.md step 6) — connects with tuned limits (max open conns,
  idle conns, conn/idle lifetime) mapped onto whatever the active driver's
  client-options type exposes
- Attach tracing at client-construction time: a contrib tracer set on the
  pool config (`otelpgx.NewTracer()`) for SQL drivers, or a command/event
  monitor passed into the client options for drivers without a maintained
  OTel package yet (e.g. the Mongo Go driver v2 — the reference
  implementation hand-rolled a small monitor wrapper for this)
- Ping with a short internal timeout — hard fail here, before the server
  ever starts accepting requests

### 5. Redis
- Call the redis package's singleton `Init(ctx, cfg, log)` (see
  CODING_SEQUENCE.md step 8) — mirrors the db singleton pattern, but returns
  a concrete client wrapper rather than an interface (no second
  implementation is planned)
- Attach `redisotel` hooks at client-construction time for span + metric
  instrumentation
- Ping with a short timeout — **hard fail** is the recommended default: once
  the async job queue (step 6) rides on Redis, a degraded connection is a
  hard dependency anyway, so failing at boot beats a silently broken queue at
  runtime. Soft-warn only if Redis is strictly an optional cache in your
  deployment.

### 6. Async Worker (Asynq over Redis)
- Before calling `Init`, build the consumer side: an
  `asyncprocessor.Router` with every domain's task handlers registered
  against it (e.g. `router.Handle(email.TaskWelcomeEmail,
  email.NewWelcomeEmailHandler(sender, log))`). asyncprocessor itself has no
  idea what any task does — this registration step, at the composition
  root, is what wires a domain's handler to its task type.
- Call the async job package's singleton `Init(ctx, cfg, log, router)` (see
  CODING_SEQUENCE.md step 13, `internal/services/asyncprocessor`) — mirrors
  the db/redis singleton pattern. `Init` does the full boot in one call:
  `asynq.NewClientFromRedisClient`/`NewServerFromRedisClient` (both reusing
  step 5's already-connected, already-instrumented redis client — never a
  second connection pool) with queue priorities (`critical`, `default`,
  `low`), then starts the server against the caller-supplied router.
- **No wrapper goroutine needed** — `asynq.Server.Start` is non-blocking (it
  spawns its own internal goroutines and returns immediately; confirmed
  against the installed module source, not assumed).
- No synchronous connectivity check here — asynq has no `Ping` equivalent.
  This relies on step 5's redis.Init having already hard-failed against the
  same address; a narrow race window between the two is accepted, not
  silently assumed.
- Teardown: `Close(ctx)` — stops the server (draining in-flight tasks up to
  `Config.ShutdownTimeout`). It does **not** close the Redis connection —
  that connection is shared with step 5's redis singleton, which owns
  closing it (asynq refuses to close a connection it doesn't own). See step
  12 for the exact shutdown position (job before redis, since the queue
  needs Redis alive to drain).

### 7. Repositories
- Pure data-access layer
- Receives: db singleton + Redis client (via server struct, or `db.Get()`/
  `db.GetMongo()` directly)
- No business logic — only queries and cache reads/writes

### 8. Services
- Business logic layer
- Receives: repos + an `Enqueuer` interface (satisfied by
  `asyncprocessor.Get().Client`) + config
- Owns all domain rules
- A service that only *enqueues* work (e.g. UserService enqueuing a welcome
  email) needs just the `Enqueuer` — it never needs an `email.Sender`
  directly. The sender is consumer-side: it's built once at step 6, to
  register the actual handler against the router; services only ever touch
  the producer side (building/enqueuing a task). A service that needs to
  send email *synchronously* (not via the queue) would receive an
  `email.Sender` (CODING_SEQUENCE.md step 12) as a plain constructor, not a
  boot-time singleton — construct it here (or in the server struct) and
  inject it, rather than adding it to main.go's start/stop sequence
- Services depend on the narrow `Enqueuer` interface, not the async job
  package's singleton accessor directly (CODING_SEQUENCE.md step 15) — this
  is what keeps a service's business rules unit-testable without a live
  Redis; the real wiring site (server struct) passes the singleton's client
  in at construction time

### 9. Handlers
- HTTP handler structs
- Receives: services (no direct DB/Redis access)
- Maps HTTP request → service call → HTTP response

### 10. Router + Middlewares
Stack order (outer → inner):
1. Rate limiter (memory store, per-IP)
2. CORS
3. Secure headers
4. Request ID
5. `otelecho.Middleware(serviceName)` — tracing
6. Context enhancer (attach logger + request ID to ctx)
7. Request logger
8. Recover (panic → 500)

Register routes: system (`/health`, `/openapi`) + versioned group `/api/v1`

### 11. HTTP Server
- Configure `http.Server` with read / write / idle timeouts
- `ListenAndServe()` in goroutine

### 12. Graceful Shutdown (on SIGINT)
Reverse of the boot order above:
1. `httpServer.Shutdown(ctx)` — drain in-flight requests (30s timeout)
2. Close the async job singleton (`asyncprocessor.Get().Close(ctx)`) — closes
   before redis since the queue needs Redis alive to drain in-flight tasks,
   and it was opened after redis at boot (step 6 follows step 5)
3. Close the redis singleton (`redis.Get().Close(ctx)`) — closes before db
   since it was opened after db at boot (step 5 follows step 4)
4. Close the db singleton (`db.Get().Close(ctx)`)
5. Flush otel tracer provider (`tp.Shutdown`)
6. Flush logger

---

## OpenTelemetry Wiring Reference

These are the Go packages used by the reference implementation; treat the
"Concern" column as the thing every language/stack needs to wire, and the
"Go package" column as one worked example. In a Rust rewrite the equivalents
live in the `tracing`/`opentelemetry` crate family; where a driver has no
maintained OTel integration yet, fall back to its own event/command hook and
emit spans by hand (see CODING_SEQUENCE.md step 6 and BOOT_SEQUENCE.md step 4).

| Concern | Go package (reference impl) |
|---|---|
| Tracer provider + OTLP export | `go.opentelemetry.io/otel` + `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` |
| SQL query tracing (Postgres/pgx) | `go.opentelemetry.io/contrib/instrumentation/github.com/jackc/pgx/otelpgx` |
| Document-store tracing (MongoDB) | `go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/v2/mongo/otelmongo` — a command monitor passed into client options, same pattern as otelpgx |
| Redis hook | `github.com/redis/go-redis/extra/redisotel/v9` |
| Echo middleware | `go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho` |
| HTTP client tracing | `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` |
