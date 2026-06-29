# Go Backend Boot Sequence

Based on: https://github.com/sriniously/go-boilerplate

> Observability: **OpenTelemetry** throughout (not New Relic)

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

### 3. DB Migrations
- Run `tern` migrations over a raw `pgx` connection (not the pool)
- Done before the pool opens so schema is ready before any service touches DB
- Skip in local env if desired — run manually via task command

### 4. Database Pool
- Open `pgxpool` with tuned limits: max open conns, idle conns, conn lifetime, idle time
- Attach `otelpgx.NewTracer()` for query tracing
- Ping with timeout — hard fail here

### 5. Redis
- Open `go-redis` client
- Attach `redisotel` hook for span instrumentation
- Ping with 5s timeout — decide: hard fail or soft warn based on your dependency needs

### 6. Async Worker (Asynq over Redis)
- `asynq.NewClient` (for enqueuing jobs)
- `asynq.NewServer` (for consuming jobs) with queue priorities: `critical`, `default`, `low`
- Register task handlers on mux
- `server.Start()` in background goroutine
- `defer jobService.Stop()` on shutdown

### 7. Repositories
- Pure data-access layer
- Receives: DB pool + Redis client (via server struct)
- No business logic — only queries and cache reads/writes

### 8. Services
- Business logic layer
- Receives: repos + job client (for enqueuing) + config
- Owns all domain rules

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
1. `httpServer.Shutdown(ctx)` — drain in-flight requests (30s timeout)
2. Close DB pool
3. Asynq server shutdown + client close
4. Flush otel tracer provider (`tp.Shutdown`)
5. Flush logger

---

## OpenTelemetry Wiring Reference

| Concern | Package |
|---|---|
| Tracer provider + OTLP export | `go.opentelemetry.io/otel` + `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` |
| pgx query tracing | `go.opentelemetry.io/contrib/instrumentation/github.com/jackc/pgx/otelpgx` |
| Redis hook | `github.com/redis/go-redis/extra/redisotel/v9` |
| Echo middleware | `go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho` |
| HTTP client tracing | `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` |
