# New / updated boilerplate — roadmap

> Status: **plan only, nothing in this folder is built yet.**
> Written: 2026-11-12. Each phase starts only after it is approved.

goboiler is becoming a **heavy-backend starter** that aims for Rails/Next.js-level
ergonomics (generators, one way to do common things, swappable adapters,
upgradeable core) without Rails-style magic: explicit wiring in Go, small
interfaces, no reflection-based DI. It is **not** a SaaS kit (no billing, orgs or
auth UI).

## Decisions (made 2026-11-12)

| # | Question | Decision | Consequence |
|---|---|---|---|
| D1 | Will projects receive boilerplate updates later? | **Yes** | The stable core must move into an importable, versioned `kit` module ([Phase 1](01-kit-module.md)). The template keeps only app wiring. |
| D2 | Databases out of the box | **MongoDB only** | Repo interfaces stay driver-agnostic (already enforced by `internal/archtest`), but only a Mongo implementation ships. |
| D3 | Process shape | **One binary** with modes: `serve`, `worker`, `scheduler`, `migrate`, `seed`, `all` | [Phase 3](03-process-modes-and-jobs.md). |
| D4 | Developer interface | **Makefile** (no custom CLI for now) | Every new command gets a `make` target with a `##` help line. |
| D5 | Queue backend | **Redis (asynq) by default**, no in-memory queue | Local dev needs Redis running (docker compose). |
| D6 | Adapters | cache → **Redis**, storage → **S3**, mail → **SMTP** | Local equivalents run in docker: Redis, MinIO (S3 API), Mailpit (SMTP). |
| D7 | Observability | Tracing, metrics and logs must be in place for **every** layer and adapter | [Phase 7](07-observability-and-health.md) adds an audit + tests that enforce it. |
| D8 | Integration tests | **Yes**, with testcontainers | [Phase 8](08-integration-tests.md). |
| D9 | Containers | `Dockerfile`, `Dockerfile.local`, `docker-compose.yaml`, `docker-compose.local.yaml` | [Phase 9](09-docker-and-deploy.md); local volumes under `output/docker-volume/` (gitignored). |

## Phases

| Phase | Title | Depends on |
|---|---|---|
| [0](00-housekeeping-and-ci.md) | Housekeeping, CI pipeline, compiled example | — |
| [1](01-kit-module.md) | Split the upgradeable core into a `kit` module | 0 |
| [2](02-data-migrations-seeds-tx.md) | Migrations, seeds, transactions, cursor pagination | 1 |
| [3](03-process-modes-and-jobs.md) | One binary with modes; worker + scheduler on Redis | 1 |
| [4](04-adapters.md) | Adapters: cache (Redis), storage (S3), mail (SMTP) | 1 |
| [5](05-http-client.md) | Outbound HTTP client | 1 |
| [6](06-domain-generator.md) | `make domain NAME=…` generator | 2, 3, 4 |
| [7](07-observability-and-health.md) | Observability audit + liveness/readiness | 3, 4, 5 |
| [8](08-integration-tests.md) | Integration tests with testcontainers | 2, 3, 4, 9 (compose services) |
| [9](09-docker-and-deploy.md) | Dockerfiles + compose files | 3 |

Phases 2–5 can run in parallel once Phase 1 lands. Suggested order if done one
at a time: **0 → 1 → 9 → 3 → 2 → 4 → 5 → 7 → 8 → 6**. Docker comes early because
every later phase needs the local stack. The generator comes last because it
should generate code that uses every other layer.

## How each phase is delivered

1. Opus writes or refines the phase plan here; you approve it.
2. A Sonnet agent implements it on its own branch.
3. Opus reviews: tests, lint, `make check`, and a read of the diff.
4. A PR into `main`. The phase doc is updated with what actually shipped.

**Every phase is done when** `make check` is green in CI, docs are updated
(`HOW_TO_USE_IT.md`, `BOOT_SEQUENCE.md`, `CODING_SEQUENCE.md`, plus the phase
doc), new commands appear in `make help`, and every new layer has tracing,
metrics and logs (D7).

## Open questions (answer before the phase that needs them)

- **Phase 0 (resolved: yes, shipped in Phase 0a):** should `configs/config.local.json` be untracked, gitignored and replaced by a committed `config.local.example.json`? (Recommended: yes. Today the tracked override also applies in CI and production.)
- **Phase 1:** one repository with a nested `kit/` module (tags like `kit/v0.3.0`), or a separate repository? (Recommended: nested module, same repo.)
- **Phase 2:** MongoDB transactions need a replica set, even with one node. Is it OK for local and test Mongo to run as a single-node replica set? (Recommended: yes.)
- **Phase 7:** local observability UI: the all-in-one `grafana/otel-lgtm` image, or separate Jaeger + Prometheus + Grafana containers? (Recommended: `otel-lgtm` locally.)
- **Phase 9:** hot reload in `Dockerfile.local` uses `air`, installed inside the image, never on your machine. OK?
