#!/usr/bin/env bash
# Proves `make init` on a scratch copy of what is COMMITTED (git archive HEAD),
# so a broken init.sh is caught before a user hits it. Boilerplate-only: init
# deletes this script (scripts/init.sh holds the old path as data, so the leftover
# check skips it). Run via `make init-smoke`; CI runs it on Linux (GNU sed)
# and macOS (BSD sed). Works with BSD and GNU tools.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

dst="$(mktemp -d "${TMPDIR:-/tmp}/init-smoke.XXXXXX")"
trap 'rm -rf "$dst"' EXIT

fail() { echo "init-smoke: FAIL: $*" >&2; exit 1; }
ok()   { echo "init-smoke: ok: $*"; }

git archive HEAD | tar -x -C "$dst"
cd "$dst"
git init -q
git add -A
git -c user.name=ci -c user.email=ci@example.invalid commit -qm init

make init MODULE=github.com/example/fruits NAME=fruits

grep -qx 'module github.com/example/fruits' go.mod || fail "go.mod module not rewritten"
leftover="$(git grep -nIF 'github.com/hritul2/goboiler' -- . ':!go.sum' ':!scripts/init.sh' || true)"
[ -z "$leftover" ] || fail "old module path left behind:
$leftover"
grep -q '^BINARY  := fruits$' Makefile || fail "Makefile BINARY not renamed"
grep -q '"name": "fruits"' configs/config.json || fail "configs/config.json app.name not renamed"
grep -q '^name: fruits$' docker-compose.local.yml || fail "compose project name not renamed"
grep -q '^DATABASE_NAME=fruits$' .env.example || fail ".env.example DATABASE_NAME not renamed"
[ "$(head -n 1 README.md)" = "# fruits" ] || fail "README title not renamed"
[ ! -e .github/workflows/boilerplate.yml ] || fail "boilerplate.yml was not removed"
[ ! -e scripts/init-smoke.sh ] || fail "init-smoke.sh was not removed"
ok "rewrite assertions"

make check
make build
test -x dist/fruits || fail "dist/fruits missing"
ok "make check and make build"

if make init MODULE=github.com/example/other >/dev/null 2>&1; then
  fail "a second make init should have been refused"
fi
ok "second init refused"

echo "init-smoke: OK"
