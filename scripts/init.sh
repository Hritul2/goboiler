#!/usr/bin/env bash
# Bootstraps a new project from this boilerplate: rewrites the module path
# (go.mod + every import) from OLD_MODULE to the path you give it, renames
# the build binary in the Makefile to match, and tidies go.mod.
#
# Usage (run from the repo root, right after cloning/copying goboiler for a
# new project):
#
#   ./scripts/init.sh github.com/hritul2/fruits/apple
#
# Safe to run once per new project. Not idempotent against itself — running
# it twice with a different path just renames again, which is fine.
set -euo pipefail

OLD_MODULE="github.com/hritul2/goboiler"

if [ $# -ne 1 ]; then
  echo "Usage: $0 <new-module-path>" >&2
  echo "Example: $0 github.com/hritul2/fruits/apple" >&2
  exit 1
fi

NEW_MODULE="$1"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel 2>/dev/null || dirname "$SCRIPT_DIR")"

if [ "$NEW_MODULE" = "$OLD_MODULE" ]; then
  echo "New module path matches the current one — nothing to do."
  exit 0
fi

if ! grep -q "^module $OLD_MODULE\$" go.mod; then
  echo "go.mod does not declare module $OLD_MODULE — has this repo already been renamed?" >&2
  exit 1
fi

echo "Renaming module: $OLD_MODULE -> $NEW_MODULE"

FILES="$(grep -rlF "$OLD_MODULE" --exclude-dir=.git .)"
echo "$FILES" | while IFS= read -r f; do
  [ -n "$f" ] || continue
  sed -i '' "s#${OLD_MODULE}#${NEW_MODULE}#g" "$f"
done

# Keep the build binary name in sync with the new project name.
NEW_BIN="$(basename "$NEW_MODULE")"
if grep -q '^BINARY  := goboiler$' Makefile; then
  sed -i '' "s/^BINARY  := goboiler\$/BINARY  := $NEW_BIN/" Makefile
fi

go mod tidy
gofmt -l . >/dev/null 2>&1 || true

echo ""
echo "Done. Module is now $NEW_MODULE (binary: $NEW_BIN)."
echo "Review with 'git diff', then commit."
