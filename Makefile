BINARY  := goboiler
OUT_DIR := dist
MAIN    := ./cmd

# TypeScript client generation (see `make gen`).
#   OUT       where the client is written; one folder per API version.
#   GEN_FLAGS extra flags for cmd/gen. Today: --axios (generate an axios-based
#             client.ts instead of the default fetch one).
# Override per call:  make gen GEN_FLAGS=--axios   make gen OUT=../web/src/api
# or pin a choice for the whole project by editing the defaults below, e.g.
#   GEN_FLAGS := --axios
# (then `make gen` and `make gen-check` always use it).
GEN_FLAGS ?=
OUT       ?= gen/ts

.DEFAULT_GOAL := help

# Non-empty when the compiled example (examples/user) is present; make init
# deletes it, and the example-* targets and their part in `make check` with it.
HAS_EXAMPLES := $(wildcard examples/user)

.PHONY: help setup ts-check init-smoke dev build run tidy fmt fmt-check lint install-hooks test test-v test-cover init gen gen-axios gen-check check example-try example-check example-docs example-docs-check

# Self-documenting help: any target line carrying a trailing `## description`
# is listed; `##@ Group` lines become section headers; a line starting with
# `## ` (and no target) is printed indented as a usage hint. Add `## ...` to a
# new target and it shows up here automatically.
##@ Help

help: ## Show this help (default target)
	@awk 'BEGIN { FS = ":.*## "; printf "Usage: make <target> [VAR=value]\n" } \
		/^[a-zA-Z0-9_-]+:.*## / { printf "  %-20s %s\n", $$1, $$2 } \
		/^##@/ { printf "\n%s\n", substr($$0, 5) } \
		/^## / { printf "      %s\n", substr($$0, 4) }' $(MAKEFILE_LIST)

##@ Setup

# NAME is honoured only when given on the command line: NAME is a common
# environment variable, and make would otherwise import it silently.
INIT_NAME := $(if $(filter command line,$(origin NAME)),$(NAME))

# Bootstrap a new project from this boilerplate: rewrites the Go module path
# (go.mod + every import and doc mention) and the project NAME (binary name,
# configs app.name, compose project, database names, README title), then runs
# go mod tidy and make gen. Run once, right after cloning/copying goboiler, on a
# clean git checkout (FORCE=1 to override). NAME defaults to the last element
# of MODULE. See scripts/init.sh for exactly what it changes.
#   make init MODULE=github.com/you/fruits
#   make init MODULE=github.com/you/fruits NAME=fruit-shop
init: ## Rename module + project for a new project: make init MODULE=<module-path> [NAME=<name>] [FORCE=1]
	@test -n "$(MODULE)" || (echo "Usage: make init MODULE=<new-module-path> [NAME=<project-name>] [FORCE=1]" >&2 && exit 1)
	./scripts/init.sh "$(MODULE)" "$(INIT_NAME)"

# Run once after cloning: points git at the committed hook script (native
# git, no pre-commit/lefthook framework needed for a pure-Go repo).
install-hooks: ## Point git at the committed .githooks directory
	git config core.hooksPath .githooks

# Creates the untracked local files from their committed examples, only when
# they do not exist yet (never overwrites). Plain `[ -f ] || cp` rather than
# `cp -n`, whose exit status differs between BSD and coreutils 9.x.
setup: ## Create .env and configs/config.local.json from their examples (never overwrites)
	@[ -f .env ] || { cp .env.example .env && echo "created .env"; }
	@[ -f configs/config.local.json ] || { cp configs/config.local.example.json configs/config.local.json && echo "created configs/config.local.json"; }

##@ Run

dev: ## Run the server from source (go run)
	go run $(MAIN)

build: ## Compile the binary into dist/
	@mkdir -p $(OUT_DIR)
	go build -o $(OUT_DIR)/$(BINARY) $(MAIN)

run: build ## Build, then run the compiled binary
	./$(OUT_DIR)/$(BINARY)

