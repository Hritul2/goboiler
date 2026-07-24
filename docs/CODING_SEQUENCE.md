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
- **Observability note: deliberately no manual tracing spans or logging
  added here.** Once step 6's driver-level auto-instrumentation
  (`otelmongo`) is attached, every query this layer issues already gets its
  own span with real query/collection/duration detail — a manual
  `tracer.Start("UserRepo.Create")` wrapped around that produces a
  redundant parent span containing exactly one child with no new
  information. A debug log per query would fire at request volume with no
  context this layer has that the calling service (which does log, see
  step 15) doesn't already have. This is a deliberate choice, not an
  oversight — don't reflexively "instrument everything."
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
- **`Router.Handle` wraps every registered handler in an OTel span
  automatically** (named `"task."+taskType`), rather than leaving each
  domain to reimplement tracing itself — this is the one generic place
  that sees every task type, so it's the right layer for the *generic*
  timing/pass-fail signal. **No log line** is added at this layer,
  deliberately: a router-level "task X ran" log with no domain fields
  would just duplicate what the domain handler already logs with actually
  useful fields (e.g. email's `to`). Split: domain-specific log fields
  stay at the domain level (unchanged), generic pass/fail becomes a span
  only, at the router.
- Depends on: config, redis, observability (for the package tracer). (Domains
  depend on *this* package's `HandlerFunc`/`Router` shape to register
  themselves — the dependency runs domain → asyncprocessor, never the
  other way.)

## 14. Validation Utils
`internal/validation/validation.go`
- The project briefly ran two validation libraries — `go-playground/validator`
  for boot-time config and `github.com/Oudwins/zog` (a "Zod for Go" schema
  builder) for request/service input — and consolidated on
  `go-playground/validator` alone for both, to avoid maintaining two
  different validation idioms in one codebase. Request/service input structs
  are tagged directly (`validate:"required,email"`,
  `validate:"required,min=2,max=100"`, etc.) instead of being paired with a
  separate `zog.Struct(zog.Shape{...})` schema variable — one struct now
  fully describes both shape and constraints, matching the config-validation
  convention already in `internal/utils/config`.
- Keep validation errors as a small package-local type (e.g. `Errors
  map[string][]string`) rather than extending the core `errors` package —
  the validation library's issue shape is its own concern; translate to
  `errs.BadRequest(...)` only at the call site (service today, handlers
  later at step 18). `Validate(dataPtr any) Errors` builds this map from
  `validator.ValidationErrors`, keyed by the **lowercased** Go struct field
  name (e.g. `"email"`, not `"Email"`) so error-map keys read the same as
  they would in a JSON request body.
- `internal/validation/<domain>.go` — one tagged input struct per domain
  (e.g. `UserCreateInput`/`UserUpdateInput`), imported directly by the
  matching service as its method parameter type — no duplicate DTO, no
  separate schema variable.
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
  authz,opentelemetry}` scaffold slots that predated this step —
  `opentelemetry` was superseded by `internal/observability`.
  `internal/services/server` (also a pre-existing stale slot) turned out
  **not** to be stale after all — step 16's Server struct is the thing that
  finally landed there, by deliberate choice, not accident.
- Business logic — call repos, enqueue jobs, send emails
- One subpackage per domain
- Depend on repo **interfaces** (never concrete driver types) and narrow
  interfaces for cross-cutting singletons — e.g. an `Enqueuer` interface
  satisfied by `*asynq.Client`, rather than calling the async job package's
  `Get()` singleton directly from inside a service. Without this, a service
  method that enqueues work is untestable without live Redis. The real
  wiring site (step 16) still passes the singleton's client into the
  constructor; the service itself never imports the singleton accessor.
- **Observability note**: each domain service gets a package-level tracer
  (`otel.Tracer("import/path")`) and wraps every public method in a span.
  Logging level is a deliberate split, not applied uniformly: mutations
  (create/update/delete) get both a success-info and a failure-error log —
  low-frequency, high-value audit trail. Reads (get/list) get
  **error-only** logging — an info line per read is volume with no
  operational value beyond what the span already gives for free
  (duration, pass/fail, correlation). A login failure is logged at `Warn`,
  not `Error` — a bad password is client-caused, not an application fault;
  a successful login isn't logged beyond the span, matching the
  reads-are-quiet default. Restated: logging is for "things a human wants
  to grep," tracing is for "the shape of things over time" — don't reach
  for the same tool for both.
- Depends on: repos, job client, email client, validation, errors,
  observability (for the tracer)

## 16. Server Struct
`internal/services/server/server.go`
- HTTP framework: **gin**, not Echo — the doc originally assumed Echo (see
  step 17's `otelecho` line, now stale); gin was the actual choice made.
- Naming note: the server struct is **the one deliberate exception** to
  "handlers/routes/middleware live in top-level `internal/`, not
  `internal/services/`" — the *Server struct itself* is a service (it
  owns the HTTP server's lifecycle, same shape as `db`/`redis`/
  `asyncprocessor` owning theirs), so it lives at
  `internal/services/server/server.go`. The handlers/router/middleware it
  wires do **not** move under `services/` with it — they stay top-level
  (steps 17–19 below), imported by the server struct, never the reverse.
- Thin fields — holds only what `Shutdown` needs (`*http.Server`, a
  logger). Repos, domain services, handlers, and the router are **local
  to `New`**, not stored on the struct — nothing outside construction
  needs to reach them once the `*gin.Engine` is wired into
  `http.Server.Handler`. This is where `repo.NewRepositories(ctx)` and
  `services.NewServices(repos, asyncprocessor.Get().Client, log)` are
  first actually called — main.go didn't reach them before this step.
- `New(ctx, cfg, log) (*Server, error)` builds everything but does **not**
  start listening. A separate `Start() <-chan error` does — split
  deliberately from step 13's "do everything in `Init`" precedent,
  because `http.Server.ListenAndServe` is itself blocking (unlike
  `asynq.Server.Start`, already non-blocking), so the caller needs the
  constructed `*Server` back before deciding how to run the listen
  goroutine and log its terminal error.
- `Shutdown(ctx) error` calls `httpServer.Shutdown(ctx)` only — does not
  touch db/redis/job, which stay owned by main.go's existing singletons
  and teardown order.
- Depends on: db, redis, job (via `asyncprocessor.Get().Client` for the
  `Enqueuer`), repo, services, router, handler, middleware, config,
  logger, observability

## 17. Middleware
`internal/middleware/cors.go` — hand-rolled (not `gin-contrib/cors`).
Config-driven via `CORSSettings{AllowedOrigins, AllowedMethods,
AllowedHeaders, AllowCredentials, MaxAgeSeconds}`, itself built from
`cfg.ServerConfig` — origins, methods, headers, and max-age are no longer
hardcoded. `AllowCredentials` is deliberately **not** defaulted in
`setDefaults` (a bool's zero value is indistinguishable from "unset", so
defaulting it to `true` would silently override an explicit `false` in a
user's config) — its natural Go zero value (`false`) is the secure default.
`internal/middleware/secure_headers.go`
`internal/middleware/request_id.go` — reuses `google/uuid` (already an
indirect dependency elsewhere); respects an inbound `X-Request-Id` before
generating one.
`internal/middleware/tracing.go` — `otelgin` wiring (not `otelecho` — gin,
not Echo). HTTP-layer metrics are deliberately **not** added on top of this
— otelgin already emits request duration/size automatically; a custom
counter here would double-count. Same reasoning applies to Redis
(`redisotel` already covers it) — custom metrics were added only where no
automatic instrumentation exists: `asyncprocessor`, `internal/lib/email`,
`internal/repo` (step 6/8/11's package doc comments state this explicitly).
`internal/middleware/context.go` — per-request logger injection: request ID
always, `trace_id`/`span_id` chained on when a valid span is active (guarded
so a no-op tracer in dev doesn't emit an all-zero trace ID).
`internal/middleware/request_logger.go` — the actual access-log line
(method/path/status/duration_ms/request_id), separated from `context.go`'s
injector since "inject a logger" and "emit one access-log line" are
different concerns even though BOOT_SEQUENCE.md's step 10 lists them
adjacently.
`internal/middleware/recover.go` — **custom, not `gin.Recovery()`**: gin's
default writes a plaintext panic body, which would break the uniform JSON
`{code,message}` shape every other error path produces via
`errs.ToResponse()`. Builds `errs.Internal(...)` and responds with the same
shape a normal error would.
`internal/middleware/error_handler.go` — gin's idiomatic `c.Errors`
accumulation pattern, converted via `errs.FromError`/`HTTPStatus`. Also
detects `*http.MaxBytesError` via `errors.As` and maps it to
`errs.PayloadTooLarge(...)` → 413, ahead of the generic fallback.
`internal/middleware/max_size.go` — `MaxReqResSize(maxReqBytes,
maxResBytes int64)` is the top-level global request-size guard:
request-side is hard-enforced (413 via `http.MaxBytesReader`); response-side
is observability-only (a warning log if `c.Writer.Size()` exceeds the
threshold after `c.Next()` — never truncated, since corrupting an
already-partially-written JSON response is worse than serving it large).
`MaxReqResSizeOverride(maxReqBytes int64)` lets a route/group raise or lower
the request limit: naively nesting a second `http.MaxBytesReader` inside the
global one does **not** let a larger outer limit win (the inner reader still
errors first), so the global middleware stashes the pristine,
pre-wrap `Body` on the gin context before wrapping, and the override
middleware re-wraps that stashed original fresh with its own limit —
required, not stylistic (see `max_size_test.go`'s
`TestMaxReqResSizeOverride_RaisesLimit`, which would fail under naive
nesting).
`internal/middleware/rate_limit.go` — `RateLimit(perMinute, burst int)`, a
per-IP token bucket via `golang.org/x/time/rate` (already an indirect
dependency via asynq, promoted to direct — no new dependency cost), keyed
by `c.ClientIP()`. A background goroutine evicts entries idle longer than
10 minutes to bound memory under many unique IPs. Exceeding the limit →
429 via `errs.TooManyRequests(...)`.
`internal/middleware/middlewares.go` — a plain `[]gin.HandlerFunc`-returning
function. Mount order: CORS → MaxReqResSize → RateLimit → secure headers →
request ID → tracing → logger injector → access log → recover → error
handler — CORS and the size/rate guards mount first so bad requests are
rejected as cheaply as possible, before any real work happens.
- **Auth is still absent, deliberately** — no auth story exists yet
  (`AuthConfig` in config is unused). Rate limiting, previously deferred, is
  now implemented (above).
- Depends on: config, errors, logger, observability (for the global tracer
  otelgin attaches to)

## 18. Handlers
`internal/handler/health.go` — `HealthHandler{db, redis Pinger, log}`
(constructor-injected, not global `Get()` calls — matches the project's
anti-global-in-testable-code pattern already used for `UserService`).
`Ping` pings both db and redis (2s deadline) and returns
`{"status":"ok"|"degraded","checks":{"db":"ok"|"down","redis":"ok"|"down"}}`,
200 if both healthy, 503 otherwise. No job-queue check — asynq has no
`Ping` equivalent, and the queue rides on the same Redis connection already
being checked, so a separate check would be redundant.
`internal/handler/handlers.go` — aggregate, mirrors `services.Services`'s
shape; has a `Health` field today, ready for a `User` field once User HTTP
endpoints land (deliberately not built this pass — see step 19's routing
note).
- Depends on: db, redis (for the `Pinger` interface satisfied structurally
  by both), errors (no domain handlers yet, so no `services`/`validation`
  dependency in practice today)

## 19. Router
`internal/router/router.go`
- `gin.New()`, not `gin.Default()` — the latter's built-in
  `Logger()`/`Recovery()` would double-log every request and reintroduce
  the plaintext-panic-body problem step 17's custom `Recover` exists to
  avoid.
- `gin.SetMode(...)` is now called before `gin.New()`, mapped from
  `cfg.AppConfig.Env` (`internal/router/mode.go`'s `ginMode`):
  `prod`/`stage` → `gin.ReleaseMode`, anything else → `gin.DebugMode`.
  Previously never called at all (always ran in debug mode, printing
  gin's unstructured startup warnings regardless of environment).
- Mounts `GET /ping` **outside any group** (a bare liveness route, not
  versioned) and an `/api/v1` route group that is **intentionally empty**
  this pass — it exists and inherits the full middleware stack, proving the
  scaffolding works, but carries no real routes yet. User CRUD/signup
  endpoints are future work, gated on deciding what needs auth first.
- Depends on: handler, middleware, config, logger

## 20. main.go
`cmd/<appname>/main.go`
- Wire it all: config → `cfg.PrintConfig()` (secrets already masked by
  `maskSecret` — safe to print early in boot for operational visibility) →
  logger → otel → db → redis → job router (`services.NewJobRouter`) →
  asyncprocessor → `server.New()` (which itself builds repos → services →
  handlers → router) → `server.Start()` → **block on SIGINT/SIGTERM** →
  graceful shutdown.
- The blocking wait on a signal is the one genuinely new piece of behavior
  here: before this step, `main()` never blocked on anything — it logged
  "application started" and returned immediately, tearing everything back
  down via the deferred teardown before any server had ever served a
  request. `signal.Notify` + a blocking channel receive is what makes
  BOOT_SEQUENCE.md step 12's "Graceful Shutdown (on SIGINT)" real for the
  first time.
- Sequence mirrors BOOT_SEQUENCE.md exactly.

## 21. Custom Metrics (asyncprocessor, email, repo)

Each package that has no automatic OTel instrumentation gets a package-level
`meter`, mirroring the existing `tracer` convention (one per package, keyed
by import path). **UserService is excluded, for now** — instruction was to
close the infra-layer gap first, not to instrument the demo service.

- `internal/services/asyncprocessor/router.go` (`instrumentedHandler`,
  wrapping every registered task handler): `task.processed` counter
  (`task_type`, `status=ok|error`), `task.duration_ms` histogram
  (`task_type`).
- `internal/lib/email/tasks.go` (`NewWelcomeEmailHandler`): `email.sent`
  counter, `email.failed` counter (`reason=unmarshal|send`).
- `internal/repo/user.go` (`instrumentOperation`, wrapping every CRUD
  method): `repo.operation` counter (`operation`, `status=ok|error`),
  `repo.operation.duration_ms` histogram (`operation`). The package doc now
  states both halves of the tradeoff explicitly: no manual tracing/logging
  (otelmongo already traces every query) but metrics *are* added (no
  automatic per-operation Mongo metrics exist, unlike tracing).

**Explicitly excluded, with reasoning in-code**: HTTP layer (otelgin
already covers request duration/size) and Redis (`redisotel` already
covers it) — adding custom metrics there would double-count.

**Testing note**: package-level OTel instruments bind to the *first* real
`MeterProvider` ever installed via `otel.SetMeterProvider` — a later,
separate `SetMeterProvider` call in a different test function does not
rebind them. Each package's metrics therefore get **one** test exercising
every code path that records a metric (success and failure) against a
single `sdkmetric.NewManualReader()`-backed provider, not one test per
path.

## 22. DLQ Read (Go API only)

`internal/services/asyncprocessor/inspector.go` —
`(*JobService).ListArchivedTasks(queue string) ([]*asynq.TaskInfo, error)`,
built on `asynq.NewInspectorFromRedisClient(redis.Get().Client())` (reuses
the shared connection, no new pool — same principle as the client/server
construction in step 13). asynq's "archive" is its dead-letter-queue
equivalent.

**Deliberately no HTTP route**: archived-task payloads may contain PII
(e.g. the welcome-email task's recipient address) and this codebase has no
auth yet. Wire an HTTP route once auth lands, gated appropriately.

## 23. Lint + Format Pre-Commit Workflow

`.golangci.yml` (repo root) — `govet`, `staticcheck`, `errcheck`, `unused`.
`.githooks/pre-commit` — gofmt check → `go vet ./...` → `golangci-lint run`;
degrades gracefully with an install link if `golangci-lint` isn't on PATH,
rather than a cryptic "command not found". Activated per-clone via
`make install-hooks` (`git config core.hooksPath .githooks` — native git,
no Python `pre-commit`/`lefthook` framework needed for a pure-Go repo).
`Makefile` also gains `lint` and `fmt-check` targets for running the same
checks outside the hook (CI, manual use).

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
imports the other. `internal/services.NewJobRouter` (not main.go directly) is
where they meet: it builds an `email.Sender`, registers
`email.NewWelcomeEmailHandler(sender, log)` against an
`asyncprocessor.Router` for `email.TaskWelcomeEmail`, then hands that
router to main.go, which passes it into `asyncprocessor.Init`. A future job
kind follows the same shape without touching either existing package.

Note: `handler`/`router`/`middleware` are top-level `internal/` packages,
**not** under `internal/services/` — the one exception living in
`services/` is the `server` struct itself (step 16), which imports all
three (plus `repo`/`services`/`asyncprocessor`) to build the `*gin.Engine`
and `*http.Server`. `main.go` calls `server.New`/`Start`/`Shutdown`; it
never touches `handler`/`router`/`middleware` directly.
