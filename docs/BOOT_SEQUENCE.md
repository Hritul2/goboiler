# Go Backend Boot Sequence

> The repository ships no domain code (no users, endpoints or jobs); the one-feature worked example is [EXAMPLE.md](EXAMPLE.md).
>
> New project? Start with [HOW_TO_USE_IT.md](HOW_TO_USE_IT.md) (first steps, decisions and their costs, getting the generated client into your frontend).

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
- `config.LoadConfig` layers the configuration, **lowest priority first**:
  `configs/config.json` (base) → `configs/config.local.json` (optional, gitignored, created from `configs/config.local.example.json` by `make setup`; found
  next to the base as `<name>.local.json`, deep-merged: objects merge, scalars
  and arrays replace) → env files (the override's `env_file_name` is loaded
  before the base's, and `godotenv` never overwrites a variable that is already
  set) → real OS environment variables, which win over everything. `".env:VAR"`
  references are interpolated on the **merged** result; validation runs on it
  too. See `HOW_TO_USE_IT.md` ("Config files and priority").
- Parse the merged map with `koanf` → unmarshal into typed `Config` struct. Every
  section is a **named field**, never an embedded struct: `cfg.App`,
  `cfg.Server`, `cfg.DB`, `cfg.OTel`, `cfg.Redis`, `cfg.Auth`, `cfg.Email`
  (so call sites read `cfg.App.Env`, `cfg.Server.Port`, `cfg.DB.Uri`). The
  `koanf` tags — the config-file section names — and the `.env:VAR`
  references in `configs/*.json` are unchanged by the Go field names.
- `setDefaults` fills in optional operational tunables not present in
  `configs/*.json` — CORS methods/headers/max-age, request/response body
  size limits, rate-limit per-minute/burst — before validation runs, so
  existing config files stay valid without needing every new field spelled
  out. `CORSAllowCredentials` is the one exception: a `bool`'s zero value
  can't be distinguished from "unset", so it is deliberately left
  undefaulted (`false` = secure-by-default) rather than risk silently
  overriding an explicit `false`.
- Validate all fields with `go-playground/validator` — fail fast if missing
- `cfg.PrintConfig()` — call once, early in boot, for operational
  visibility. It starts with the config and env files that were actually used
  (and which env files were missing), lowest priority first. Every secret (Mongo URI, Redis/email passwords, auth token
  secrets) is masked via `maskSecret` before printing — safe to call
  unconditionally, no environment gating needed.

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
  against it (`services.NewJobRouter`; the skeleton registers none — a
  worker with no handlers starts fine, and EXAMPLE.md step 5 shows
  `router.Handle(email.TaskWelcomeEmail, email.NewWelcomeEmailHandler(sender,
  log))`). asyncprocessor itself has no idea what any task does — this
  registration step, at the composition root, is what wires a domain's
  handler to its task type.
