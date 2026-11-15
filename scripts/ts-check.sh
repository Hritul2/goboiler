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
#
# With `--tree DIR`, DIR is a project made by `make example-try DIR=...` (the
# skeleton with the User example laid over it). Instead of the four folders
# above, this checks, per client kind (fetch and axios):
#   example-<kind>-v1 / -v2   the example's generated /gen/ts/v1 and /v2 output
#                             (v2 comes from a throwaway cmd written into DIR)
#   web-<kind>                the frontend snippets of docs/EXAMPLE.md
#                             (web/<kind>/src/*.ts, *.tsx) against the v1 client,
#                             with exactOptionalPropertyTypes
#
# Needs go, pnpm and node. The npm packages are pinned in
# .github/ts-check/package.json + pnpm-lock.yaml (CI-only; not a dependency of
# the application). Works with BSD (macOS) and GNU tools.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

tree=""
case "${1:-}" in
  "") ;;
  --tree)
    [ $# -eq 2 ] || { echo "usage: $0 [--tree <example-try dir>]" >&2; exit 2; }
    tree="$(cd "$2" && pwd)" || exit 2 ;;
  *) echo "usage: $0 [--tree <example-try dir>]" >&2; exit 2 ;;
esac

tmp="$(mktemp -d "${TMPDIR:-/tmp}/ts-check.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT

cp -R .github/ts-check/. "$tmp/"

# tsc_check NAME DIR INCLUDE... : strict-check the files of DIR, in the scratch
# directory that has node_modules. Extra compiler options come from TS_EXTRA.
tsc_check() {
  local name=$1 dir=$2 include=$3
  printf '{ "extends": "%s/tsconfig.base.json", "compilerOptions": { %s }, "include": [%s] }\n' \
    "$tmp" "${TS_EXTRA:-}" "$include" > "$dir/tsconfig.json"
  echo "ts-check: tsc $name ($dir)"
  pnpm exec tsc -p "$dir/tsconfig.json"
}

if [ -n "$tree" ]; then
  [ -f "$tree/go.mod" ] || { echo "ts-check: $tree is not a project (no go.mod); make it with make example-try" >&2; exit 1; }
  [ -f "$tree/web/fetch/src/api.ts" ] && [ -d "$tree/internal/handler/userv2" ] ||
    { echo "ts-check: $tree is not an example tree; make it with make example-try" >&2; exit 1; }

  # v2 is not registered in the example's versions.go (the doc shows it as an
  # extra step), so generate v1 and v2 with a throwaway command in the tree.
  mkdir -p "$tree/cmd/genv2"
  cat > "$tree/cmd/genv2/main.go" <<'GO'
// Throwaway, written by scripts/ts-check.sh --tree: generates v1 and v2 of the
// User example. Not part of the example.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rs/zerolog"

	"MODULE/internal/api"
	"MODULE/internal/gen"
	"MODULE/internal/handler/user"
	"MODULE/internal/handler/userv2"
)

func main() {
	out := flag.String("out", "", "output directory")
	axios := flag.Bool("axios", false, "axios client")
	flag.Parse()
	opts := gen.Options{Client: gen.ClientFetch}
	if *axios {
		opts.Client = gen.ClientAxios
	}
	h := user.NewHandler(nil, zerolog.Nop())
	versions := []api.Version{
		{Name: "v1", Prefix: "/api/v1/rpc", Endpoints: user.Endpoints(h)},
		{Name: "v2", Prefix: "/api/v2/rpc", Endpoints: userv2.Endpoints(h)},
	}
	if err := gen.Write(*out, versions, opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
GO
  module="$(sed -n 's/^module \(.*\)$/\1/p' "$tree/go.mod" | head -n 1)"
  genv2="$tree/cmd/genv2/main.go"
  sed -e "s#MODULE/#$module/#" "$genv2" > "$genv2.new" && mv "$genv2.new" "$genv2"
  echo "ts-check: generating v1 + v2 clients from $tree"
  (cd "$tree" && go run ./cmd/genv2 -out "$tmp/example-fetch" && go run ./cmd/genv2 -axios -out "$tmp/example-axios")
  rm -rf "$tree/cmd/genv2"

  # Frontend snippets: they import "./gen/ts/v1", so give each its own copy.
  for kind in fetch axios; do
    web="$tmp/web-$kind"
    mkdir -p "$web/src/gen/ts"
    cp -R "$tree/web/$kind/src/." "$web/src/"
    cp -R "$tmp/example-$kind/v1" "$web/src/gen/ts/v1"
  done

  cd "$tmp"
  echo "ts-check: pnpm install"
  pnpm install --frozen-lockfile

  for kind in fetch axios; do
    for v in v1 v2; do
      tsc_check "example-$kind-$v" "$tmp/example-$kind/$v" '"./*.ts"'
    done
    TS_EXTRA='"exactOptionalPropertyTypes": true' tsc_check "web-$kind" "$tmp/web-$kind" '"./src/**/*"'
  done
  echo "ts-check: OK"
  exit 0
fi

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
  client="$(find "$src" -name 'client.ts' | head -n 1)"
  [ -n "$client" ] || { echo "ts-check: no client.ts under $src" >&2; exit 1; }
  ts_dir="$(dirname "$client")"
  printf '{ "extends": "%s/tsconfig.base.json", "include": ["./*.ts"] }\n' "$tmp" > "$ts_dir/tsconfig.json"
  echo "ts-check: tsc $name ($ts_dir)"
  pnpm exec tsc -p "$ts_dir/tsconfig.json"
done

echo "ts-check: OK"