##@ Quality

fmt: ## Format all Go code (go fmt)
	go fmt ./...

fmt-check: ## Fail if any Go file is not gofmt-formatted
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

lint: ## Run golangci-lint
	golangci-lint run

test: ## Run unit tests
	go test ./internal/...

test-v: ## Run unit tests with verbose output
	go test ./internal/... -v

test-cover: ## Run unit tests and print per-function coverage
	go test ./internal/... -coverprofile=coverage.out
	go tool cover -func=coverage.out

ts-check: ## Type-check generated TS clients (needs pnpm)
	./scripts/ts-check.sh

# Boilerplate-only: scripts/init-smoke.sh is deleted by `make init`, and so is
# this target with it.
ifneq ($(wildcard scripts/init-smoke.sh),)
init-smoke: ## (boilerplate only) Prove make init on a scratch copy
	./scripts/init-smoke.sh
endif

# Everything CI (and a pre-push run) should pass: formatting, lint, unit
# tests, and generated-client drift.
check: fmt-check lint test gen-check $(if $(HAS_EXAMPLES),example-check example-docs-check) ## Run everything CI runs: fmt-check, lint, test, gen-check, example-check, example-docs-check

##@ Example

# The compiled User example (examples/user) and the page generated from it.
# `make init` deletes examples/ (unless KEEP_EXAMPLES=1), and these targets go
# with it. They work on a scratch copy of the working tree with the example
# laid over it (examples/exampletool), never on the tree itself, except
# example-docs, which rewrites docs/EXAMPLE.md.
EXAMPLE_TOOL := go run ./examples/exampletool

ifneq ($(HAS_EXAMPLES),)
example-try: ## Skeleton + the User example in DIR, to explore: make example-try DIR=/tmp/try
	@test -n "$(DIR)" || (echo "Usage: make example-try DIR=<empty-or-new-directory>" >&2 && exit 1)
	$(EXAMPLE_TOOL) materialize -dst "$(DIR)"
	cd "$(DIR)" && go run ./cmd/gen -out gen/ts && go run ./cmd/gen -axios -out gen/ts-axios
	@echo "example-try: $(DIR) is a project with the User example; try: cd $(DIR) && go test ./..."

example-check: ## Build, vet, lint, test and generate clients for skeleton + User example (scratch copy)
	go test ./examples/exampletool/...
	@tmp="$$(mktemp -d "$${TMPDIR:-/tmp}/example-check.XXXXXX")" && trap 'rm -rf "$$tmp"' EXIT && set -e && \
	$(EXAMPLE_TOOL) materialize -dst "$$tmp" && cd "$$tmp" && \
	echo "example-check: gofmt" && { test -z "$$(gofmt -l .)" || { gofmt -l . >&2; exit 1; }; } && \
	echo "example-check: go build" && go build ./... && \
	echo "example-check: go vet" && go vet ./... && \
	echo "example-check: golangci-lint" && golangci-lint run && \
	echo "example-check: go test" && go test ./... && \
	echo "example-check: cmd/gen (fetch, axios)" && go run ./cmd/gen -out gen/ts && go run ./cmd/gen -axios -out gen/ts-axios && \
	echo "example-check: OK"

# EXAMPLE_OUT is where example-docs writes; example-docs-check points it at a
# temp file.
EXAMPLE_OUT ?= docs/EXAMPLE.md

example-docs: ## Regenerate docs/EXAMPLE.md from examples/user (commit the result)
	@tmp="$$(mktemp -d "$${TMPDIR:-/tmp}/example-docs.XXXXXX")" && trap 'rm -rf "$$tmp"' EXIT && set -e && \
	$(EXAMPLE_TOOL) materialize -dst "$$tmp" && \
	(cd "$$tmp" && go run ./cmd/gen -out gen/ts) && \
	$(EXAMPLE_TOOL) docs -tree "$$tmp" -tmpl examples/user/EXAMPLE.md.tmpl -out "$(EXAMPLE_OUT)"

