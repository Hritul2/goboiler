#!/usr/bin/env bash
# Bootstraps a new project from this boilerplate. Run it ONCE, right after
# cloning/copying goboiler for the new project, from a clean git checkout:
#
#   make init MODULE=github.com/you/fruits                  # NAME defaults to "fruits"
#   make init MODULE=github.com/you/fruits NAME=fruit-shop
#   ./scripts/init.sh github.com/you/fruits [fruit-shop]
#
# It changes two kinds of thing:
#
#  1. The Go module path (github.com/hritul2/goboiler -> MODULE): go.mod and
#     every tracked text file that mentions it, including docs, except this
#     script and gen/ (regenerated instead).
#  2. The project NAME, wherever the project's identity (not its history) lives:
#       Makefile                  BINARY
#       configs/*.json            app.name (OpenTelemetry service name, logs)
#       docker-compose.local.yml  compose project name, POSTGRES_DB
#       .env.example              DATABASE_NAME and the sample Postgres URL
#       README.md                 the title line
#     Docs that describe the boilerplate itself are left alone.
#
# It also deletes the files that only make sense in the boilerplate repository
# itself: .github/workflows/boilerplate.yml and scripts/init-smoke.sh (the CI
# proof that this very script works).
#
# Then it runs `go mod tidy` and `make gen`.
#
# Safety:
#  - needs a git checkout with a clean tree (so a mistake is one `git checkout .`
#    away); set FORCE=1 to run on a dirty tree anyway,
#  - MODULE and NAME are validated before anything is touched,
#  - only git-tracked (or untracked-but-not-ignored) text files are edited:
#    .git, ignored files such as .env, and binaries are never touched.
#
# Portable: works with BSD (macOS) and GNU (Linux) sed; the only grep options
# used are the POSIX -E/-F/-q (file discovery uses `git grep`). Needs bash, git,
# sed, grep, go, make.
set -euo pipefail

OLD_MODULE="github.com/hritul2/goboiler"
OLD_NAME="goboiler"                 # Makefile BINARY, compose project, DB names, README title
OLD_APP_NAME="golang-boilerplate"   # configs/*.json app.name

die()  { echo "init: $*" >&2; exit 1; }
warn() { echo "init: warning: $*" >&2; }
log()  { echo "  $*"; }

# --- portable helpers -------------------------------------------------------

# sed_inplace EXPR FILE — in-place edit that works with BSD and GNU sed (they
# disagree on `-i`: BSD wants a suffix argument, GNU takes it attached). Using
# an explicit backup suffix is the common subset; the backup is removed.
sed_inplace() {
  sed -i.bak -e "$1" "$2" && rm -f "$2.bak"
}

# esc_regex STRING — escape STRING for use in a basic-regex sed pattern with
# `#` as the delimiter.
esc_regex() {
  printf '%s' "$1" | sed 's/[][\.*^$#]/\\&/g'
}

# rename_in FILE OLD NEW — replace the literal OLD with NEW in FILE, or warn if
# OLD is not there (the file was already customised).
rename_in() {
  local file=$1 old=$2 new=$3
  if [ ! -f "$file" ]; then
    warn "$file not found; skipped"
    return
  fi
  if grep -qF -- "$old" "$file"; then
    sed_inplace "s#$(esc_regex "$old")#$new#g" "$file"
    log "$file: $old -> $new"
  else
    warn "$file: expected '$old' not found; skipped (already customised?)"
  fi
}

# --- arguments --------------------------------------------------------------