- Call the async job package's singleton `Init(ctx, cfg, log, router)` (see
  CODING_SEQUENCE.md step 13, `internal/services/asyncprocessor`) — mirrors
  the db/redis singleton pattern. `Init` does the full boot in one call:
  `asynq.NewClientFromRedisClient`/`NewServerFromRedisClient` (both reusing
  step 5's already-connected, already-instrumented redis client — never a
  second connection pool) with queue priorities (`critical`, `default`,
  `low`), then starts the server against the caller-supplied router. An
  `asynq.Inspector` is constructed the same way
  (`NewInspectorFromRedisClient`, same shared connection, no eager dial) and
  stored alongside `Client`/`server` — it backs `ListArchivedTasks`, the
  Go-API-only read path for asynq's "archive" (its dead-letter-queue
  equivalent). Deliberately no HTTP route for it: archived-task payloads may
  contain PII (e.g. an email task's recipient) and no auth exists
  yet.
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
- Pure data-access layer, and the **only** database-specific code besides
  `internal/db`. `internal/repo` holds driver-agnostic interfaces
  (one per entity, IDs are opaque `models.ID` strings) and the
  `Repositories` aggregate; each database is an implementation package under
  it — `internal/repo/mongo` (a Postgres/SQLite one would be
  `internal/repo/<driver>` implementing the same interfaces). The skeleton
  declares no repository yet, so `NewRepositories` needs no database and the
  server boots (and `/ping` works) before any domain exists; EXAMPLE.md step 2
  builds the interface and the Mongo implementation.
- The Mongo implementation receives the db singleton (`db.GetMongo()`),
  keeps a private bson document struct plus to/from-model conversion, parses
  the hex ID (a malformed ID is `errs.NotFound` — it cannot exist), and
  normalises driver errors through `db.MapDBError`.
- No business logic — only queries and cache reads/writes
- First actually constructed inside step 16's server struct
  (`repo.NewRepositories(ctx)` — the single place that picks the Mongo
  implementation and runs `EnsureIndexes`) — nothing before that step called
  it; the singleton existed but was unused until the HTTP layer needed it.
- `internal/archtest` fails `go test` if any package other than
  `internal/db` and `internal/repo/mongo` imports the Mongo driver.

### 8. Services
- Business logic layer
- Receives: repo **interfaces** (never a driver type) + an `Enqueuer`
  interface (satisfied by `asyncprocessor.Get().Client`) + config
- Owns all domain rules
- A service that only *enqueues* work (e.g. the User service enqueuing a
  welcome email, EXAMPLE.md step 4) needs just the `Enqueuer` — it never needs an `email.Sender`
  directly. The sender is consumer-side: it's built once at step 6 (inside
  `internal/services.NewJobRouter`), to register the actual handler against
  the router; services only ever touch the producer side (building/
  enqueuing a task).
- Services depend on the narrow `Enqueuer` interface, not the async job
  package's singleton accessor directly (CODING_SEQUENCE.md step 15) — this
  is what keeps a service's business rules unit-testable without a live
  Redis.
- Each service method is wrapped in an OTel span (package-level tracer) and
  logs at a level matched to its cost/value (mutations: info+error; reads:
  error-only; failed auth: warn, not error) — see CODING_SEQUENCE.md step
  15's observability note for the full reasoning.
- First actually constructed inside step 16's server struct
  (`services.NewServices(repos, asyncprocessor.Get().Client, log)`) —
  same story as repos above.

### 9. Handlers
- Domain handlers (`internal/handler/<domain>`) are **typed functions**
  `func(ctx, Req) (Res, error)` over a narrow service interface — they know
  nothing about gin or JSON. The `internal/api` adapter, not the handler,
  does the HTTP work: decode the JSON body → `validation.Validate` (400 with
  a per-field `fields` map on failure) → call the function → render `200` +
  JSON, or hand the error to `middleware.ErrorHandler`. The same endpoint
  declarations feed the router (step 10) and the TypeScript generator — see
  `docs/API_HANDLERS.md`.
- Receives: services (no direct DB/Redis access), except `HealthHandler`,
  which is constructor-injected with `db.Get()`/`redis.Get()` directly
  (both satisfy a small `Pinger` interface structurally, no adapter code)
  — the only handler that legitimately needs dependency access rather than
  going through a service.
- `HealthHandler.Ping` performs a real check: pings db and redis with a 2s
  deadline, returns `{"status":"ok"|"degraded","checks":{"db":...,
  "redis":...}}`, 200 if both healthy, 503 otherwise. No job-queue check —
  asynq has no `Ping`, and the queue rides on the same Redis connection
  already being checked.
- Handlers pass DTO strings straight through as `models.ID`; the repository
  decides whether an ID exists.

### 10. Router + Middlewares
`gin.SetMode(...)` is called before `gin.New()`, mapped from
`cfg.App.Env` (`prod`/`stage` → release mode, else debug) —
previously never called, so gin always ran in debug mode regardless of
environment.

Stack order (outer → inner), as actually built:
1. CORS — config-driven (`CORSSettings` built from `cfg.Server`:
   origins, methods, headers, credentials, max-age)
