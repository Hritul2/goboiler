# Go Backend — Coding Sequence

The rule: write things with zero internal dependencies first.
Each layer only imports from layers above it in this list.

> This sequence is written against the Go reference implementation
> (`goboiler`), but the layering and ordering is language-agnostic — it
> applies just as well to a Rust rewrite (modules/crates instead of
> packages, traits instead of interfaces). Where a step names a concrete
> Go package or library, treat it as an illustration of the pattern, not
> a hard requirement. Naming lessons learned building the Go version are
> called out inline so a fresh repo can pick better names up front.

---

## 1. Project Scaffold
- `go mod init`
- Directory structure: `cmd/`, `internal/`, `static/`, `templates/`
- `.env.sample`, `.gitignore`, `Taskfile.yml` (or Makefile)

## 2. Config
`internal/config/config.go`
- Define all config structs (`ServerConfig`, `DbConfig`, `RedisConfig`, `AuthConfig`, etc.)
- Write the `LoadConfig()` loader (koanf + godotenv + validator)
- No other internal package imports — this is the root of everything
- Naming note: in the Go reference this ended up at `internal/utils/config`
  rather than `internal/config` — a `utils` catch-all crept in early. Decide
  the top-level package name deliberately up front instead.

## 3. Error Types
`internal/errors/types.go` + `internal/errors/http.go`
- Define your domain error types (`NotFound`, `Unauthorized`, `Conflict`, etc.)
- Write the HTTP error mapper
- Depends on nothing internal — define these early so every layer can use them
- Naming note: calling the package `errs` (as originally planned here) reads
  well at the definition site but forces every importer to alias it anyway
  (`errs "github.com/.../errors"`) because it collides with the standard
  library's `errors` package. The reference implementation names the package
  `errors` and standardizes on the `errs` alias only at import sites — pick
  one convention and document it in the package doc comment.

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
`internal/db/db.go`
- Define a store-agnostic `Database` interface — the minimum every backing
  store must satisfy (e.g. `Ping`, `Close`, `DriverName`). Keep it thin;
  push anything store-specific (collection/table access, transactions) onto
  the concrete type, not the interface.
- Wire it as an **explicit singleton**: `Init(ctx, cfg, log)` connects, pings
  with a short hard-fail deadline, and installs the instance; `Get()` returns
  it and panics if called before `Init` (a boot-order bug, not a runtime
  condition worth a soft error). Avoid a lazy `sync.Once`-on-first-use
  singleton — it defeats fail-fast startup by moving connection failures
  from boot time to first request.
- One file per driver, all satisfying the same interface, e.g.
  `internal/db/mongo.go` (active) and `internal/db/postgres.go` (sketched,
  commented out until adopted). This keeps the swap between document and
  relational stores — or between Go and Rust driver crates in a rewrite — a
  one-file, one-function change (`newMongo` → `newPostgres` inside `Init`).
- Attach tracing instrumentation at client-construction time, not after —
  e.g. a command/event monitor passed into the client options builder, or an
  `otelpgx`-style tracer set on the pool config. If no maintained OTel contrib
  package exists yet for a given driver (true for the Mongo Go driver v2 at
  the time of writing), write a small wrapper around the driver's
  command-monitor/event-listener hook that emits spans — same shape as
  `otelpgx`, just hand-rolled.
- `Ping()` inside `Init`, not deferred to first query — hard fail on an
  unreachable deployment.

`internal/db/migrations/*.sql` (SQL backends only)
- `tern` (or equivalent) migrator over a raw connection, run before the pool
  opens.
- Document stores (Mongo) are schema-less — there is no migrations step;
  the closest equivalent is an idempotent index-creation routine run once at
  boot (`ensureIndexes`), not a versioned migration chain.

