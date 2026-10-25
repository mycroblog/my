#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Fails when the desired-state schema of one dialect cannot upgrade a database of a base schema without losing or
# rewriting data (ADR-0087 §3): `scripts/task.sh db:lint BASE=<ref> DB_DIALECT=sqlite|postgres` runs it.
#
#   scripts/db-lint.sh [options]
#     --dialect sqlite|postgres  the dialect (required)
#     --dev-url URL              an empty dev database that Atlas may write to (required)
#     --base REF                 compare with schema/<dialect>/ at the merge base of REF and HEAD (default: origin/main)
#     --base-dir DIR             compare with the schema files in DIR instead (tests)
#     --target-dir DIR           the schema to check (default: schema/<dialect>/ of the working tree)
#     --allow-destructive        report destructive changes but do not fail on them; the target must still apply
#     --atlas PATH               the Atlas binary (default: bin/atlas)
#
# It writes two migrations into a temporary directory, empty -> base and base -> target, with `atlas migrate diff`,
# and runs `atlas migrate lint --latest 1` on the second, which replays it on the dev database. No migration file is
# left in the repository (ADR-0023). A change is destructive when:
#
#   - the lint reports any diagnostic, warnings included (dropped tables and columns, a column renamed, which is a drop
#     and an add, NOT NULL on an existing column or a new one without DEFAULT, a new unique index, ...);
#   - `atlas schema inspect` of base and target shows a column of a table in both whose type changed, or a CHECK new to
#     a table in both whose expression names a column of the base table (a CHECK on new columns alone is additive).
#
# Exit status: 0 when the change is additive (or destructive with --allow-destructive), 1 when it is destructive or
# the target does not apply on top of the base, 2 on a usage error.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

usage() {
  sed -n '7,15p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 2
}

dialect="" dev_url="" base="origin/main" base_dir="" target_dir="" allow=0 atlas="$root/bin/atlas"
while [ "$#" -gt 0 ]; do
  case "$1" in
    --dialect) dialect="${2:-}" && shift ;;
    --dev-url) dev_url="${2:-}" && shift ;;
    --base) base="${2:-}" && shift ;;
    --base-dir) base_dir="${2:-}" && shift ;;
    --target-dir) target_dir="${2:-}" && shift ;;
    --allow-destructive) allow=1 ;;
    --atlas) atlas="${2:-}" && shift ;;
    *) usage ;;
  esac
  shift
done
case "$dialect" in sqlite | postgres) ;; *) usage ;; esac
[ -n "$dev_url" ] || usage
[ -n "$target_dir" ] || target_dir="$root/schema/$dialect"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/base" "$work/target" "$work/migrations" "$work/tmp"

# Atlas locks a SQLite dev database with a file in the temporary directory, named after the operation, so two runs at
# once (the CI steps of db:lint and db:lint-test) would wait on each other's lock and fail: give each run its own
# temporary directory and its own in-memory dev database.
export TMPDIR="$work/tmp"
if [ "$dialect" = sqlite ]; then
  case "$dev_url" in
    sqlite://*mode=memory*) dev_url="sqlite://dblint-$$?mode=memory" ;;
  esac
fi

