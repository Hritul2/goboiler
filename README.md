# goboiler

[![CI](https://github.com/hritul2/goboiler/actions/workflows/ci.yml/badge.svg)](https://github.com/hritul2/goboiler/actions/workflows/ci.yml)

A Go (gin) backend boilerplate: typed RPC endpoints, a generated TypeScript
client, MongoDB behind swappable repository interfaces, Redis + asynq background
jobs, OpenTelemetry throughout.

> **Starting a new project from this? Read these two first:**
>
> 1. **[docs/HOW_TO_USE_IT.md](docs/HOW_TO_USE_IT.md)** — first steps (rename, config,
>    local services, run), the decisions this boilerplate makes for you and what
>    each one costs, and how to get the generated client into your frontend.
> 2. **[docs/EXAMPLE.md](docs/EXAMPLE.md)** — one complete feature (a `User` with a
>    welcome-email job, typed endpoints, a generated TypeScript client and a Clerk
>    webhook), built layer by layer with the full code and tests. The repository
>    itself ships **no domain code**; every place a domain plugs in is marked
>    `// ADD YOUR DOMAIN`.

## Quick start

```sh
make init MODULE=github.com/you/yourproject   # once; optional NAME=..., needs a clean git tree
make install-hooks
make setup                                    # creates .env and configs/config.local.json from their examples
docker compose -f docker-compose.local.yml up -d mongo redis mailpit
make dev                                      # then: curl localhost:8080/ping
make help                                     # every command
```

CI (`.github/workflows/ci.yml`) runs `make check` (fmt, lint, tests,
generated-client drift) and a TypeScript type-check of the generated clients
(`make ts-check`) on every push and pull request. Locally, `make check` needs
[golangci-lint v2](https://golangci-lint.run/docs/welcome/install/).

## Docs

| | |
|---|---|
| [docs/HOW_TO_USE_IT.md](docs/HOW_TO_USE_IT.md) | Start here: first steps, decisions and trade-offs, frontend delivery options |
| [docs/EXAMPLE.md](docs/EXAMPLE.md) | One feature end to end, with code and tests at every step |
| [docs/API_HANDLERS.md](docs/API_HANDLERS.md) | Adding backend functions, errors, the generated TypeScript client (fetch or axios) |
| [docs/BOOT_SEQUENCE.md](docs/BOOT_SEQUENCE.md) | What happens at startup and shutdown; the build-time generator |
| [docs/CODING_SEQUENCE.md](docs/CODING_SEQUENCE.md) | The layers, in the order to build them, and why |
| [docs/ASYNC_JOBS.md](docs/ASYNC_JOBS.md) | The background job queue |
| [docs/OBSERVABILITY.md](docs/OBSERVABILITY.md) | Tracing, metrics, logging |
| [docs/new-updated-boilerplate/](docs/new-updated-boilerplate/README.md) | Roadmap (plan only): kit module, process modes, migrations, adapters, generator, Docker |
