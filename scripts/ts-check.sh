#!/usr/bin/env bash
# Type-checks the TypeScript clients that cmd/gen produces, with a strict
# tsconfig, so a generator change that emits invalid TS fails CI.
#
# Four folders are checked, in a scratch directory (nothing is written to the
# repository):
#   skeleton-fetch / skeleton-axios   what `go run ./cmd/gen` emits for the
#                                     current (still empty) skeleton API
#   fixture-fetch / fixture-axios     the generator's golden fixtures, the only
#                                     output that contains real endpoints
#
# Needs go, pnpm and node. The npm packages are pinned in
# .github/ts-check/package.json + pnpm-lock.yaml (CI-only; not a dependency of
# the application). Works with BSD (macOS) and GNU tools.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

tmp="$(mktemp -d "${TMPDIR:-/tmp}/ts-check.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT

cp -R .github/ts-check/. "$tmp/"

echo "ts-check: generating clients"
go run ./cmd/gen -out "$tmp/skeleton-fetch"
go run ./cmd/gen -axios -out "$tmp/skeleton-axios"

golden="$root/internal/gen/testdata"
for kind in fetch axios; do
  dir="$tmp/fixture-$kind"
  mkdir -p "$dir"
  for f in schemas hooks index; do
    cp "$golden/fixture_$f.ts.golden" "$dir/$f.ts"
  done
  if [ "$kind" = axios ]; then
    cp "$golden/fixture_client.axios.ts.golden" "$dir/client.ts"
  else
    cp "$golden/fixture_client.ts.golden" "$dir/client.ts"
  fi
done

cd "$tmp"
echo "ts-check: pnpm install"
pnpm install --frozen-lockfile

# cmd/gen writes into <out>/<version>/, so find the folder holding the .ts files.
for name in skeleton-fetch skeleton-axios fixture-fetch fixture-axios; do
  src="$tmp/$name"
  ts_dir="$(dirname "$(find "$src" -name 'client.ts' | head -n 1)")"
  [ -n "$ts_dir" ] && [ -d "$ts_dir" ] || { echo "ts-check: no client.ts under $src" >&2; exit 1; }
  printf '{ "extends": "%s/tsconfig.base.json", "include": ["./*.ts"] }\n' "$tmp" > "$ts_dir/tsconfig.json"
  echo "ts-check: tsc $name ($ts_dir)"
  pnpm exec tsc -p "$ts_dir/tsconfig.json"
done

echo "ts-check: OK"
