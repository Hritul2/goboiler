# Phase 0 — Housekeeping, CI pipeline, compiled example

**Goal:** protect what exists before building more. CI runs `make check` on every
push, `make init` is proven on Linux, and the User example can no longer drift
from the code.

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

### 0.2 Compiled example instead of a drifting doc
Today `docs/EXAMPLE.md` (≈3,370 lines) is a snapshot that nothing compiles.
- Restore the User feature as real code under `examples/user/` (model, repo +
  Mongo impl, validation, service, welcome-email job, DTOs/handler/endpoints,
  tests). It is wired into a small example app in `examples/user/cmd/` so it builds
  and its tests run in `make test`, but it is **not** wired into the main app.
- `docs/EXAMPLE.md` becomes **generated** from those files (a small Go program or
  script with `// region:` markers) so every code block is real, compiled code.
  `make example-docs` regenerates it, and `make check` fails if it is stale.
- `make init` deletes `examples/` in a new project (optional flag `KEEP_EXAMPLES=1`).
- The "Last updated / commit" header is written by the generator.

### 0.3 `config.local.json` (pending decision)
If approved: `git rm --cached configs/config.local.json`, add it to `.gitignore`,
commit `configs/config.local.example.json`, and document "copy the example".

## Done when
- CI is green on `main` and on the PR; a broken `gen/` or lint error fails CI.
- `init-smoke` passes on Linux.
- Changing a line in `examples/user` without running `make example-docs` fails `make check`.

## Risks
- Generating the doc from code needs a stable marker convention; keep it simple
  (start/end comments, no AST tricks).