2. `MaxReqResSize` — global request-size guard (hard 413 via
   `http.MaxBytesReader`) + response-size observability warning (never
   truncated). A route/group can override the request limit via
   `MaxReqResSizeOverride`, which re-wraps the pristine body stashed by the
   global middleware (naive nested `MaxBytesReader`s can't raise a limit —
   the inner reader still wins).
3. `RateLimit` — per-IP token bucket (`golang.org/x/time/rate`), config-driven
   per-minute/burst, 429 on exceeding
4. Secure headers
5. Request ID
6. `otelgin.Middleware(serviceName)` — tracing (gin, not Echo — the doc's
   original `otelecho` assumption was never implemented)
7. Logger injector (request ID + trace/span ID onto a per-request logger)
8. Access-log line (method/path/status/duration/request_id)
9. Recover — **custom**, not `gin.Recovery()`, so a panic produces the same
   JSON `{code,message}` shape as every other error path
10. Error handler (`c.Errors` → JSON via `errs.FromError`/`HTTPStatus`,
    including `*http.MaxBytesError` → `errs.PayloadTooLarge` → 413)

CORS and the size/rate guards mount first so bad requests are rejected as
cheaply as possible. **Auth is still absent, deliberately** — remains
future work; rate limiting, previously deferred, is now implemented above.

Register three independent route families (`internal/router/router.go`;
full guide in `docs/API_HANDLERS.md`):
- `GET /ping` **outside any group** (bare liveness check, not versioned). It
  performs the real db/redis dependency checks described in step 9 — a real
  `/health` under a different name was considered and rejected; `/ping`
  keeps its name and just gets real checks.
- One gin group per RPC API version from `router.APIVersions`
  (`POST /api/v1/rpc/<function>`, mounted with `api.Mount`, which panics at
  boot on a malformed endpoint declaration). Each group inherits the
  middleware stack; adding `/api/v2` is a new entry in `versions.go`. The
  skeleton's `v1` has no functions — a valid, empty version that mounts no
  routes.
- A reserved, still empty `/webhook` group for provider callbacks
  (Clerk, Dodo, …), which need the raw request body and are not part of the
  generated client.

### 11. HTTP Server
- Configure `http.Server` with read / write / idle timeouts from
  `cfg.Server`, `Handler` set to the `*gin.Engine` from step 10
- Built by `internal/services/server.New`, which does **not** start
  listening — a separate `Start() <-chan error` does, spawning
  `ListenAndServe` in a goroutine and returning a channel that receives
  exactly one value: a genuine listen failure (the caller filters out
  `http.ErrServerClosed`, the expected result of a graceful shutdown)

### 12. Graceful Shutdown (on SIGINT)
Reverse of the boot order above. **This is the first point in the codebase
where `main()` actually blocks on anything** — before this step, `main()`
logged "application started" and returned immediately, tearing everything
back down before any server had served a single request. Now:
`signal.Notify` + a blocking channel receive on SIGINT/SIGTERM triggers the
sequence below:
1. `httpServer.Shutdown(ctx)` — drain in-flight requests (`cfg.Server.ShutdownTimeout`)
2. Close the async job singleton (`asyncprocessor.Get().Close(ctx)`) — closes
   before redis since the queue needs Redis alive to drain in-flight tasks,
   and it was opened after redis at boot (step 6 follows step 5)
3. Close the redis singleton (`redis.Get().Close(ctx)`) — closes before db
   since it was opened after db at boot (step 5 follows step 4)
4. Close the db singleton (`db.Get().Close(ctx)`)
5. Flush otel tracer provider (`tp.Shutdown`)
6. Flush logger

Live-verified: the full sequence above produces exactly this log order on a
clean shutdown (`"shutdown signal received"` → job shutting-down/shut-down
→ `"shutdown complete"`), with the HTTP/redis/db/otel steps each logging
only on error.

---

## Build-time: TypeScript generator (cmd/gen)

Not part of server startup — `cmd/gen` is a separate binary (`make gen`) with
its own, much shorter sequence. It needs **no config, database, Redis,
network or running server**.

