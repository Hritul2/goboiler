# API Handlers, RPC Functions & the Generated TypeScript Client

> New project? Start with [HOW_TO_USE_IT.md](HOW_TO_USE_IT.md) (first steps, decisions and their costs, getting the generated client into your frontend).

How HTTP gets into goboiler, how to add a backend function, and how the
frontend consumes it with full type safety.

The Go code is the single source of truth. You write a function with a
request struct and a response struct; goboiler serves it over HTTP **and**
generates a typed TypeScript client (zod schemas, fetch functions, TanStack
Query hooks) from the same declaration. Frontend and backend stay separate
repos/deployables — the only thing that crosses the boundary is the generated
`gen/ts/` folder.

---

## There is no example domain in the code

The repository ships the framework and **no domain code**: `v1` has no
functions, the job queue has no jobs, there are no users. (A compiled User
example does exist, but outside the application: `examples/user`, laid over the
skeleton only by `make example-check` and `make example-try`, and deleted by
`make init` unless `KEEP_EXAMPLES=1`.) The pattern is
documented end to end — model, repository, service, welcome-email job, DTOs,
handler, endpoints, registration, generated client, a Clerk webhook, `/api/v2` —
with full, tested code at every step in **[EXAMPLE.md](EXAMPLE.md)**. The
framework itself is domain-independent: `internal/api`, `internal/gen`,
`cmd/gen`, `internal/errors`, `internal/validation/validation.go`.

- **Services depend only on the repo interfaces** (`internal/repo`) and
  `models.ID` strings; handlers depend only on a narrow service interface.
  The database (Mongo provided; Postgres/SQLite possible) is visible **only** in
  `internal/db` and `internal/repo/<driver>` — `internal/archtest` fails the
  test run if anything else imports the Mongo driver. Swapping databases or
  adding self-auth/Clerk never touches handlers or services.
- Each place a domain plugs in is marked `// ADD YOUR DOMAIN: ... — see
  docs/EXAMPLE.md step N`.
