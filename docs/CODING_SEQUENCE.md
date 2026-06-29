# Go Backend — Coding Sequence

The rule: write things with zero internal dependencies first.
Each layer only imports from layers above it in this list.

---

## 1. Project Scaffold
- `go mod init`
- Directory structure: `cmd/`, `internal/`, `static/`, `templates/`
- `.env.sample`, `.gitignore`, `Taskfile.yml` (or Makefile)

## 2. Config
`internal/config/config.go`
- Define all config structs (`ServerConfig`, `DatabaseConfig`, `RedisConfig`, `AuthConfig`, etc.)
- Write the `LoadConfig()` loader (koanf + godotenv + validator)
- No other internal package imports — this is the root of everything

## 3. Error Types
`internal/errs/types.go` + `internal/errs/http.go`
- Define your domain error types (`NotFound`, `Unauthorized`, `Conflict`, etc.)
- Write the HTTP error mapper
- Depends on nothing internal — define these early so every layer can use them

## 4. Logger
`internal/logger/logger.go`
- Zerolog setup with console (dev) and JSON (prod) writers
- OTel log bridge if you want unified log shipping
- Depends only on config

## 5. OpenTelemetry
`internal/observability/otel.go`
- Init tracer provider + OTLP exporter
- Resource with service name + env from config
- Expose `Shutdown()` for main to defer
- Depends on config + logger

## 6. Database
`internal/database/database.go`
- pgxpool setup with connection limits
- Attach `otelpgx` tracer
- `Ping()` on init

`internal/database/migrator.go`
- `tern` migrator over raw `pgx` connection (separate from pool)

`internal/database/migrations/*.sql`
- Write migration files here as you define models

## 7. SQL Error Handler
`internal/sqlerr/error.go` + `internal/sqlerr/handler.go`
- Map pgx error codes → your domain error types (`errs`)
- e.g. `unique_violation` → `errs.Conflict`
- Depends on: errs

## 8. Redis
`internal/redis/redis.go` (or inline in server)
- `go-redis` client init
- Attach `redisotel` hook
- Ping on init

## 9. Base Model
`internal/model/base.go`
- Shared fields: `ID`, `CreatedAt`, `UpdatedAt`
- Any shared model utilities (pagination, sort params)
- Depends on nothing internal

## 10. Domain Models
`internal/model/<domain>.go`
- One file per domain entity (e.g. `user.go`, `workspace.go`)
- Pure structs — no DB or HTTP logic here

## 11. Repositories
`internal/repository/<domain>.go`
`internal/repository/repositories.go`
- One repo per domain, interface + implementation
- All SQL lives here
- Use `sqlerr` to map DB errors to domain errors
- Depends on: database, model, sqlerr, errs

## 12. Email Client
`internal/lib/email/client.go`
- Resend (or SMTP) client init
- `internal/lib/email/templates.go` — load HTML templates
- `internal/lib/email/emails.go` — typed send functions (e.g. `SendWelcome`)
- Depends on: config

## 13. Async Job Service
`internal/lib/job/job.go`
- `asynq.NewClient` + `asynq.NewServer`
- Queue priority config

`internal/lib/job/handlers.go`
- Task handler implementations (receive Redis message → do work)

`internal/lib/job/email_tasks.go`
- Task type constants + payload structs for email jobs
- Depends on: config, email client

## 14. Validation Utils
`internal/validation/utils.go`
- Custom validator registration
- Request parsing + validation helpers
- Depends on nothing internal (or just errs)

## 15. Services
`internal/service/<domain>.go`
`internal/service/services.go`
- Business logic — call repos, enqueue jobs, send emails
- One service per domain
- Depends on: repos, job client, email client, errs

## 16. Server Struct
`internal/server/server.go`
- The central wiring struct: holds DB, Redis, Job, Logger, Config, OTel
- `New()` initialises and returns it
- `Shutdown()` tears everything down in reverse order
- Depends on: database, redis, job, logger, config, observability

## 17. Middleware
`internal/middleware/global.go` — CORS, secure headers, recover, request logger, error handler
`internal/middleware/request_id.go`
`internal/middleware/tracing.go` — `otelecho` wiring
`internal/middleware/auth.go` — JWT validation
`internal/middleware/context.go` — attach logger + request ID to ctx
`internal/middleware/rate_limit.go`
`internal/middleware/middlewares.go` — aggregate struct
- Depends on: server, errs

## 18. Handlers
`internal/handler/<domain>.go`
`internal/handler/handlers.go`
`internal/handler/health.go`
- Parse + validate request → call service → write response
- Depends on: services, errs, validation

## 19. Router
`internal/router/router.go`
- Mount middlewares in order (see BOOT_SEQUENCE.md step 10)
- Register all routes
- Depends on: server, handlers, middleware, services

## 20. main.go
`cmd/<appname>/main.go`
- Wire it all: config → logger → otel → server.New() → repos → services → handlers → router → http.Server → graceful shutdown
- Sequence mirrors BOOT_SEQUENCE.md exactly

---

## Dependency Graph (simplified)

```
config
  └── logger
  └── observability (otel)
  └── errs
        └── sqlerr
  └── database ──────────┐
  └── redis              │
  └── model              │
        └── repository ──┤
  └── email              │
  └── job (asynq+redis)  │
        └── service ─────┤
              └── handler─┤
                    └── router
                          └── server struct
                                └── main.go
```
