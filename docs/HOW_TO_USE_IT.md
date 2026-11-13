# How to use goboiler for a new project

For someone who has just copied or cloned goboiler to start a project. It
covers first steps, the decisions this boilerplate makes for you (and what
each one costs), and the one decision it deliberately leaves to you: how the
generated TypeScript client reaches your frontend.

Reading order for the rest of the docs: this file → **`EXAMPLE.md`** (one
complete feature built layer by layer, with code and tests) → `API_HANDLERS.md`
(day to day: adding backend functions, errors, the generated client) →
`BOOT_SEQUENCE.md` (what happens at startup) → `CODING_SEQUENCE.md` (why each
layer exists and in what order to build one).

The repository ships **no domain code**: no users, no endpoints, no jobs. The
pattern is in `EXAMPLE.md`, not in the source tree.

- [1. First steps](#1-first-steps)
- [2. Adding your first feature](#2-adding-your-first-feature)
- [3. Decisions and their implications](#3-decisions-and-their-implications)
- [4. Getting gen/ts into your frontend — your decision](#4-getting-gents-into-your-frontend--your-decision)

---

## 1. First steps

### 1.1 Rename the module and the project

Start from a clean git checkout (copy or clone goboiler, `git init`/commit if
needed), then:

```sh
make init MODULE=github.com/you/fruits                  # project NAME defaults to "fruits"
make init MODULE=github.com/you/fruits NAME=fruit-shop  # or name it explicitly
```

`scripts/init.sh` changes two things, and nothing else:

1. **The Go module path** (`go.mod` and every tracked text file that mentions the
   old one, docs and import snippets included; generated files are regenerated
   instead of edited).
2. **The project name** wherever it is an identity rather than history:

   | Where | What changes |
   |---|---|
   | `Makefile` | `BINARY` (the built binary's name) |
   | `configs/config.json` | `app.name` — the OpenTelemetry service name and tracing middleware name |
   | `docker-compose.local.yml` | the compose project `name:` (prefix of container, network and volume names) and `POSTGRES_DB` |
   | `.env.example` | `DATABASE_NAME` and the sample Postgres URL |
   | `README.md` | the title line |

   Docs that describe the boilerplate itself are left as written. Your own
   untracked `.env` is not touched: set its `DATABASE_NAME` yourself.

Then it runs `go mod tidy` and `make gen`. Review with `git diff`, run
`make check`, commit.

- **NAME** must be lowercase letters, digits and `-`, starting with a letter; it
  becomes the binary name, app name and compose project, and (with `-` → `_`)
  the database name. If you omit it, the last element of `MODULE` is used
  (a trailing `/v2` is skipped, upper case is lowered).
- **MODULE** may contain only letters, digits and `. _ ~ - /`; anything else is
  rejected before a file is touched.
- **Safety:** it refuses to run outside a git checkout, or with uncommitted
  changes (`FORCE=1` overrides the second), so `git diff` shows exactly what it
  changed and `git checkout .` undoes it. It edits only tracked (or untracked
  but not ignored) text files; `.git`, ignored files and binaries are never
  touched.
- **Portable:** works with both macOS (BSD) and Linux (GNU) `sed`/`grep`.
- It refuses to run if `go.mod` no longer declares the original module, so it is
  a once-per-project step. The script itself is not rewritten.

### 1.2 Turn on the git hooks, learn the commands

```sh
make install-hooks   # git config core.hooksPath .githooks (per clone)
make help            # every target, one line each
```

The pre-commit hook runs gofmt → `go vet` → `golangci-lint run`.
**golangci-lint v2 is required** (`.golangci.yml` is in the v2 format);
install it from <https://golangci-lint.run/docs/welcome/install/>.

### 1.3 Configure

```sh
make setup    # creates .env and configs/config.local.json from their examples; never overwrites
```

`.env` and `configs/config.local.json` are gitignored; never commit real
secrets. (`make setup` is the same as `cp .env.example .env` and
`cp configs/config.local.example.json configs/config.local.json`, each only if
the target does not exist yet.)

How config works (`internal/utils/config`); the layering rules are in
[Config files and priority](#config-files-and-priority) below:

- `configs/config.json` is the shape of the config. Values are either literals
  (timeouts, pool sizes, Redis DB index, token expiries) or references into
  the environment: `".env:NAME"` (string), `".env:int:NAME"`,
  `".env:bool:NAME"`, `".env:float:NAME"`, `".env:array:NAME"` (comma
  separated).
- Everything is validated at boot (`go-playground/validator`) **after** the
  files are merged; a missing or invalid value stops startup with a list of the
  failing fields.
- `app.name` is the OpenTelemetry service name and the tracing middleware's
  name. It ships as `golang-boilerplate`; `make init` sets it to your project
  name (change it by hand if you skip `init`).
- Code reads config through named sections: `cfg.App`, `cfg.Server`, `cfg.DB`,
  `cfg.OTel`, `cfg.Redis`, `cfg.Auth`, `cfg.Email`.
- At boot `cfg.PrintConfig()` lists which config and env files were used.

Variables in `.env.example`:

| Variable | Required? | Notes |
|---|---|---|
| `APP_ENV` | yes | `dev` \| `stage` \| `prod`; `prod`/`stage` switch gin to release mode |
| `APP_VERSION` | yes | free-form; reported in telemetry |
| `LOG_LEVEL` | no | `debug` \| `info` \| `warn` \| `error`; default `info` |
| `SERVER_PORT` | yes | `8080` in the example |
| `CORS_ALLOWED_ORIGINS` | yes | comma-separated origins (your frontend's) |
| `DATABASE_URL`, `DATABASE_NAME` | yes | Mongo connection string and database name |
| `OTEL_ENABLED`, `OTEL_ENDPOINT`, `OTEL_SAMPLE_RATE` | enabled flag yes; endpoint only if enabled | no-op providers when disabled |
| `REDIS_ADDR`, `REDIS_PASSWORD` | `REDIS_ADDR` yes | Redis is a hard dependency (job queue); boot fails if unreachable |
| `EMAIL_ENABLED`, `EMAIL_HOST`, `EMAIL_PORT`, `EMAIL_USERNAME`, `EMAIL_PASSWORD`, `EMAIL_FROM`, `EMAIL_FROM_NAME` | host/port/from only if enabled | disabled → a logging no-op sender |
| `ACCESS_TOKEN_SECRET`, `REFRESH_TOKEN_SECRET` | yes, ≥ 32 chars | validated at boot but **no code uses them yet** (no auth is included) |

### Config files and priority

`config.LoadConfig` builds the configuration from these layers, **lowest
priority first**; a higher layer wins:

| # | Source | Notes |
|---|---|---|
| 1 | `configs/config.json` | the base; always loaded |
| 2 | `configs/config.local.json` | **optional** override, deep-merged onto the base |
| 3 | env file named by the base's `env_file_name` (`.env`) | supplies `".env:VAR"` references |
| 4 | env file named by the override's `env_file_name` (`.env.local`), if it sets one | beats the base env file for any variable both define |
| 5 | real OS environment variables | beat **every** file; a file never overrides a variable that is already set |

How the override merges (step 2):

- JSON objects merge key by key, recursively: the override file only needs the
  keys it changes, and their siblings keep the base values
  (`{"config":{"server":{"cors_allow_credentials":true}}}` changes that one flag).
- Scalars and **arrays replace**; arrays are never concatenated
  (`cors_allowed_origins` in the override *is* the list).
- A key set to `null` replaces the base value with `null`.
- The override is found **next to the base file** as `<name>.local.json`:
  `configs/config.json` → `configs/config.local.json`;
  `LoadConfig(&path)` with `/etc/app/prod.json` looks for
  `/etc/app/prod.local.json`. Absent file → just the base. Invalid JSON in it is
  a boot error naming the file.
- `".env:VAR"` / `".env:int:VAR"` / `".env:array:VAR"` references are resolved on
  the **merged** result, from the layered environment, so the override can
  introduce them too.
- Validation runs on the merged config.

The shipped `configs/config.local.example.json` contains only what differs
from the base: `env_file_name` (`.env.local`) and
`server.cors_allow_credentials: true`. `make setup` copies it to
`configs/config.local.json`, which is **gitignored**.

> **The override is per machine, on purpose.** It is picked up automatically
> wherever the file exists, so a committed one would apply in CI, a Docker
> build and production too. goboiler therefore commits only the example; if
> your project needs an override everywhere, commit your own file under another
> name and point `LoadConfig` at it. If you upgrade an existing clone, git
> deletes the previously tracked `configs/config.local.json` on pull: run
> `make setup` again.

Real example of the priority: `SERVER_PORT=9000 make dev` beats `.env` and
`.env.local`; `.env.local` beats `.env`; `config.local.json` beats `config.json`.

### 1.4 Start local services (Docker only)

```sh
docker compose -f docker-compose.local.yml up -d mongo redis mailpit
```

`docker-compose.local.yml` also defines `otel-collector` (OTLP on 4318, set
`OTEL_ENABLED=true`) and a `postgres` service that is there for a future
Postgres repository and is not used by any code today.

Use the compose Mongo rather than a locally installed `mongod`: both bind
27017, and a local server would shadow the container, making a project's data
look empty.

To see outgoing email locally, run Mailpit (included above) and set
`EMAIL_ENABLED=true`, `EMAIL_HOST=localhost`, `EMAIL_PORT=1025` and an
`EMAIL_FROM` address (required when email is enabled); the web UI is
<http://localhost:8025>.

### 1.5 Run it

```sh
make dev                       # go run ./cmd (make build / make run for the binary)
curl localhost:8080/ping       # {"status":"ok","checks":{"db":"ok","redis":"ok"}}
```

Port = your `SERVER_PORT`. `/ping` is the only route in a fresh checkout; RPC
functions appear once you add a feature (section 2). Stop with Ctrl-C; shutdown
is graceful (see `BOOT_SEQUENCE.md` step 12).

### 1.6 Generate the TypeScript client

```sh
make gen                       # writes gen/ts/v1/{schemas,client,hooks,index}.ts (fetch client)
make gen-axios                 # same, but an axios-based client.ts
```

This needs no database or config. Until you add a function the generated client
has zero functions (valid TypeScript, with the shared `ApiError` and schemas).
How the files get to a frontend is
[your decision](#4-getting-gents-into-your-frontend--your-decision).

### 1.7 Before every push

```sh
make check    # fmt-check + lint + test + gen-check — what CI runs
make ts-check # type-check the generated TS clients (needs pnpm); CI runs it too
```

CI (`.github/workflows/ci.yml`) runs both on every push and pull request to
`main`. `.github/workflows/boilerplate.yml` and `scripts/init-smoke.sh` (which
prove `make init` on Linux and macOS) exist only in the boilerplate; `make init`
deletes them.

---

## 2. Adding your first feature

**→ [EXAMPLE.md](EXAMPLE.md).** It builds one complete feature (`User`) from the
model up through the generated frontend client, with the full code and a test at
every step, matching the code as it was verified. The skeleton marks every place
a domain plugs in:

```sh
grep -rn "ADD YOUR DOMAIN" --include='*.go' .
```

| Where | What goes there | EXAMPLE.md |
|---|---|---|
| `internal/models/` | one file per entity | step 1 |
| `internal/repo/` + `internal/repo/mongo/` | repository interface, Mongo implementation, aggregate wiring | step 2 |
| `internal/validation/` | tagged input structs your services validate | step 3 |
| `internal/services/<domain>/` | business logic | step 4 |
| `internal/services/registerasynctasks.go` (`NewJobRouter`) | background-job handlers | step 5 |
| `internal/services/services.go` | the service aggregate | step 6 |
| `internal/handler/<domain>/`, `internal/handler/handlers.go` | DTOs, typed functions, endpoints | step 7 |
| `internal/router/versions.go` | list the endpoints in `v1` | step 8 |
| `make gen` | the TypeScript client | step 9 |
| `internal/router/router.go` (`/webhook` group) | provider webhooks | step 10 |
| `internal/router/versions.go` | a second API version | step 11 |

You do not need all of it: steps 1–4 and 6–8 are a complete feature; the job,
the frontend, the webhook and v2 are independent extras.

---

## 3. Decisions and their implications

Each entry: **chose** → **why** → **costs you** → **to change**.

### POST-only RPC

- **Chose:** every backend function is `POST /api/<ver>/rpc/<name>` with a
  JSON body; no GET/PATCH/DELETE, no path or query parameters.
- **Why:** one calling convention means one adapter, one generated client
  shape, and one place to attach auth, rate limits and validation.
- **Costs you:** no HTTP-level caching of reads (CDN/browser), no pretty
  REST URLs, and anything that is not JSON-in/JSON-out does not fit.
- **To change:** endpoints that must be plain HTTP (file download, SSE,
  health checks) are ordinary gin routes registered in
  `internal/router/router.go`, like `GET /ping` — they simply are not
  generated. Supporting GET inside `internal/api` would mean extending the
  adapter and templates; it is not built.

### Query vs Mutation

- **Chose:** `api.Query` for reads, `api.Mutation` for writes.
- **Why:** it picks the right TanStack hook (`useQuery`, cached and refetched,
  vs `useMutation`, imperative).
- **Costs you:** the distinction is a promise you keep by hand; nothing stops
  a `Query` from writing. It has no effect on HTTP — both are POST.
- **To change:** switch the constructor in `endpoints.go` and run `make gen`.

### Versions as a list

- **Chose:** `router.APIVersions` returns a list of `api.Version`; each is its
  own route group, middleware stack and generated folder
  (`gen/ts/v1`, `gen/ts/v2`).
- **Why:** `/api/v2` can be added without touching v1; the router and the
  generator read the same list, so the served API and the client cannot
  drift.
- **Costs you:** each version is generated separately (schemas are duplicated
  across versions), and you maintain both until v1 is retired.
- **To change:** add or remove entries in `internal/router/versions.go` and
  `make gen`. A single version is perfectly fine; ignore v2 until you need it.

### Webhooks are separate from RPC

- **Chose:** `/webhook/<provider>` routes are plain `gin.HandlerFunc`s in a
  reserved group, not `internal/api` endpoints.
- **Why:** signature verification needs the **raw request body**; the
  provider (Clerk, Dodo Payments, …) calls you, so there is no frontend
  client; and the payload shape is the provider's.
- **Costs you:** no generated types or validation for webhook payloads — you
  verify and decode by hand, and you must make handlers idempotent.
- **To change:** the pattern is in `API_HANDLERS.md` §8, with a full sketch in
  [EXAMPLE.md step 10](EXAMPLE.md#step-10--a-clerk-webhook-that-syncs-users).

### No auth is included

- **Chose:** the boilerplate ships **no authentication or authorisation**.
  `AuthConfig` (token secrets/expiries) is loaded and validated but unused,
  and every function you add is open to anyone who can reach the server.
- **Why:** self-hosted auth and an external provider like Clerk are different
  designs; baking one in would be wrong for the other.
- **Costs you:** until you add it, do not expose the service beyond local
  development.
- **Where to plug it in:**
  - Whole API version: `api.Version.Middleware`. Single function:
    `api.WithMiddleware(...)` on the endpoint. Both take ordinary gin
    middleware; reject with `_ = c.Error(errs.Unauthorized("..."))` then
    `c.Abort()` and the standard `{code,message}` error is rendered.
  - **Self-auth:** a middleware that validates your token (the User service in
    EXAMPLE.md step 4 shows an `Authenticate` that checks email + bcrypt password
    hash; issuing/verifying tokens with the `AuthConfig` secrets is yours to write)
    and stores the identity on the gin context; `login`/`refresh` are normal
    RPC functions, and the frontend sends the token via the client's `headers`
    option (or your axios instance's interceptors).
  - **Clerk:** two pieces. (1) A session middleware that verifies the Clerk
    session token on each request (Clerk's SDK or JWKS verification) and puts
    the Clerk user ID on the context. (2) A `/webhook/clerk` handler
    (Svix-signed, raw body) that syncs user create/update/delete into your own
    tables — [EXAMPLE.md step 10](EXAMPLE.md#step-10--a-clerk-webhook-that-syncs-users)
    is a tested sketch of exactly that webhook. Because the user ID is then Clerk's string, see "`models.ID` is a
    string" below.

### The repo layer is the only database-specific code

- **Chose:** `internal/repo` holds driver-free interfaces; each database is a
  package under it (`internal/repo/mongo`); connection and error mapping live
  in `internal/db`. Models, services and handlers never import a driver, and
  `internal/archtest` fails the tests if anything but `internal/db` and
  `internal/repo/mongo` imports the Mongo driver.
- **Why:** MongoDB, Postgres or SQLite can back a project without touching
  business logic.
- **Costs you:** a private row/document struct plus conversion code per
  entity, and repository interfaces to keep in step with every
  implementation.
- **To add Postgres or SQLite:** create `internal/repo/<driver>` implementing
  the same interfaces, add the connection in `internal/db` and map its errors
  to the `errs` sentinels next to `db.MapDBError`, switch the constructor in
  `repo.NewRepositories`, and add the driver to the rules in
  `internal/archtest`. The numbered steps are in the `internal/repo` package
  comment. Nothing above the repo layer changes.

### `models.ID` is a string

- **Chose:** `type ID string`, opaque outside the repo; the store assigns it on
  `Create`; DTO IDs are `validate:"required,max=64"`.
- **Why:** an ObjectID hex, a UUID, a serial number or a Clerk `user_…` string
  all fit, and DTOs/the generated client stay database-neutral.
- **Costs you:** no compile-time difference between a user ID and an order ID
  (both `models.ID`), and ID *format* is no longer a validation error: an ID
  the store cannot parse is simply **404 NOT_FOUND** (with Mongo, `"abc"`).
- **To change:** define distinct types (`type UserID string`) per entity and
  thread them through repo/service/handler signatures — mechanical but wide.

### Named config fields

- **Chose:** `cfg.App.Env`, `cfg.Server.Port`, `cfg.DB.Uri`, … rather than
  embedding the section structs.
- **Why:** one explicit way to reach a value; no mix of promoted
  (`cfg.Env`) and qualified forms.
- **Costs you:** slightly longer access paths. The config-file and env-var
  keys are not affected.
- **To change:** edit `internal/utils/config/config.go` and the call sites;
  the compiler lists them all.

### Config: a base file, an optional local override, then the environment

- **Chose:** `configs/config.json` is the base; `configs/config.local.json`
  (optional) is deep-merged on top; env files and real environment variables
  fill in `".env:VAR"` references, with OS variables winning over everything
  (full rules: [Config files and priority](#config-files-and-priority)).
- **Why:** the base documents the shape and safe defaults; a developer (or an
  environment) changes only the keys that differ, without copying the whole file
  and drifting from it.
- **Costs you:** an override file is loaded automatically wherever it exists, so
  a *committed* one applies in CI and production too; and there are now five
  places a value can come from (`PrintConfig` at boot says which files were
  used). Arrays replace rather than extend, which surprises people who expect
  a union.
- **Done for you:** `configs/config.local.json` is gitignored and
  `configs/config.local.example.json` is committed (`make setup` copies it).
- **To change:** point `LoadConfig` at another base path
  (`LoadConfig(&path)`; its override is `<name>.local.json` beside it); or drop
  the override layer by deleting the file.

### CI: GitHub Actions running `make check`

- **Chose:** `.github/workflows/ci.yml` runs `make check` (fmt-check, lint,
  tests, gen-check) and `scripts/ts-check.sh` (strict `tsc` over the generated
  fetch and axios clients, for the skeleton and the generator's golden
  fixtures). Actions are pinned by commit SHA; golangci-lint is pinned to the
  version in the workflow.
- **Why:** a red check on the pull request is cheaper than a broken `main`.
- **Costs you:** the TypeScript check installs a few npm packages
  (`.github/ts-check/`, pnpm, lockfile committed) in CI only; they are not
  dependencies of your application. Bump them deliberately.
- **To change:** edit `.github/workflows/ci.yml`; delete the `ts` job if you
  have no TypeScript consumers.

### Error responses carry `fields`

- **Chose:** every error is `{code,message}`; validation failures (400
  `BAD_REQUEST`) add `fields`: JSON field path → messages
  (`"address.city"`, `"lines[1].sku"`). The generated `ApiError` exposes
  `status`, `code`, `message` and `fields`.
- **Why:** a form can attach each message to its input without parsing text.
- **Costs you:** keys are JSON field names, so DTOs must keep explicit `json`
  tags (the handler test in EXAMPLE.md reflects over every DTO to check); `message` repeats the details in one
  line, which is longer than a bare "validation failed".
- **To change:** the wording/kinds live in `internal/validation`
  (`tagMessage`), the shape in `internal/errors` (`errs.Validation`).

### Lenient JSON decoding

- **Chose:** unknown fields in a request body are ignored, and an empty body
  means `{}`.
- **Why:** newer and older clients keep working across deploys, and
  `api.Empty` functions need no body.
- **Costs you:** a misspelled optional field is silently ignored (a misspelled
  *required* one fails validation as "is required").
- **To change:** `decodeBody` in `internal/api/handle.go`; calling
  `DisallowUnknownFields()` on the decoder makes unknown fields a 400.

### Invalid API declarations panic at boot

- **Chose:** `api.Mount` panics if a version is malformed (bad function name,
  duplicate name, non-struct request/response, colliding type names).
- **Why:** these are programming errors in a static list; failing at startup
  beats a half-registered API or a broken client. The generator reports the
  same problems as an error instead.
- **Costs you:** a mistake takes the process down at boot rather than at one
  request. `go test ./internal/router` mounts the real versions, so CI catches
  it first.
- **To change:** unlikely to be worth it; the rules are in `Version.Validate`
  (`internal/api/version.go`).

### Generated client is committed and checked in CI

- **Chose:** `gen/ts` is committed, and `make gen-check` (part of
  `make check`) fails if it is stale or has untracked files.
- **Why:** turns "changed the Go API, forgot to regenerate" into a failing
  build, and gives the frontend a reviewable diff of every API change.
- **Costs you:** generated churn in pull requests, and CI must run the same
  `OUT`/`GEN_FLAGS` you generate with. `gen-check` refuses to run when `OUT`
  is outside this repository.
- **To change:** if you would rather generate during the frontend's build,
  gitignore `gen/` and drop `gen-check` from `check`. Where the files go is
  covered in [section 4](#4-getting-gents-into-your-frontend--your-decision).

### Generated client: fetch (default) or axios

- **Chose:** `make gen` produces a `fetch`-based `client.ts` (no extra
  dependency); `make gen-axios` (= `make gen GEN_FLAGS=--axios`) produces an axios-based one.
  Only `client.ts` differs — `schemas.ts`, `hooks.ts`, `index.ts` and the
  public surface (`createClient`, function names and signatures, `ApiError`,
  `Client`) are identical.
- **Why:** many frontends already standardise on axios (interceptors for auth
  tokens, retries, a shared instance); others want zero dependencies.
- **Costs you:** the option names differ (`baseUrl`/`fetch` vs
  `baseURL`/`axios`). Error handling is the same in both: an HTTP error becomes
  an `ApiError` from the response body, and "no response" (offline, DNS, CORS,
  timeout) becomes an `ApiError` with status `0` and code `NETWORK_ERROR`;
  cancellations are rethrown unchanged. axios is a peer dependency
  (`axios@^1`).
- **To change:** run `make gen-axios` (or pass `GEN_FLAGS=--axios`) per call, or
  pin it for the project by setting `GEN_FLAGS := --axios` at the top of the
  Makefile (then `make gen` and `make gen-check` both use it). If you commit an
  axios client, pin it: `make gen-axios` is one-off, and an unpinned
  `make gen-check` would regenerate fetch and fail. Usage: `API_HANDLERS.md` §10.

### golangci-lint v2

- **Chose:** `.golangci.yml` is in the v2 format (`version: "2"`).
- **Costs you:** `make lint` / `make check` and the pre-commit hook need a v2
  binary; v1 rejects the file.
- **To change:** install v2 (CI included), or rewrite the config for v1.

### Native git hooks, not a hook framework

- **Chose:** `.githooks/pre-commit`, activated per clone by
  `make install-hooks`.
- **Why:** no extra tooling for a pure-Go repo.
- **Costs you:** it is opt-in per clone and bypassable (`--no-verify`), so
  CI must run `make check` as the real gate.
- **To change:** swap in lefthook/pre-commit if you prefer; the hook only runs
  gofmt, `go vet` and golangci-lint.

### The Makefile is the command index

- **Chose:** every routine task is a `make` target; `make help` lists them from
  the `## description` comments next to each target.
- **Costs you:** you need `make`.
- **To change:** add a target and put `## what it does` after it; it appears
  in `make help` automatically.

---

## 4. Getting gen/ts into your frontend — your decision

**This is the project owner's choice. goboiler does not pick one**: it
generates `gen/ts/<version>/*.ts` (and can write it elsewhere with `OUT=`)
and stops there. Pick per project; you can change later because the generated
files are the only contract.

Whatever you choose, the generated code needs `zod@^4` in the consuming app,
`@tanstack/react-query@^5` if it imports `hooks.ts`, and `axios@^1` if you
generated with `--axios`.

### Option A — write straight into the frontend checkout

```sh
make gen OUT=../web/src/api            # or: make gen-axios OUT=../web/src/api
```

- **Good:** nothing to copy; the frontend sees the change instantly during
  local development.
- **Bad:** both repositories must sit side by side on every developer's
  machine. The output is committed in the *frontend* repo, so
  `make gen-check` (which only checks this repo, and refuses an `OUT` outside
  it) cannot guard it, and the frontend's CI cannot re-run the generator
  without access to the backend code. Nothing in the backend repo records
  which API the frontend currently has.
- **Fits:** one person or one small team owning both, no CI guard needed.

### Option B — commit `gen/ts` here, copy it to the frontend

The backend commits `gen/ts` (the default); `make gen-check` in backend CI
guarantees it matches the Go API. The frontend gets a copy by a script you
write (`cp`/`rsync`), a git submodule or subtree pointing at this repo's
`gen/ts`, or a manual copy in the release checklist.

- **Good:** the backend is the source of truth and is verified in CI; the
  frontend stays independent and only needs the files.
- **Bad:** a copy step that people forget; the frontend can lag the backend
  until someone runs it.
- **Fits:** separate repos and teams with a modest release cadence.

### Option C — monorepo: import it in place

Backend and frontend live in one repository and the frontend imports
`gen/ts/v1` through a path alias or workspace package.

- **Good:** no copy step and no version skew; one PR changes both sides, and
  `make gen-check` guards the same tree the frontend uses.
- **Bad:** one repo/CI to share; the generated files import `zod` (and react-query /
  axios), which must resolve from the frontend's `node_modules` (hoisted
  workspace deps, or a bundler alias to the app's copy), not from a
  `node_modules` next to the Go code.
- **Fits:** one team shipping both together.

### Option D — publish a private npm package from CI

CI runs `make gen` (on tag or main) and publishes `gen/ts` as a package; the
frontend installs it and bumps the version to adopt API changes.

- **Good:** explicit versions and changelog; many frontends can consume it;
  the frontend never needs backend code. Add a hand-written `package.json`
  inside `gen/ts` (with `zod`, `@tanstack/react-query`, `axios` as
  `peerDependencies`) — the generator only rewrites `.ts` files in the version
  folders and leaves other files alone.
- **Bad:** the most machinery: a registry, a publish pipeline, versioning
  discipline (a breaking Go change must be a new major / new `v2`), and a
  release delay between a backend change and the frontend being able to use
  it. The `package.json` and publish workflow are yours to write; none is
  provided.
- **Fits:** several consumers, or teams that want explicit API versions.