# CI guard, like gen-check: regenerate into a temp file, then fail if
# docs/EXAMPLE.md differs from it. Catches "changed the example but forgot make
# example-docs". It compares files, not git state, so it also works right after
# make init KEEP_EXAMPLES=1, before the rewrite is committed.
example-docs-check: ## Fail if docs/EXAMPLE.md is out of date with examples/user
	@tmp="$$(mktemp "$${TMPDIR:-/tmp}/example-docs-check.XXXXXX")" && trap 'rm -f "$$tmp"' EXIT && \
	$(MAKE) --no-print-directory example-docs EXAMPLE_OUT="$$tmp" && \
	{ diff -u docs/EXAMPLE.md "$$tmp" || { echo "example-docs-check: docs/EXAMPLE.md is out of date with examples/user. Run 'make example-docs' and commit the result." >&2; exit 1; }; }
endif

##@ Codegen

# Regenerate the typed TypeScript client (zod schemas, fetch or axios client,
# React Query hooks) for every version in internal/router/versions.go. Output
# goes to $(OUT)/<version>/*.ts. Run after changing any endpoint, request or
# response struct, then commit the result (or ship it to your frontend; see
# docs/HOW_TO_USE_IT.md).
gen: ## Regenerate the TypeScript client into OUT (default gen/ts)
## make gen GEN_FLAGS=--axios      axios-based client.ts instead of fetch
## make gen OUT=../web/src/api     write straight into a frontend repo
	go run ./cmd/gen -out $(OUT) $(GEN_FLAGS)

# Shortcut for `make gen GEN_FLAGS=--axios`. OUT still applies. Note that
# gen-check uses GEN_FLAGS, so if you rely on the axios client, pin
# GEN_FLAGS := --axios above (or pass it to gen-check) to keep CI consistent.
gen-axios: ## Regenerate the TypeScript client with an axios-based client.ts
## make gen-axios OUT=../web/src/api
	@$(MAKE) --no-print-directory gen OUT="$(OUT)" GEN_FLAGS="--axios"

# CI guard: regenerate, then fail if $(OUT) differs from what is committed
# (modified files via git diff, brand-new files via the untracked check,
# which plain git diff would miss). Catches "changed the Go API but forgot
# `make gen`". It compares against THIS repository's git tree, so it only
# makes sense when OUT is inside the repo; for an OUT elsewhere (a separate
# frontend repo) it refuses to run, before generating anything. Run the
# equivalent check in the repo that owns the generated files instead.
OUT_ABS    := $(abspath $(OUT))
OUT_INSIDE := $(filter $(CURDIR) $(CURDIR)/%,$(OUT_ABS))

gen-check: ## Fail if OUT (default gen/ts) is out of date with the Go API; OUT must be inside this repo
ifeq ($(OUT_INSIDE),)
	@echo "gen-check: OUT=$(OUT) is outside this repository ($(CURDIR)). gen-check compares against this repo's git tree, so it only works for an output directory inside it (default: gen/ts). If the client is committed in another repo, run the equivalent check there." >&2
	@exit 1
else
	@$(MAKE) --no-print-directory gen OUT="$(OUT)" GEN_FLAGS="$(GEN_FLAGS)"
	@git diff --exit-code -- $(OUT) || (echo "gen-check: $(OUT) is out of date with the Go API. Run 'make gen' (same OUT/GEN_FLAGS) and commit the result." >&2 && exit 1)
	@test -z "$$(git ls-files --others --exclude-standard -- $(OUT))" || (echo "gen-check: untracked files under $(OUT). Run 'make gen' (same OUT/GEN_FLAGS) and commit the result." >&2 && git ls-files --others --exclude-standard -- $(OUT) >&2 && exit 1)
endif

##@ Deps

tidy: ## Tidy go.mod / go.sum (go mod tidy)
	go mod tidy
