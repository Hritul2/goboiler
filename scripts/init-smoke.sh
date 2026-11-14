#!/usr/bin/env bash
# Proves `make init` on a scratch copy of what is COMMITTED (git archive HEAD),
# so a broken init.sh is caught before a user hits it. Boilerplate-only: init
# deletes this script (scripts/init.sh holds the old path as data, so the leftover
# check skips it). Run via `make init-smoke`; CI runs it on Linux (GNU sed)
# and macOS (BSD sed). Works with BSD and GNU tools.
#
# KEEP_EXAMPLES=1 make init-smoke  exercises `make init KEEP_EXAMPLES=1`: the
# project keeps examples/ and its docs/EXAMPLE.md must stay generated and pass
# make check under the new module path. Without it examples/ must be gone.
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

hash_before="$(grep -o 'Source hash: sha256:[0-9a-f]*' docs/EXAMPLE.md || true)"
[ -n "$hash_before" ] || fail "docs/EXAMPLE.md has no source hash before init"

make init MODULE=github.com/example/fruits NAME=fruits KEEP_EXAMPLES="${KEEP_EXAMPLES:-}"

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
if [ "${KEEP_EXAMPLES:-}" = 1 ]; then
  [ -d examples/user/_overlay ] || fail "KEEP_EXAMPLES=1 but examples/ is gone"
  [ "$(grep -o 'Source hash: sha256:[0-9a-f]*' docs/EXAMPLE.md || true)" = "$hash_before" ] ||
    fail "docs/EXAMPLE.md source hash changed with the module path"
  grep -q 'github.com/example/fruits/internal/services/user' docs/EXAMPLE.md || fail "docs/EXAMPLE.md was not rewritten to the new module"
  ok "examples/ kept and docs/EXAMPLE.md regenerated"
else
  [ ! -e examples ] || fail "examples/ was not removed"
  grep -q 'Snapshot of the compiled example' docs/EXAMPLE.md || fail "docs/EXAMPLE.md provenance line was not replaced"
  ! grep -q 'Source hash' docs/EXAMPLE.md || fail "docs/EXAMPLE.md still claims to be generated"
  ok "examples/ removed and docs/EXAMPLE.md marked as a snapshot"
fi
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
