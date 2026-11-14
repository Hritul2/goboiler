# Phase 0 — Housekeeping, CI pipeline, compiled example

**Goal:** protect what exists before building more. CI runs `make check` on every
push, `make init` is proven on Linux, and the User example can no longer drift
from the code.

## Shipped in Phase 0a

- `.github/workflows/ci.yml`: `check` (golangci-lint v2.13.0 + `make check`) and
  `ts` (`scripts/ts-check.sh`: strict `tsc` over the skeleton and golden-fixture
  clients, fetch and axios, packages pinned in `.github/ts-check/`).
- `.github/workflows/boilerplate.yml` + `scripts/init-smoke.sh`
  (`make init-smoke`): `make init` on a scratch copy, on Linux (GNU sed) and
  macOS (BSD sed). Both are deleted by `make init`.
- `configs/config.local.json` untracked; `config.local.example.json` and
  `make setup` added; the shipped-config tests no longer depend on a local file.
- `log/.gitkeep` is now tracked (`.gitignore` fixed); `/output/` ignored.
- `TestNew_timeFieldFormat` no longer writes into the source tree.

## Shipped in Phase 0b

- **The compiled example**: `examples/user/_overlay/` holds the User feature as
  files at their real destination paths (Go tools skip `_` directories, so the
  root build never sees them). `make example-check` copies the working tree
  (`git ls-files -co --exclude-standard`) into a scratch directory, lays the
  overlay over it and runs gofmt, build, vet, golangci-lint, tests and `cmd/gen`
  there. The overlay is not an `examples/user/cmd` app and `make test` does not run
  it: a separate package cannot compile code written as files inside skeleton
  packages.
- **`examples/exampletool`** (standard library only, with tests): `materialize`
  and `docs`.
- **`docs/EXAMPLE.md` is generated** from `examples/user/EXAMPLE.md.tmpl`
  (directives `file`, `region`, `excerpt`, `sketch`; `make example-docs`).
  `make example-docs-check` (part of `make check`) fails if it is stale. The
  header carries a deterministic source hash (sha256 over `examples/user/**`
  with the module path normalised), not a date or commit, which would change on
  every run. The Clerk webhook, a sketch before, is compiled now; the Postgres
  block stays a sketch (`examples/user/sketches/`).
- **`make init`** removes `examples/` unless `KEEP_EXAMPLES=1`; `docs/EXAMPLE.md`
  stays as a marked snapshot. `init-smoke` asserts both variants, and CI runs
  the `KEEP_EXAMPLES=1` one on Linux.
- **CI `ts` job**: `scripts/ts-check.sh --tree` type-checks the example's
  generated v1 and v2 clients (fetch and axios) and the frontend snippets.
- **Make targets**: `example-try DIR=...`, `example-check`, `example-docs`,
  `example-docs-check`, which exist only while `examples/user` does.

## Scope

### 0.1 CI pipeline (GitHub Actions)
- `.github/workflows/ci.yml` on push and pull request to `main`, on `ubuntu-latest`:
  - Go version read from `go.mod`; module and build cache.
  - golangci-lint **v2** (pinned version), then `make check`
    (fmt-check, lint, test, gen-check).
  - TypeScript check of `gen/ts/*` for the fetch **and** `--axios` output, using
    a throwaway `package.json` inside the CI job (typescript, zod, @tanstack/react-query, react, axios). Nothing is added to the repo's own dependencies.
- Separate job **init-smoke**: copy the repo, `git init` + commit, run
  `make init MODULE=github.com/example/fruits NAME=fruits`, then `go build ./...`,
  `make test`, `make gen`. This proves `scripts/init.sh` on Linux (GNU sed/grep).
- Later phases add an `integration` job (Phase 8) and a `docker build` job (Phase 9).

### 0.2 Compiled example instead of a drifting doc (done in 0b)
The plan below is the original; "Shipped in Phase 0b" above says what was built
(an overlay rather than an example app, a source hash rather than a date).

Before 0b, `docs/EXAMPLE.md` (≈3,370 lines) was a snapshot that nothing compiles.
- Restore the User feature as real code under `examples/user/` (model, repo +
  Mongo impl, validation, service, welcome-email job, DTOs/handler/endpoints,
  tests). It is wired into a small example app in `examples/user/cmd/` so it builds
  and its tests run in `make test`, but it is **not** wired into the main app.
- `docs/EXAMPLE.md` becomes **generated** from those files (a small Go program or
  script with `// region:` markers) so every code block is real, compiled code.
  `make example-docs` regenerates it, and `make check` fails if it is stale.
- `make init` deletes `examples/` in a new project (optional flag `KEEP_EXAMPLES=1`).
- ~~The "Last updated / commit" header is written by the generator.~~ Replaced by
  the source hash: a date or commit changes on every run, which defeats "stale
  fails `make check`", and a commit cannot contain its own hash.

### 0.3 `config.local.json` (done in 0a)
Approved and shipped: `git rm --cached configs/config.local.json`, add it to `.gitignore`,
commit `configs/config.local.example.json`, and document "copy the example".

## Done when
- CI is green on `main` and on the PR; a broken `gen/` or lint error fails CI.
- `init-smoke` passes on Linux.
- Changing a line in `examples/user` without running `make example-docs` fails `make check`.

## Risks
- Generating the doc from code needs a stable marker convention; keep it simple
  (start/end comments, no AST tricks).