# The base: the files of DIR, or of schema/<dialect>/ at the commit BASE (none when the commit has no such directory).
if [ -n "$base_dir" ]; then
  cp "$base_dir"/*.sql "$work/base/"
  label="$base_dir"
else
  # The merge base of BASE and HEAD, so that a change merged into BASE after the branch started is not read as this
  # branch undoing it. A shallow clone without the history (the merge queue's) compares with BASE itself.
  ref="$base"
  base="$(git -C "$root" merge-base "$base" HEAD 2> /dev/null || echo "$base")"
  if git -C "$root" cat-file -e "$base:schema/$dialect" 2> /dev/null; then
    git -C "$root" archive --format=tar "$base" "schema/$dialect" | tar -x -C "$work/base" --strip-components=2
  fi
  label="$ref (merge base $(git -C "$root" rev-parse --short "$base^{commit}"))"
fi
cp "$target_dir"/*.sql "$work/target/"
echo "db:lint ($dialect): $label -> $target_dir"

# Atlas reads an empty directory as an empty schema, but `migrate diff` needs a file to read.
[ -n "$(ls -A "$work/base")" ] || : > "$work/base/empty.sql"

# migrate diff names each file after the second it runs in, so two files may share a version: rename them so that
# the base sorts first, and hash the directory again.
"$atlas" migrate diff base --dir "file://$work/migrations" --to "file://$work/base" --dev-url "$dev_url" > /dev/null
for f in "$work/migrations"/*_base.sql; do
  if [ -e "$f" ]; then mv "$f" "$work/migrations/1_base.sql"; fi
done
"$atlas" migrate hash --dir "file://$work/migrations"
"$atlas" migrate diff target --dir "file://$work/migrations" --to "file://$work/target" --dev-url "$dev_url" > /dev/null
changes=0
for f in "$work/migrations"/*_target.sql; do
  [ -e "$f" ] || continue
  mv "$f" "$work/migrations/2_target.sql"
  changes=1
done
if [ "$changes" = 0 ]; then
  echo "db:lint ($dialect): no schema change"
  exit 0
fi
"$atlas" migrate hash --dir "file://$work/migrations"
echo "--- the change, as Atlas plans it on an existing database:"
sed -n '/^-- /!p' "$work/migrations/2_target.sql"

findings="$work/findings"
: > "$findings"

# The lint. It exits with 1 both when an analyzer reports an error (a dropped column) and when the migration does not
# run on the dev database (a type PostgreSQL cannot cast, say); the report tells them apart: a failed run is the error
# of a step (Replay Migration Files), an analyzer's error is in the result of its step. A change that does not apply
# fails whatever the approval.
lint_status=0
"$atlas" migrate lint --dir "file://$work/migrations" --dev-url "$dev_url" --latest 1 --format '{{ json . }}' \
  > "$work/lint.json" || lint_status=$?
jq -r '.Files[]? | . as $f | .Reports[]?.Diagnostics[]? |
  "lint " + .Code + " (L" + ($f.Text[0:.Pos] | split("\n") | length | tostring) + "): " + .Text' \
  "$work/lint.json" >> "$findings" 2> /dev/null || true
if ! jq -e '[.Steps[]? | select(.Error != null)] | length == 0' "$work/lint.json" > /dev/null 2>&1 ||
  { [ "$lint_status" != 0 ] && [ ! -s "$findings" ]; }; then
  echo "db:lint ($dialect): the change does not apply on top of the base:" >&2
  jq -r '.Steps[]? | select(.Error != null) | "  " + .Name + ": " + .Error' "$work/lint.json" >&2 2> /dev/null ||
    cat "$work/lint.json" >&2
  exit 1
fi

# The inspection. `schema inspect` prints HCL, two spaces for each level; reduce it to one line per column type and
# per CHECK: "column<TAB>table<TAB>column<TAB>type" and "check<TAB>table<TAB>expression".
inspect() {
  "$atlas" schema inspect --url "file://$1" --dev-url "$dev_url" |
    awk '
      /^table "/ { split($0, p, "\""); table = p[2]; next }
      /^[^ ]/ { table = ""; next }
      table == "" { next }
      /^  column "/ { split($0, p, "\""); column = p[2]; block = "column"; next }
      /^  check / { block = "check"; next }
      /^  [^ ]/ { block = ""; next }
      block == "column" && /^    type = / { sub(/^    type = /, ""); printf "column\t%s\t%s\t%s\n", table, column, $0; next }
      block == "check" && /^    expr = / { sub(/^    expr = /, ""); printf "check\t%s\t%s\n", table, $0; next }
    '
}
inspect "$work/base" > "$work/base.tsv"
inspect "$work/target" > "$work/target.tsv"
awk -F '\t' '
  # inlist(EXPR) reduces a CHECK that a column is one of a list of values, "(kind IN (\x27a\x27, \x27b\x27))" in
  # SQLite and "(kind = ANY (ARRAY[\x27a\x27::text, \x27b\x27::text]))" as PostgreSQL prints it, to
  # "kind<TAB>,\x27a\x27,\x27b\x27,", and any other expression to "". PostgreSQL prints a list of one value as
  # "(kind = \x27a\x27::text)".
  function inlist(e,    col) {
    gsub(/^"|"$/, "", e)
    # SQLite keeps the expression as written, line breaks (\n in HCL) and indentation included.
    gsub(/\\n/, " ", e)
    gsub(/[ \t]+/, " ", e)
    gsub(/\( /, "(", e)
    gsub(/ \)/, ")", e)
    while (e ~ /^\(.*\)$/) e = substr(e, 2, length(e) - 2)
    if (!match(e, /^[A-Za-z_][A-Za-z0-9_]* /)) return ""
    col = substr(e, 1, RLENGTH - 1)
    e = substr(e, RLENGTH + 1)
    if (e ~ /^IN \(.*\)$/) e = substr(e, 5, length(e) - 5)
    else if (e ~ /^= ANY \(ARRAY\[.*\]\)$/) e = substr(e, 14, length(e) - 15)
    else if (e ~ /^= \047[^\047]*\047(::[a-z ]+)?$/) e = substr(e, 3)
    else return ""
    gsub(/::[a-z ]+/, "", e)
    gsub(/ *, */, ",", e)
    return col "\t," e ","
  }
  # relaxes(TABLE, NEW) is whether a CHECK of the base table on the same column allows a subset of the values the
  # list NEW allows: adding a value to the list of an enum relaxes the constraint.
  function relaxes(t, n,    k, b, nb, nn, i, bv, nv, vals) {
    if (n == "") return 0
    split(n, nv, "\t")
    for (k in checks) {
      split(k, b, SUBSEP)
      if (b[1] != t) continue
      split(inlist(b[2]), bv, "\t")
      if (bv[1] != nv[1]) continue
      nb = split(substr(bv[2], 2, length(bv[2]) - 2), vals, ",")
      nn = 0
      for (i = 1; i <= nb; i++) if (index(nv[2], "," vals[i] ",") > 0) nn++
      if (nn == nb) return 1
    }
    return 0
  }
  # The base: its columns, with types, and its CHECKs.
  NR == FNR {
    if ($1 == "column") { type[$2 SUBSEP $3] = $4; tables[$2] = 1; columns[$2] = columns[$2] " " $3 " " }
    if ($1 == "check") checks[$2 SUBSEP $3] = 1
    next
  }
  $1 == "column" && (($2 SUBSEP $3) in type) && type[$2 SUBSEP $3] != $4 {
    printf "inspect: the type of column \"%s\".\"%s\" changes from %s to %s\n", $2, $3, type[$2 SUBSEP $3], $4
  }
  $1 == "check" && ($2 in tables) && !(($2 SUBSEP $3) in checks) && !relaxes($2, inlist($3)) {
    # The words of the expression; any that names a column of the base table makes the CHECK apply to existing rows.
    n = split($3, words, /[^A-Za-z0-9_]+/)
    for (i = 1; i <= n; i++) {
      if (words[i] != "" && index(columns[$2], " " words[i] " ") > 0) {
        printf "inspect: table \"%s\" gets a CHECK on existing columns: %s\n", $2, $3
        break
      }
    }
  }
' "$work/base.tsv" "$work/target.tsv" >> "$findings"

if [ ! -s "$findings" ]; then
  echo "db:lint ($dialect): the change is additive"
  exit 0
fi
echo "--- destructive changes:"
sed 's/^/  /' "$findings"
if [ "$allow" = 1 ]; then
  echo "db:lint ($dialect): destructive changes allowed (--allow-destructive); the change applies on top of the base"
  exit 0
fi
cat >&2 << 'EOF'
db:lint: the change can lose or rewrite data of an existing database. Split it into expand and contract
(schema/README.md#changing-the-schema); a change to a projection or a contract needs the decider's approval,
the db:destructive label on the pull request.
EOF
exit 1