[ $# -ge 1 ] && [ $# -le 2 ] || die "usage: $0 <module-path> [name]   (e.g. $0 github.com/you/fruits fruits)"
NEW_MODULE=$1
NAME=${2:-}

# MODULE: Go module path characters only. No `#`, `&`, `\`, spaces or quotes —
# the value is used inside sed replacements, so this is also what keeps it safe.
case "$NEW_MODULE" in
  ''|-*|/*|*/|*//*|*..*|*[!A-Za-z0-9._~/-]*)
    die "'$NEW_MODULE' is not a valid module path (letters, digits and . _ ~ - separated by single /, e.g. github.com/you/fruits)" ;;
esac

# NAME defaults to the last path element (skipping a /v2-style major-version
# suffix), lower-cased, with . and _ turned into -.
if [ -z "$NAME" ]; then
  NAME=${NEW_MODULE##*/}
  case "$NAME" in
    v[0-9]*)
      rest=${NEW_MODULE%/*}
      [ "$rest" != "$NEW_MODULE" ] && NAME=${rest##*/} ;;
  esac
  NAME=$(printf '%s' "$NAME" | sed 'y/ABCDEFGHIJKLMNOPQRSTUVWXYZ._/abcdefghijklmnopqrstuvwxyz--/')
  echo "init: NAME not given; using '$NAME' (override with NAME=...)"
fi
# NAME ends up as the binary name, app name, compose project and (with - -> _)
# database name, so keep it to lowercase letters, digits and hyphens.
printf '%s' "$NAME" | grep -Eq '^[a-z]([a-z0-9-]{0,48}[a-z0-9])?$' ||
  die "'$NAME' is not a valid project name (start with a letter; lowercase letters, digits and - only; no trailing -). Pass NAME=..."
DBNAME=$(printf '%s' "$NAME" | sed 's/-/_/g')

if [ "$NEW_MODULE" = "$OLD_MODULE" ]; then
  echo "init: new module path matches the current one; nothing to do."
  exit 0
fi

# --- preconditions ----------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."   # the project root, wherever the git toplevel is

[ -f go.mod ] || die "no go.mod in $(pwd)"
grep -q "^module $OLD_MODULE\$" go.mod ||
  die "go.mod does not declare module $OLD_MODULE; has this project already been initialised?"

git rev-parse --is-inside-work-tree >/dev/null 2>&1 ||
  die "not a git checkout. Run 'git init && git add -A && git commit -m initial' first so the rewrite can be reviewed and undone."

if [ -n "$(git status --porcelain -- .)" ] && [ "${FORCE:-}" != "1" ]; then
  die "the working tree has uncommitted changes. Commit or stash them first (so 'git diff' shows exactly what init changed), or re-run with FORCE=1."
fi

# Boilerplate-only files: the init-smoke CI job and its script test this
# script, which a generated project no longer has any use for.
rm -f .github/workflows/boilerplate.yml scripts/init-smoke.sh

echo "init: module  $OLD_MODULE -> $NEW_MODULE"
echo "init: project $OLD_NAME -> $NAME  (database name: $DBNAME)"

# --- 1. module path ---------------------------------------------------------

OLD_RE=$(esc_regex "$OLD_MODULE")
changed=0
changed_go=""
# `git grep -I -l` lists the files that contain the old module path, skipping
# binary files itself: tracked files plus untracked-but-not-ignored ones, never
# .git, .env, log/ or dist/. Using git (not `grep -I` over `git ls-files`) keeps
# the text-file test identical on every platform, whatever grep is installed.
while IFS= read -r -d '' f; do
  case "$f" in
    scripts/init.sh) continue ;;   # holds OLD_MODULE as data
    gen/*) continue ;;             # generated; regenerated below
    go.sum) continue ;;            # dependencies only
  esac
  [ -f "$f" ] && [ ! -L "$f" ] || continue
  sed_inplace "s#$OLD_RE#$NEW_MODULE#g" "$f"
  changed=$((changed + 1))
  case "$f" in *.go) changed_go="$changed_go
$f" ;; esac
done < <(git grep -I -l -z -F --untracked -e "$OLD_MODULE" -- . || true)
echo "init: module path rewritten in $changed file(s)"

# --- 2. project name --------------------------------------------------------

echo "init: project name:"
rename_in Makefile "BINARY  := $OLD_NAME" "BINARY  := $NAME"
for f in configs/*.json; do
  [ -f "$f" ] || continue
  # Only config.json is required to carry the name; the .local override
  # normally does not.
  if grep -qF -- "\"$OLD_APP_NAME\"" "$f"; then
    rename_in "$f" "\"$OLD_APP_NAME\"" "\"$NAME\""
  elif [ "$f" = "configs/config.json" ]; then
    warn "$f: app.name \"$OLD_APP_NAME\" not found; set it yourself"
  fi
done
rename_in docker-compose.local.yml "name: $OLD_NAME" "name: $NAME"
rename_in docker-compose.local.yml "POSTGRES_DB: ${OLD_NAME}_local" "POSTGRES_DB: ${DBNAME}_local"
rename_in .env.example "DATABASE_NAME=$OLD_NAME" "DATABASE_NAME=$DBNAME"
rename_in .env.example "5432/$OLD_NAME?" "5432/$DBNAME?"
if [ -f README.md ] && [ "$(head -n 1 README.md)" = "# $OLD_NAME" ]; then
  sed_inplace "1s|^# $OLD_NAME\$|# $NAME|" README.md
  log "README.md: title -> # $NAME"
else
  warn "README.md: first line is not '# $OLD_NAME'; skipped"
fi

# --- 3. tidy, format, regenerate -------------------------------------------

if [ -n "$changed_go" ]; then
  # Renaming can change import ordering; keep `make fmt-check` green.
  printf '%s\n' "$changed_go" | grep -v '^$' | while IFS= read -r f; do gofmt -w "$f"; done
fi

echo "init: go mod tidy"
go mod tidy || warn "go mod tidy failed (offline?); run it yourself"

echo "init: make gen"
make gen || warn "make gen failed; run it yourself once the project builds"

cat <<EOF

Done. Module is now $NEW_MODULE, project name '$NAME'.

Next:
  1. Review:   git diff --stat && git diff
  2. Your own .env (not tracked) still has the old DATABASE_NAME; set it to $DBNAME.
  3. Check:    make check      # needs golangci-lint v2
  4. Commit.
EOF