1. **Flags** — `-out` (default `gen/ts`; `make gen OUT=...`): the directory
   each version is written under; `-axios`/`--axios` (`make gen-axios`, or
   `make gen GEN_FLAGS=--axios`):
   generate an axios-based `client.ts` instead of the default fetch one (the
   other three files are identical in both modes).
2. **Handler tree with nil deps** — `handler.New(nil, nil, &services.Services{}, zerolog.Nop())`.
   `New` calls nothing on its arguments, and the generator only reads
   endpoint *metadata*; the typed functions are bound to nil services and
   never invoked.
3. **Versions** — `router.APIVersions(h)`: the exact list `router.New`
   mounts, so the generated client cannot drift from the served API.
4. **Validate** — `Version.Validate()` per version (name/prefix shape,
   lowerCamel unique function names, named non-generic struct Req/Res, no
   schema-name collisions) plus a duplicate-version-name check. Returns an
   error here instead of panicking like `api.Mount`.
5. **Types → zod** — for each endpoint in registration order, its `Req` and
   `Res` `reflect.Type`s go through `zen` (the only importer, along with
   `internal/gen`) to produce zod schemas from the `json`/`validate` tags.
   zen panics on a tag it cannot express; `Generate` recovers that into an
   error naming the version.
6. **Templates** — the embedded `internal/gen/templates/*.tmpl` render
   `schemas.ts`, `client.ts`, `hooks.ts` and `index.ts` (`client.ts` = the
   shared `client.ts.tmpl` plus the transport blocks of `client_fetch.tmpl` or
   `client_axios.tmpl`; including the
   hand-written `ApiErrorSchema`/`ApiError`, guarded by a test against
   `errs.ErrorResponse`). Output is pure and deterministic.
7. **Write** — `gen.Write` writes `gen/ts/<version>/*.ts` and removes stale
   `.ts` files inside the version folders it owns (nothing else in `-out`).
8. **CI guard** — `make gen-check` re-runs the generator (same `OUT` and
   `GEN_FLAGS`) and fails if `OUT` (default `gen/ts`) differs from the committed
   tree or contains untracked files; it refuses an `OUT` outside this repo.
   `make check` includes it. Generated output is committed. `make gen-axios`
   does not set `GEN_FLAGS`, so a committed axios client needs
   `GEN_FLAGS := --axios` pinned in the Makefile for `gen-check` to agree.

Details, file map and the golden-file tests: CODING_SEQUENCE.md step 24 and
`docs/API_HANDLERS.md` §9.

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
| HTTP framework middleware | `go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin` (gin was the actual choice — the doc originally assumed Echo/`otelecho`, never implemented) |
| Async job task dispatch | No maintained contrib package for asynq — hand-rolled: `internal/services/asyncprocessor`'s `Router.Handle` wraps every registered handler in a span (`otel.Tracer(pkgpath)`), same "hand-roll when nothing maintained exists" principle as step 4's Mongo tracing |
| Custom metrics (no contrib coverage) | `otel.Meter(pkgpath)` per package, hand-rolled where no automatic instrumentation exists: `asyncprocessor` (`task.processed`/`task.duration_ms`), `internal/repo/mongo` (`repo.operation`/`repo.operation.duration_ms`); a domain job adds its own (the welcome-email job in EXAMPLE.md step 5 records `email.sent`/`email.failed`). **Not** added for HTTP (otelgin already covers it) or Redis (`redisotel` already covers it) — would double-count. **Not** added for domain services — scoped out. |

---

## Pre-Commit Workflow

Native git, no external framework: `.githooks/pre-commit` (gofmt check →
`go vet` → `golangci-lint run`, degrading gracefully with an install link if
`golangci-lint` isn't on PATH) is activated per-clone via `make
install-hooks` (`git config core.hooksPath .githooks`). `.golangci.yml` at
the repo root configures the enabled linters in **golangci-lint v2** format
(`version: "2"`), so `make lint` and `make check` require a v2 binary.
`make lint`/`make fmt-check` run the same checks standalone (CI, manual use)
outside of a commit; `make check` adds tests and the generated-client drift
check.