## 7. Driver Error Mapping
`internal/db/errors.go`
- Map driver-specific errors → your domain error types (`errs`), colocated
  with the db adapter rather than a separate top-level package — the mapping
  is inherently driver-aware (pgx error codes vs. Mongo write-exception codes
  vs. a Rust driver's error enum), so it lives next to the code that
  constructs those errors.
- e.g. Postgres `unique_violation` → `errs.Conflict`; Mongo write-exception
  code `11000` → the same sentinel. Classify specific error classes first,
  transport/timeout classes last, and fall back to a safe internal error for
  anything unrecognized.
- Depends on: errors

## 8. Redis
`internal/redis/redis.go`
- Wire it as a singleton mirroring step 6's db pattern: `Init(ctx, cfg, log)` +
  `Get()` (panic before Init), pinging with a hard-fail deadline.
- Unlike db, expose a **concrete wrapper, not an interface** — there is no
  second Redis implementation planned, and downstream consumers (the async
  job queue, rate-limit middleware) need the raw client directly. Don't
  introduce an abstraction for a single implementation.
- `internal/redis/errors.go` — driver-error normalisation colocated with the
  client (same split as step 7's `db/errors.go`), mapping cache-miss/
  timeout/unavailable onto sentinels.
- Attach `redisotel` tracing + metrics hooks at client-construction time (same
  principle as step 6's tracing guidance).
- Depends on: config, errors

## 9. Base Model
`internal/models/base.go`
- Naming note: the doc originally said `internal/model` (singular); the
  reference implementation uses `internal/models` (plural) — pick one
  convention and apply it consistently to the model and repository
  directories (step 11 has the same singular/plural choice to make).
- Shared fields: `ID` (the driver-native ID type — e.g. `bson.ObjectID` for
  Mongo), `CreatedAt`, `UpdatedAt` (UTC) + a `Touch()` helper that bumps
  `UpdatedAt` before a mutating write.
- Shared pagination utility: a `Pagination{Page, Limit}` struct with
  `Normalize()` (clamp to safe defaults and a max limit) and `Skip()` (offset
  calculation) — centralising this here means every repository's list method
  shares one clamp policy instead of reimplementing it.
- Depends on nothing internal

## 10. Domain Models
`internal/models/<domain>.go`
- One file per domain entity (e.g. `user.go`, `workspace.go`)
- Embed the base model using the driver's flatten idiom (e.g. `bson:",inline"`
  for Mongo) so persisted documents don't nest a `"base"` sub-object — this is
  easy to get silently wrong, so round-trip it through a marshal/unmarshal
  test rather than trusting the tag.
- Hide sensitive fields from API responses at the model layer (e.g.
  `json:"-"` on a password hash) rather than relying on every handler to
  remember to omit them.
- Pure structs — no DB or HTTP logic here

## 11. Repositories
`internal/repo/<domain>.go`
`internal/repo/repositories.go`
- Naming note: the doc originally said `internal/repository` (singular); the
  reference implementation shortened it to `internal/repo` — same
  singular/plural/abbreviation drift as step 6's `database` → `db`. Pick the
  final name deliberately rather than letting it happen mid-implementation.
- One repo per domain, interface + implementation
- All data access lives here — via `db.Get()`/`db.GetMongo()` (or the
  equivalent typed accessor for whichever driver is active)
- Index/schema setup: expose an explicit `EnsureIndexes(ctx)` per repo,
  invoked once by the aggregate constructor (`NewRepositories`) rather than
  hidden inside each repo's per-request constructor — keeps index creation
  fail-fast at wiring time instead of silent background I/O on every
  instantiation.
- Testing note: a concrete driver type (e.g. `*mongo.Collection`) can't be
  faked without an abstraction layer worse than the problem it solves.
  Factor the pure logic — index specs, sort/pagination options — into
  standalone functions and unit-test those; verify the CRUD glue itself
  against a real local instance (docker compose) rather than building a mock
  collection interface just for test purity.
- Use the db package's error mapper (step 7) to map driver errors to domain
  errors before returning them
- Depends on: db, models, errors

## 12. Email Client
`internal/lib/email/client.go`
- SMTP client init — the reference implementation uses standard SMTP via
  `wneessen/go-mail` (the Go "nodemailer equivalent"), so any provider works
  (Zoho, Gmail app password, AWS SES SMTP). A vendor SDK like Resend is a
  valid alternative behind the same `Sender` interface.
- Plain constructor, not a singleton — SMTP dials per send, so there is no
  connection state worth boot-time management; a disabled config returns a
  logging no-op sender so local dev boots without credentials.
- `internal/lib/email/templates.go` — embedded HTML templates (`html/template`
  for escaping)
- `internal/lib/email/emails.go` — typed send functions (e.g. `SendWelcome`)
- `internal/lib/email/tasks.go` — this domain's async producer + consumer
  (step 13's task type constant, payload struct, `New<X>Task` producer, and
  `New<X>Handler` consumer factory), colocated with the synchronous `Sender`
  it wraps rather than living inside the queue package. The consumer
  factory's returned func matches asyncprocessor's `HandlerFunc` shape
  structurally — no import of asyncprocessor needed here, dependency runs
  the other way (composition root imports both and wires them together).
- Depends on: config (and asynq, inherently, for producing/consuming tasks —
  not something worth abstracting away absent a second queue library)

## 13. Async Job Service
`internal/services/asyncprocessor/job.go`
- Library: `hibiken/asynq` over Redis — confirmed by reading the reference
  implementation's (github.com/sriniously/go-boilerplate) go.mod directly
  rather than assuming a version.
- Naming note: the doc originally said `internal/lib/job` (matching the
  reference exactly). The final choice moved it under `internal/services/`
  as `asyncprocessor` instead — this repo groups the async job **service**
  with the domain services (`internal/services/user`) rather than with
  passive infra clients (`internal/lib/email`), since it owns a running
  background worker with its own lifecycle, not just an outbound client.
  `internal/services/` uses one subpackage per concern (`asyncprocessor`,
  `user`), plus a top-level `services.go` aggregator.
- Wire it as a **singleton** (`Init`/`Get`, mirroring step 6/8's db/redis
  pattern) rather than the reference's plain-constructor-held-on-a-struct
  design — this repo has no server struct yet, and the established idiom
  here for boot-managed cross-cutting infra is the package singleton.
  `Init` does the **full boot** in one call — client, server, and starting
  consumption — since `asynq.Server.Start` is **non-blocking** (spawns its
  own internal goroutines and returns immediately; confirmed by reading the
  installed module source, not assumed). No separate exported `Start()`
  step.
- **Stay 100% domain-agnostic — this package must never import anything
  domain-specific (email, notifications, exports, whatever).** The first
  version of this step hardcoded a single email task directly into `job.go`
  (`Init` built an `email.Sender` itself, registered one handler inline) —
  that works for exactly one task kind and forces every new job type to
  edit `job.go`'s imports and constructor signature. Instead, split
  producer/consumer properly:
  - `internal/services/asyncprocessor/router.go` — a `HandlerFunc` type
    (`func(ctx, *asynq.Task) error`, named to parallel `http.HandlerFunc`)
    and a `Router` (thin wrapper over `asynq.ServeMux`) with one method,
    `Handle(taskType string, h HandlerFunc)`. This is "the service which
    routes the task to the right place" — the single place dispatch is
    decided, with zero knowledge of what any task does.
  - `Init(ctx, cfg, log, router *Router)` takes a **caller-supplied**
    Router and calls `server.Start(router.mux)` — the caller (composition
    root) builds the router and registers every domain's handlers against
    it *before* calling `Init`.
  - Each domain owns its own task type constant, payload struct, a
    **producer** (`New<X>Task(...) (*asynq.Task, error)`, pure, unit-tested)
    and a **consumer factory** (`New<X>Handler(deps...) func(ctx, *asynq.Task)
    error`, closing over whatever the handler needs — e.g. email's lives in
    `internal/lib/email/tasks.go`, right next to the synchronous `Sender` it
    wraps). Adding a second job kind later means adding one new domain file
    plus one `router.Handle(...)` call at the composition root — zero
    changes to asyncprocessor itself.
- asynq has no `Ping` equivalent, so `Init` cannot hard-fail on an
  unreachable Redis the way db/redis do — it relies on redis's own Init
  having already hard-failed against the same address earlier in boot.
  Document this narrow gap rather than silently assuming connectivity.
- **Reuse the redis singleton's connection — don't build a second one.**
  The reference implementation (and most asynq tutorials) construct the job
  queue straight from `asynq.RedisClientOpt{Addr, Password, DB}`, which
  looks harmless but silently opens an entirely separate connection pool to
  the same Redis server, bypassing whatever tracing/instrumentation the
  step-8 redis singleton already attached (`redisotel` here) and doubling
  live connections for no reason. Instead, once step 8's redis singleton
  exists, fetch its live client (`redis.Get().Client()`, a
  `redis.UniversalClient`) and pass it to `asynq.NewClientFromRedisClient`/
  `NewServerFromRedisClient` — asynq supports this exact pattern precisely
  so callers can share a connection. One consequence: asynq then refuses to
  close that connection itself (`Client.Close()` returns an error for a
  shared connection) — ownership of closing it stays with whichever
  package opened it (redis's `Close`, later in the shutdown order), so the
  job service's own `Close` only stops its server, nothing else.
- `asynq.Option` is a public interface, so a producer's task-construction
  options (retry count, queue, timeout) are unit-testable, unlike a concrete
  driver type. `Router.Handle`'s dispatch is unit-testable too — register a
  fake handler, build a task, invoke the underlying mux directly — with no
  live Redis needed; only `Init`'s real dial+`Start` is integration-verified
  manually.
- A consumer handler that only calls an injected interface (e.g.
  `email.Sender`) is fully unit-testable with no live Redis.
- Depends on: config, redis. (Domains depend on *this* package's
  `HandlerFunc`/`Router` shape to register themselves — the dependency runs
  domain → asyncprocessor, never the other way.)

## 14. Validation Utils
`internal/validation/validation.go`
- The reference implementation actually uses `go-playground/validator`
  (struct tags), not a true schema validator — if you want a real
  "Zod for Go" equivalent (fluent chainable schemas, e.g.
  `zog.String().Email().Required()`), pick a dedicated library
  (e.g. `github.com/Oudwins/zog`) rather than assuming the reference's
  approach is what "zod equivalent" means.
- Keep validation errors as a small package-local type (e.g. `Errors
  map[string][]string`) rather than extending the core `errors` package —
  the validation library's issue shape is its own concern; translate to
  `errs.BadRequest(...)` only at the call site (service today, handlers
  later at step 18).
- `internal/validation/<domain>.go` — one schema + input struct pair per
  domain (e.g. `UserCreateSchema`/`UserCreateInput`), imported directly by
  the matching service as its method parameter type — no duplicate DTO.
- Depends on nothing internal (or just errors)

## 15. Services
`internal/services/<domain>/<domain>.go`
`internal/services/services.go`
- Naming note: the doc originally said `internal/service` (singular,
  domain-service files flat in one package). The final choice consolidated
  every service — domain services and infra services like the async job
  queue — under one `internal/services/` tree, one subpackage per concern,
  with a top-level `services.go` aggregator importing each subpackage. This
  also absorbed the stale, never-implemented `internal/services/{authc,
  authz,opentelemetry,server}` scaffold slots that predated this step —
  `opentelemetry`/`server` were already superseded by `internal/observability`
  and the future step-16 `internal/server`.
- Business logic — call repos, enqueue jobs, send emails
- One subpackage per domain
- Depend on repo **interfaces** (never concrete driver types) and narrow
  interfaces for cross-cutting singletons — e.g. an `Enqueuer` interface
  satisfied by `*asynq.Client`, rather than calling the async job package's
  `Get()` singleton directly from inside a service. Without this, a service
  method that enqueues work is untestable without live Redis. The real
  wiring site (step 16) still passes the singleton's client into the
  constructor; the service itself never imports the singleton accessor.
- Depends on: repos, job client, email client, validation, errors

## 16. Server Struct
`internal/server/server.go`
- The central wiring struct: holds DB, Redis, Job, Logger, Config, OTel
- `New()` initialises and returns it
- `Shutdown()` tears everything down in reverse order
- Depends on: db, redis, job, logger, config, observability

## 17. Middleware
`internal/middleware/global.go` — CORS, secure headers, recover, request logger, error handler
`internal/middleware/request_id.go`
`internal/middleware/tracing.go` — `otelecho` wiring
`internal/middleware/auth.go` — JWT validation
`internal/middleware/context.go` — attach logger + request ID to ctx
`internal/middleware/rate_limit.go`
`internal/middleware/middlewares.go` — aggregate struct
- Depends on: server, errors

## 18. Handlers
`internal/handler/<domain>.go`
`internal/handler/handlers.go`
`internal/handler/health.go`
- Parse + validate request → call service → write response
- Depends on: services, errors, validation

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
  └── errors (imported as `errs`)
  └── db (driver-agnostic interface + error mapping) ──┐
  └── redis                                            │
  └── models                                           │
        └── repo ────────────────────────────────────┤
  └── email                                            │
  └── validation                                       │
  └── services/asyncprocessor (asynq+redis)            │
        └── services/<domain> ────────────────────────┤
              └── handler────────────────────────────┤
                    └── router
                          └── server struct
                                └── main.go
```

Note: `email` and `services/asyncprocessor` have no edge between them — neither
imports the other. main.go (the composition root, future server struct) is
where they meet: it builds an `email.Sender`, registers
`email.NewWelcomeEmailHandler(sender, log)` against an
`asyncprocessor.Router` for `email.TaskWelcomeEmail`, then passes that
router into `asyncprocessor.Init`. A future job kind follows the same shape
without touching either existing package.
