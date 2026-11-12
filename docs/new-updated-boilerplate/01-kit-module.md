# Phase 1 — Split the upgradeable core into a `kit` module

**Goal (D1):** projects started from goboiler can take later fixes with
`go get github.com/hritul2/goboiler/kit@vX.Y.Z` instead of copying code by hand.

## What goes where

| Moves into `kit/` (versioned, imported) | Stays in the template (copied once, owned by the project) |
|---|---|
| `api` (Endpoint, Query/Mutation, Version, Mount, adapter) | `cmd/main.go` and the mode wiring |
| `gen` + the generator entry point | `internal/router/versions.go` |
| `errors`, `validation` | domain packages (models, repo, services, handlers) |
| `middleware` | `configs/*.json`, `.env.example` |
| `config` loader (layers, interpolation) — not the app's `Config` struct | the app's `Config` struct |
| `logger`, `observability` | Dockerfiles, compose files, Makefile, CI |
| `db` (Mongo connection), `redis` | docs specific to the project |
| adapter interfaces + implementations (Phase 4), HTTP client (Phase 5), jobs runtime (Phase 3), migrations runtime (Phase 2) | |
| `archtest` helpers | |

## Design rules
- `kit` never imports the template. The template imports `kit`.
- The config **loader** is generic: it takes a target struct, so each project keeps its own `Config` type.
- Public API is small and documented. Anything not meant for projects stays in `kit/internal/...`.
- Versioning: semantic tags `kit/v0.x.y` while the API settles; a `CHANGELOG.md` in `kit/` lists breaking changes and how to migrate.
- During development the template uses a `replace` directive or `go.work` to point at the local `kit/`. CI checks that the template builds against the **tagged** version before a release.
- `make init` removes the `replace`/workspace so a new project depends on the published tag.

## Steps
1. Create `kit/go.mod` (`github.com/hritul2/goboiler/kit`).
2. Move packages one at a time with their tests, keeping `make check` green after each move.
3. Update imports in the template; add `go.work` for local development.
4. Add an `UPGRADING.md` to the template docs: how a project bumps `kit`, reads the changelog and fixes breaking changes.
5. CI: build and test both modules; a release job tags `kit/vX.Y.Z`.

## Done when
- The template contains no copy of moved code.
- A fresh `make init` project builds against the tagged `kit` without `replace`.
- `UPGRADING.md` documents a real upgrade from one tag to the next.

## Open question
Nested module in this repo (recommended) or a separate `goboiler-kit` repository?