- Nothing is authenticated until you add auth (see [§6](#6-per-endpoint-and-per-version-middleware)).

---

## 1. The three route families

Everything is mounted in `internal/router/router.go`, and the three families
are deliberately **independent** — adding one never touches another.

| Family | Path | Built with | Generated client? |
|---|---|---|---|
| Health | `GET /ping` | plain gin handler (`internal/handler/health.go`) | no |
| RPC API, one per version | `POST /api/<ver>/rpc/<function>` | `internal/api` endpoints | **yes**, one folder per version |
| Webhooks, one per provider | `POST /webhook/<provider>` | plain `gin.HandlerFunc` (`internal/webhook/<provider>/`) | no |

The RPC family is **POST-only**: no GET/PATCH/DELETE, no path or query
parameters. One calling convention means one adapter, one generated client
shape and one place to attach auth. The function name is in the URL, the
input object is the JSON body:

```
POST /api/v1/rpc/deleteUser
Content-Type: application/json

{ "userId": "665f1c2e8b1e4a0012a3b4c5" }
```

(`deleteUser` is the function built in [EXAMPLE.md](EXAMPLE.md); a fresh
checkout has no RPC functions yet.) Once you have one, try it against a running
server (`make dev`). The port comes from `SERVER_PORT` in your env file
(`.env.example` ships `8080`):

```sh
curl -X POST localhost:8080/api/v1/rpc/listUsers \
  -H 'Content-Type: application/json' -d '{}'
```

The only route in a fresh checkout is `GET /ping`.

Success is always `200` with a JSON body. Errors use the repo-wide
`{ "code": "NOT_FOUND", "message": "..." }` shape (plus `fields` for
validation failures — see [§5](#5-returning-errors)) and the status from
`internal/errors`.

---

## 2. Adding a function (step by step)

The full worked feature, with code and tests at every layer, is
**[EXAMPLE.md](EXAMPLE.md)**. This is the short version for adding one
function to a feature that exists. As a running example, take
`getUserByEmail` on the User feature from EXAMPLE.md; it touches every layer,
which is the point of the walkthrough.

1. **DTOs** (`internal/handler/<domain>/dto.go`). A request struct (here
   `GetUserByEmailReq{Email string \`json:"email" validate:"required,email"\`}`)
   and a response struct (reuse `UserRes`). See [DTO tags](#dto-tags) before
   writing tags. → [EXAMPLE.md step 7](EXAMPLE.md#step-7--dtos-handler-endpoints)
2. **Service, and repo if needed** (`internal/services/<domain>/`). Add the
   business method (`UserService.GetByEmail`, which calls the repository's
   existing `GetByEmail`). If the **repository** lacks the method, add it to the
   repository interface in `internal/repo` **and** to every implementation
   (`internal/repo/mongo` today). Services never reach for a driver; they only
   call the interface. → steps [2](EXAMPLE.md#step-2--repository) and
   [4](EXAMPLE.md#step-4--service)
3. **Typed function** (`handler.go`). A plain method with the exact shape
   `func(context.Context, Req) (Res, error)`; it knows nothing about gin, HTTP or
   JSON. Extend the handler's narrow `userService` interface with the new
   method first, then write it. Depend on that interface, never the concrete
   service: it is what makes the handler unit-testable with a fake.
4. **List it in `Endpoints`** (`endpoints.go`):

   ```go
   api.Query("getUserByEmail", h.GetUserByEmail,
       api.WithDescription("Fetches one user by email.")),
   ```

5. **Register** in `internal/router/versions.go`. If the domain's `Endpoints(h)`
   is already spliced into the version, you are done with this step. For a
   **new domain**, add its `Endpoints` to the version
   (`slices.Concat(user.Endpoints(h.User), billing.Endpoints(h.Billing))`) and
   wire it through the two aggregates: a field on `services.Services` built in
   `services.NewServices` (`internal/services/services.go`, fed by
   `repo.Repositories`), and a field on `handler.Handlers` built in `handler.New`
   (`internal/handler/handlers.go`) from `svcs.<Domain>`. →
   [EXAMPLE.md steps 6 and 8](EXAMPLE.md#step-6--service-wiring)
6. **Regenerate and commit:** `make gen`, then `git add gen/ts`.
7. **Test.** Write a handler test with a fake service: happy path, bad input →
   400, service error → mapped status. Mount the real `Endpoints(h)` in a gin
   engine so the test covers decoding and validation too (the style is shown in
   EXAMPLE.md's [Testing section](EXAMPLE.md#testing-the-style-per-layer)).

---

## 3. Query vs Mutation

| | `api.Query` | `api.Mutation` |
|---|---|---|
| Use for | reads that are safe to repeat | anything that changes state |
| HTTP | `POST` (same as all RPC) | `POST` |
| Generated hook | `useQuery`, key `[version, name, input]` — cached, deduped, refetched | `useMutation` — imperative, never cached |

The kind affects **only** the generated React hooks. Pick `Query` only if
calling the function twice is harmless; otherwise use `Mutation`.

Functions with no input or output use `api.Empty` as the type:
`func(ctx, api.Empty) (api.Empty, error)`. For an `api.Empty` **request** the
input is optional everywhere on the client:

- client function: `input = {}` — call `client.listAll()` (or `client.listAll({})`);
- query hook: no `input` parameter — `useListAll(options?)`, query key
  `[version, "listAll", {}]`;
- mutation hook: no variables — call `mutate()`.

---

## 4. Rules

### Naming (checked at boot — a violation **panics** in `api.Mount`)

- Function names are **lowerCamel** identifiers: `^[a-z][a-zA-Z0-9]*$`
  (`createUser`, not `CreateUser`, `create-user` or `create_user`). The name is the URL
  suffix *and* the TypeScript function name; `useCreateUser` is derived from it.
- Names are unique within a version.
- Request and response types must be **named, non-generic structs**. The
  generator derives schema names from the Go type name, so anonymous structs,
  `Foo[int]`, pointers, maps and slices are rejected. Wrap them in a struct.
- Two different Go types with the same short name in one version (e.g.
  `user.Req` and `billing.Req`) are rejected: they would both become `ReqSchema`.
  Use descriptive names (`CreateUserReq`).
- Version names are lowercase (`v1`); prefixes start with `/` and have no
  trailing `/`.

### DTO tags

- **Every field has an explicit camelCase `json` tag.** The tag is the wire
  contract *and* the zod/TypeScript property name. A missing tag leaks Go's
  `PascalCase` field name to the frontend (EXAMPLE.md's handler test reflects over
  every DTO to assert this — copy that test for each domain). It is also the key
  a validation failure is reported under (see [§5](#5-returning-errors)).
- Inputs carry `validate` tags (go-playground/validator). They are enforced on
  the server by the adapter **and** translated to zod checks by the generator,
  so both sides validate identically.
- **IDs are opaque strings**, tagged `validate:"required,max=64"`. The DTO
  bounds the size and nothing more: it must not assume the database's ID
  format (no `mongodb` tag, no `bson.ObjectID`, no UUID tag). The handler
  passes the string through as a `models.ID`; the repository decides whether
  it exists. An ID the store cannot parse (e.g. `"abc"` against Mongo) is
  therefore **404 NOT_FOUND**, not 400.
- Response DTOs are separate from `models.*` so a new model field (or a
  password hash) can never reach a client by accident. Map in one place
  (`toUserRes`), `ID: string(u.ID)` / `u.ID.String()`.
- Use `omitempty` for optional input fields (`json:"page,omitempty"`).

### Tag → zod table and caveats

| Go | Generated zod |
|---|---|
| `string` | `z.string()` |
| `validate:"required"` (string) | `.min(1)` |
| `validate:"email"` | `.check(z.email())` |
| `validate:"min=2,max=100"` (string) | length refinements (counted in characters) |
| ID: `string` + `validate:"required,max=64"` | `.min(1)` + a ≤ 64-character refinement |
| `int`, `float64` | `z.number()` |
| `validate:"min=1,max=100"` (number) | `.gte(1).lte(100)` |
| `bool` | `z.boolean()` |
| `time.Time` | `z.coerce.date()` — JSON string in, JS `Date` out |
| `*T` | `T.nullable()` |
| `*T` with `json:",omitempty"` | `T.optional()` |
| field with `json:",omitempty"` | `.optional()` |
| `[]T` | `T.array().nullable()` — a nil Go slice encodes as `null` |
| `[]T` + `validate:"required"` | `T.array()` (never null; make sure you return a non-nil slice) |
| `map[string]T` | `z.record(z.string(), T).nullable()` |
| nested struct | referenced schema (`AddressSchema`) |
| `api.Empty` | `z.object({})` |

There is no mapping for driver types (`bson.ObjectID`, `pgtype.UUID`, ...):
DTOs only use plain Go types, which is what keeps them database-agnostic.

Caveats:

- **Unsupported `validate` tags make the generator fail** with a clear error
  (zen panics on unknown tags; `Generate` converts that to an error). Use the
  tags above and others listed in zen's README, or avoid the tag on the DTO.
- `validate` tags that exist only on the server (e.g. cross-field rules) can't
  be expressed in zod; the server still enforces them.
- `json:"-"` fields are skipped.
- Self-referential types are supported; other cycles are not.
- Unknown JSON fields in a request are ignored (lenient decoding); an empty
  body is treated as `{}`.

---

## 5. Returning errors

Return `*errs.AppError` values (alias `errs "github.com/hritul2/goboiler/internal/errors"`).
Never write a response yourself — the adapter does `c.Error(err)` and
`middleware.ErrorHandler` renders `{code,message}` with the right status.

| Return | Status | Code |
|---|---|---|
| `errs.BadRequest("...")` | 400 | `BAD_REQUEST` |
| `errs.Validation(fields)` | 400 | `BAD_REQUEST` + `fields` map |
| `errs.Unauthorized("...")` | 401 | `UNAUTHORIZED` |
| `errs.Forbidden("...")` | 403 | `FORBIDDEN` |
| `errs.NotFound("...")` | 404 | `NOT_FOUND` |
| `errs.Conflict("...")` | 409 | `CONFLICT` |
| `errs.Unprocessable("...")` | 422 | `UNPROCESSABLE` |
| `errs.TooManyRequests("...")` | 429 | `TOO_MANY_REQUESTS` |
| anything else (or `errs.Internal(err)`) | 500 | `INTERNAL` (cause is logged, never sent) |

Adapter-generated errors: malformed JSON or wrong field types → 400;
`validate` failures → 400 with a `fields` map; body over the configured
limit → 413 `PAYLOAD_TOO_LARGE`.

### Validation errors and the `fields` map

A failed `validate` rule (in the adapter, or in a service that validates its
own input) produces:

```http
HTTP/1.1 400 Bad Request

{
  "code": "BAD_REQUEST",
  "message": "validation failed: email: must be a valid email address; password: must be at least 8 characters",
  "fields": {
    "email":    ["must be a valid email address"],
    "password": ["must be at least 8 characters"]
  }
}
```

`message` is a deterministic one-line summary (usable as a toast or in
logs); `fields` is the structured form for mapping onto form inputs. Other
errors have no `fields` key.

**Keys are JSON field paths** — the names the client sent, not Go names:

- the field's `json` tag name (`userId`);
- no `json` tag, or `json:"-"` → lowerCamel of the Go field name
  (`Email` → `email`, `UserID` → `userID`, `ID` → `id`);
- nested structs use dots: `address.city`;
- slice elements keep their index: `lines[1].sku`.

Messages depend on the field's kind: `min`/`max`/`gte`/`lte` say
`must be at least 8 characters` for strings, `must be at least 18` for
numbers and `must be at least 2 items` for slices/maps. Also covered:
`required`, `email`, `url`, `uuid`, `oneof` (`must be one of: a, b`), `gt`,
`lt`, `len`; any other tag falls back to `failed '<tag>' validation`.
Implemented in `internal/validation/validation.go`; add a `case` there when
you start using a new tag.

In Go, build one with `errs.Validation(map[string][]string{...})`.
`validation.Errors` is assignable to that map type, so a service validating
its own input does:

```go
if verrs := validation.Validate(&in); verrs.HasErrors() {
    return nil, errs.Validation(verrs)
}
```

(`internal/errors` takes a plain map so it never imports `internal/validation`.)

### On the frontend

Every non-2xx becomes an `ApiError` with `status`, `code`, `message` and, for
validation failures, `fields?: Record<string, string[]>`:

```ts
try {
  await client.createUser(values)
} catch (e) {
  if (e instanceof ApiError && e.fields) {
    // react-hook-form: attach each message to its input
    for (const [path, messages] of Object.entries(e.fields)) {
      form.setError(path as FieldPath<typeof values>, { message: messages[0] })
    }
  } else if (e instanceof ApiError && e.code === "CONFLICT") {
    // e.status === 409
  }
}
```

The same keys are the property names of the generated request types, so
`e.fields.email` lines up with `CreateUserReq["email"]`.

---

## 6. Per-endpoint and per-version middleware

```go
api.Mutation("deleteUser", h.DeleteUser, api.WithMiddleware(requireAdmin))
```

`api.WithMiddleware` takes ordinary `gin.HandlerFunc`s that run before the
handler, in order. To protect a whole version, set `api.Version.Middleware`.
Middleware reports failures the same way handlers do: `c.Error(errs.Unauthorized(...))`
then `c.Abort()`.

> ⚠️ **Nothing is authenticated.** Any function you add (including the User
> endpoints in EXAMPLE.md) is open to anyone who can reach the server. Add auth
> middleware (via the options above) before exposing the service beyond local
> development.

---

## 7. Adding `/api/v2`

Versions are independent: each is its own gin group, with its own middleware,
and generates to its own folder. In `internal/router/versions.go`:

```go
func APIVersions(h *handler.Handlers) []api.Version {
    return []api.Version{
        {Name: "v1", Prefix: "/api/v1/rpc", Endpoints: user.Endpoints(h.User)},
        {Name: "v2", Prefix: "/api/v2/rpc", Endpoints: userv2.Endpoints(h.User)},
    }
}
```

([EXAMPLE.md step 11](EXAMPLE.md#step-11--add-apiv2-for-this-feature) builds that `userv2`
package — v2 keeps every v1 function and replaces one — and tests it.)

Then `make gen` writes `gen/ts/v2/` next to `gen/ts/v1/`. v2 can reuse v1's
handlers, wrap them, or diverge entirely; nothing in `router.go` changes
because it loops over `APIVersions`. Frontends migrate by changing an import
path (`gen/ts/v1` → `gen/ts/v2`), and both can coexist in one app.

---

## 8. Webhooks (Clerk, Dodo Payments, …)

Webhooks are **not** RPC functions and are not generated. They differ in three
ways: the provider calls *us* (no frontend client), we need the **raw request
body** to verify the provider's signature (a JSON-decode-first adapter would
break that), and the payload shape is the provider's, not ours.

`router.go` reserves a `/webhook` group. To add a provider:

1. Create `internal/webhook/<provider>/` (e.g. `internal/webhook/clerk/`) with
   a constructor-injected handler holding the provider's signing secret from
   config:

   ```go
   func (h *Handler) Handle(c *gin.Context) {
       raw, err := c.GetRawData() // body exactly as sent
       if err != nil { _ = c.Error(err); return } // oversized => 413
       if err := h.verify(raw, c.Request.Header); err != nil {
           _ = c.Error(errs.Unauthorized("invalid signature"))
           return
       }
       var evt event
       if err := json.Unmarshal(raw, &evt); err != nil {
           _ = c.Error(errs.BadRequest("invalid event"))
           return
       }
       // dispatch on evt.Type …
       c.Status(http.StatusOK) // providers retry on non-2xx
   }
   ```

   Verify the signature **before** parsing, over the raw bytes, with a
   constant-time comparison (or the provider's SDK verifier — e.g. Svix for
   Clerk), and check the timestamp header to reject replays.

2. Register it in `router.New` on the webhook group:

   ```go
   webhooks := engine.Group("/webhook")
   webhooks.POST("/clerk", h.Clerk.Handle)
   webhooks.POST("/dodopayment", h.Dodo.Handle)
   ```

3. If the provider sends large payloads, override the global body limit for that
   route only: `webhooks.POST("/dodopayment", middleware.MaxReqResSizeOverride(5<<20), h.Dodo.Handle)`.

4. Make handlers **idempotent** — providers deliver at-least-once. Do slow work
   by enqueuing an asynq job (see `docs/ASYNC_JOBS.md`) and returning 200 fast.

Adding or removing a webhook never affects `/api/<ver>` and vice versa.

A complete, tested **sketch** of a Clerk `user.created` webhook (Svix signature
over the raw body, replay check, idempotent sync, registration on this group)
is in [EXAMPLE.md step 10](EXAMPLE.md#step-10--a-clerk-webhook-that-syncs-users).

---

## 9. Running the generator

```
make help                       # list every make target with a one-line description
make gen                        # writes gen/ts/<version>/{schemas,client,hooks,index}.ts (fetch client)
make gen-axios                  # same, but client.ts is axios-based (= make gen GEN_FLAGS=--axios)
make gen OUT=../web/src/api     # write somewhere else (e.g. straight into a frontend repo)
make gen-check                  # regenerates, fails if OUT (default gen/ts) differs from what's committed
make lint                       # golangci-lint run
make check                      # fmt-check + lint + test + gen-check (+ example-check, example-docs-check while examples/ exists) (CI / pre-push)
```

**Client kinds.** The default `client.ts` uses the global `fetch` (no extra
dependency). `make gen-axios` (shorthand for `make gen GEN_FLAGS=--axios`; the
`--axios` flag of `cmd/gen`) generates an axios-based `client.ts` instead.
Only `client.ts` differs: `schemas.ts`, `hooks.ts`, `index.ts` and the public
surface (`createClient`, one function per endpoint with identical signatures,
`ApiError`, `Client`) are byte-identical, so
switching is a regenerate plus changing the `createClient` options
([§10](#10-using-the-client-in-a-frontend)). The file header names the kind and
the command that regenerates it.

**Pinning flags for a project.** `GEN_FLAGS` and `OUT` are `?=` variables at the
top of the `Makefile`; to make axios (or another `OUT`) the project default, set
`GEN_FLAGS := --axios` there. `make gen-check` uses the same variables, so CI and
developers stay in step. `make gen-axios` does *not* change those variables: it
is a one-off, so if you commit an axios client without pinning `GEN_FLAGS`,
`make gen-check` regenerates a fetch client and reports drift. Pin it (or pass
`GEN_FLAGS=--axios` to `gen-check`) when axios is the project choice.
`go run ./cmd/gen -out gen/ts -axios` is the underlying command.

A fresh checkout has no functions, so `gen/ts/v1` is a valid client with zero
functions (it compiles even under `noUnusedLocals`); it fills in as you add
endpoints.

`gen-check` compares against *this* repository's git tree, so it only works when
`OUT` is inside the repo; for an `OUT` elsewhere it stops with an explanatory
message before generating anything. How the files reach a frontend is a project
decision: see `HOW_TO_USE_IT.md` §4.

**`make lint` and `make check` need golangci-lint v2** (`.golangci.yml` is in
the v2 format, `version: "2"`; a v1 binary rejects it). Install it from
<https://golangci-lint.run/docs/welcome/install/> and confirm with
`golangci-lint --version`. `make gen` / `make test` do not need it.

`make gen` runs `go run ./cmd/gen -out gen/ts`. It needs no database, Redis or
config: it builds the handler tree with nil dependencies because it only reads
endpoint metadata (names, kinds, request/response types) and never calls a
handler. Output is deterministic (registration order), so diffs are minimal.
It only deletes stale `.ts` files inside the version folders it owns. The full
pipeline is described in `docs/BOOT_SEQUENCE.md` ("Build-time: TypeScript
generator") and `docs/CODING_SEQUENCE.md` (step 24).

**Commit `gen/ts/`.** The frontend consumes it (copy, git submodule, or a
published package — your call), and `make gen-check` in CI turns "I changed
the Go API but forgot to regenerate" into a failing build. `gen-check` also
fails on untracked files under `gen/`, so a newly generated version must be
committed too.

Only `cmd/gen` and `internal/gen` import `github.com/hypersequent/zen`; the
server binary never links it. The generator's own tests are golden-file tests
(`internal/gen/testdata/*.golden`); after an intentional template change run
`go test ./internal/gen -update` and review the diff.

---

## 10. Using the client in a frontend

Peer dependencies of the generated code (not installed by this repo):
**`zod@^4`**, for `hooks.ts` **`@tanstack/react-query@^5`**, and for the axios
client (`--axios`) **`axios@^1`**.

The snippets below use the functions of the User feature from
[EXAMPLE.md](EXAMPLE.md#step-9--generate-the-client-and-use-it-from-a-frontend)
(`getUser`, `listUsers`, `createUser`, `deleteUser`); a fresh checkout generates
a client with no functions, so yours will have your own names.

Plain client (no React), default **fetch** variant:

```ts
import { createClient, ApiError } from "./gen/ts/v1"

const api = createClient({
  baseUrl: "https://api.example.com",
  headers: () => ({ Authorization: `Bearer ${getToken()}` }), // optional; object or function
  // fetch: customFetch,                                      // optional
})

try {
  const user = await api.getUser({ userId })      // typed, response parsed by zod
  user.createdAt                                  // Date
} catch (e) {
  if (e instanceof ApiError && e.code === "NOT_FOUND") { /* e.status === 404 */ }
}
```

### axios variant (`make gen-axios`)

Same functions, `ApiError` and hooks; only the `createClient` options differ.
Pass either `baseURL` or your own axios instance (or both; an explicit `baseURL`
overrides the instance's):

```ts
import axios from "axios"
import { createClient, ApiError } from "./gen/ts/v1"

// 1. Simplest: a fresh axios instance
const api = createClient({ baseURL: "https://api.example.com" })

// 2. Your own instance — its interceptors run, so auth lives in one place
const http = axios.create({ baseURL: "https://api.example.com" })
http.interceptors.request.use((cfg) => {
  cfg.headers.set("Authorization", `Bearer ${getToken()}`)
  return cfg
})
const api2 = createClient({ axios: http })      // `headers` (object or function) is also accepted

try {
  await api2.getUser({ userId })
} catch (e) {
  if (e instanceof ApiError) {
    e.status   // HTTP status, or 0 when there was no response
    e.code     // backend code, "UNKNOWN" for a non-JSON body, "NETWORK_ERROR" for no response
    e.fields   // validation messages, as in §5
  }
}
```

Behaviour worth knowing: every call is `POST` with a JSON body; any non-2xx
response is thrown as an `ApiError` built from the body (your instance's
response interceptors still see the rejection first, e.g. to refresh a token on
401); network failures and timeouts (no response) become `ApiError` with
`status` 0 and `code` `"NETWORK_ERROR"`; cancellations and errors thrown by your
own interceptors are rethrown unchanged. The default fetch client behaves the
same way: a rejected `fetch` (no response) becomes `ApiError` with status 0 and
code `"NETWORK_ERROR"`, and an `AbortError` is rethrown untouched.

### React + TanStack Query (either variant)

```tsx
import { QueryClient, QueryClientProvider, useQueryClient } from "@tanstack/react-query"
import { createClient, createHooks } from "./gen/ts/v1"

const client = createClient({ baseUrl: import.meta.env.VITE_API_URL })
export const { useGetUser, useListUsers, useCreateUser, useDeleteUser } = createHooks(client)

function Users() {
  const qc = useQueryClient()
  const { data, isPending } = useListUsers({ page: 1 })      // useQuery, key ["v1","listUsers",{page:1}]
  const del = useDeleteUser({
    onSuccess: () => qc.invalidateQueries({ queryKey: ["v1", "listUsers"] }),
  })
  // del.mutate({ userId })
}
```

Call `createHooks` once at module level (not inside a component). The
`<Name>Schema` exports (e.g. `CreateUserReqSchema`) are the same rules the
server enforces — use them for form validation (react-hook-form's zod
resolver, etc.), and map server-side failures back onto the form with
`ApiError.fields` ([§5](#on-the-frontend)). The client does **not** validate
input before sending; the server is authoritative.

If you don't use React, import from `./gen/ts/v1/client` and
`./gen/ts/v1/schemas` directly (`index.ts` also re-exports `hooks.ts`, which
needs react-query).

---

## 11. Where things live

```
internal/api/            RPC framework: Endpoint, Query/Mutation, Version, Mount, gin adapter
internal/handler/<dom>/  dto.go, handler.go, endpoints.go per domain (none yet; see EXAMPLE.md)
internal/services/<dom>/ business logic; depends on repo interfaces only
internal/repo/           repository INTERFACES + Repositories wiring (no DB driver)
internal/repo/<driver>/  one implementation per database (mongo today) — private doc/row structs
internal/models/         pure domain structs; models.ID is an opaque string
internal/validation/     validator wrapper: JSON-keyed errors for the `fields` map
internal/archtest/       import-rule tests (the Mongo driver stays in internal/db + internal/repo/mongo)
internal/router/         router.go (mounts all families), versions.go (API version registry)
internal/webhook/<prov>/ webhook handlers (when added) — not generated
internal/gen/            Generate/Write + templates/*.tmpl + golden tests
cmd/gen/                 the `make gen` CLI
gen/ts/<version>/        generated TypeScript (committed)
```
