# Phase 6 — `make domain NAME=…` generator

**Goal:** `make domain NAME=order` writes a complete, wired, tested feature in
seconds, the way `rails g scaffold` does. It replaces copying from `EXAMPLE.md`.

## Usage (D4: Makefile)
```
make domain NAME=order                 # full CRUD feature
make domain NAME=order FIELDS="title:string amount:int paidAt:time?"
make domain NAME=order DRY_RUN=1       # print the files, write nothing
```
- `NAME` is singular lowerCamel; plural and Pascal forms are derived (`order`, `orders`, `Order`).
- `FIELDS` is optional; `?` marks an optional field. Supported types: string, int, int64, float, bool, time, id, plus `[]` of those. Without `FIELDS` a single `name` field is generated.

## What it writes
| Layer | Files |
|---|---|
| Model | `internal/models/order.go` (+ test) |
| Repo | interface in `internal/repo/order.go`, Mongo impl in `internal/repo/mongo/order.go` (+ tests) |
| Migration | `internal/migrations/<timestamp>_create_orders_indexes.go` (Phase 2) |
| Validation | `internal/validation/order.go` |
| Service | `internal/services/order/` (+ tests with a fake repo) |
| API | `internal/handler/order/{dto,handler,endpoints}.go` (+ tests mounting real endpoints) |
| Generated client | runs `make gen` at the end |

## Wiring
The generator inserts one line at each existing `// ADD YOUR DOMAIN:` marker
(repo aggregate, services, handlers, `versions.go`). Insertions are idempotent:
running it twice for the same name refuses with a clear message. Every change is
shown as a summary at the end; the project then reviews `git diff`.

## Implementation
- A Go program in `kit` (`kit/cmd/domaingen`) using `text/template` and `go/format`; templates are the code in `examples/user` (Phase 0), with `user` replaced by a placeholder. This keeps the example, the generator and the docs in one source.
- After writing, it runs `gofmt`, `go build ./...` and the new package's tests, and prints the result.

## Done when
- CI job: generate two domains in a scratch copy, then `make check` passes.
- Generated code has the same structure and quality as `examples/user`, including tracing and metrics in the repo and service.
- `EXAMPLE.md` and `API_HANDLERS.md` start with "run `make domain`", with the walkthrough kept as explanation.
