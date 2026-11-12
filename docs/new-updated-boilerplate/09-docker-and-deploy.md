# Phase 9 — Dockerfiles and compose files

**Goal (D9):** one command starts everything locally; one image runs in
production in any mode.

## Files

| File | Purpose |
|---|---|
| `Dockerfile` | Production image. Multi-stage: build with the Go version from `go.mod`, `CGO_ENABLED=0`, version/commit via `-ldflags`; run on a distroless (or `scratch` + CA certs) base as a **non-root** user. Entrypoint is the binary; the mode is the command (`serve`, `worker`, `scheduler`, `migrate up`). Includes `HEALTHCHECK` hints in docs (orchestrators probe `/live` and `/ready`). |
| `Dockerfile.local` | Development image with hot reload (`air`, installed **inside** the image), source mounted from the repo, Go module and build caches as volumes. |
| `docker-compose.local.yaml` | Local **infrastructure only**: MongoDB (single-node replica set), Redis, MinIO (+ bucket init), Mailpit, OpenTelemetry collector, and the observability UI (`grafana/otel-lgtm`, Phase 7). Replaces today's `docker-compose.local.yml`. You run the app on your machine (`make dev`) or add the profile below. |
| `docker-compose.yaml` | **Full stack**, production-like: the app built from `Dockerfile` as three services (`api` → `serve`, `worker`, `scheduler`), a one-shot `migrate` service that runs before them, plus the same infrastructure. Used for "does it all work together" checks and as a deploy reference. |
| `.dockerignore` | Keeps `.git`, `output/`, `dist/`, `log/`, `.env*`, `gen/` (rebuilt if needed) out of the build context. |

A compose **profile** `app` in `docker-compose.local.yaml` runs the app from
`Dockerfile.local` with hot reload inside the same network, for people who don't
want Go installed locally.

## Local volumes
- All local data lives in **`output/docker-volume/<service>/`** inside this repo (bind mounts, not named volumes), so it is easy to inspect and delete.
- `output/` is added to `.gitignore`.
- `make docker-clean` stops the stack and deletes `output/docker-volume/` after asking for confirmation.
- Postgres is removed from compose (D2: Mongo only). Note in the docs how to add it back.

## Networking
- One docker network per compose project (name from `app.name`, renamed by `make init`).
- Services reach each other by service name; the app's local config (`config.local.json` / `.env.local`) points to `localhost` ports when running on the host, and the compose `app` profile overrides them with service names through environment variables.

## Makefile targets (D4)
`make up` / `make down` / `make logs` / `make ps` (local infra), `make up-app` (with the app profile), `make stack-up` / `make stack-down` (full `docker-compose.yaml`), `make docker-build` (production image), `make docker-clean`.

## CI
- `docker build` job on every PR (production `Dockerfile`), image scanned with a vulnerability scanner (e.g. Trivy) — non-blocking at first.
- A smoke job: `docker compose up` the full stack, wait for `/ready`, call one RPC, tear down.

## Other important things in this phase
- `.env.example` covers every variable the compose files and config need.
- Healthchecks on every compose service; the app waits for dependencies with `depends_on: condition: service_healthy`.
- Resource limits in the full-stack compose as an example for production.
- `docs/DEPLOY.md`: how to run each mode in production, migrations as a release step, exactly one scheduler, scaling workers, required env vars, health probes.

## Done when
- `make up && make dev` works on a clean machine with only Docker and Go installed.
- `make up-app` works with only Docker installed.
- `make stack-up` brings up api + worker + scheduler + infra and the smoke test passes.
- Deleting `output/docker-volume/` resets all local data.
