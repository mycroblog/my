#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/db-lint.sh on the fixtures of schema/testdata/lint/<dialect>/: each case directory is a schema that
# the script compares with base/. The name of a case gives what the script must do with it:
#
#   pass-*   additive: passes
#   fail-*   destructive: fails, and passes with --allow-destructive (the approval label, the merge queue)
#   error-*  does not apply on top of the base: fails, also with --allow-destructive
#
#   scripts/db-lint-test.sh --dialect sqlite|postgres --dev-url URL [--atlas PATH]
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
dialect="" args=()
while [ "$#" -gt 0 ]; do
  case "$1" in
    --dialect) dialect="${2:-}" ;;
    --dev-url | --atlas) args+=("$1" "${2:-}") ;;
    *)
      echo "usage: scripts/db-lint-test.sh --dialect sqlite|postgres --dev-url URL [--atlas PATH]" >&2
      exit 2
      ;;
  esac
  shift 2
done
fixtures="$root/schema/testdata/lint/$dialect"
if [ -z "$dialect" ] || [ ! -d "$fixtures/base" ]; then
  echo "db-lint-test.sh: no fixtures in $fixtures" >&2
  exit 2
fi

failed=0
log="$(mktemp)"
trap 'rm -f "$log"' EXIT

# run CASE [OPTION...] runs the script on CASE and returns its exit status, keeping its output in $log.
run() {
  local case="$1"
  shift
  "$root/scripts/db-lint.sh" --dialect "$dialect" "${args[@]}" --base-dir "$fixtures/base" \
    --target-dir "$fixtures/$case" "$@" > "$log" 2>&1
}

# expect CASE WANT [OPTION...] fails the test when the script's exit status is not WANT.
expect() {
  local case="$1" want="$2" got=0
  shift 2
  run "$case" "$@" || got=$?
  if [ "$got" != "$want" ]; then
    echo "FAIL: $dialect $case $*: want exit status $want, got $got" >&2
    sed 's/^/  /' "$log" >&2
    failed=1
    return
  fi
  # The `-- want: TEXT` lines of the case: what the output must say, so that a case fails for its own reason.
  local line
  while IFS= read -r line; do
    if ! grep -qF -- "$line" "$log"; then
      echo "FAIL: $dialect $case $*: want the output to contain '$line'" >&2
      sed 's/^/  /' "$log" >&2
      failed=1
      return
    fi
  done < <(sed -n 's/^-- want: //p' "$fixtures/$case/01.sql")
  echo "ok: $dialect $case $* (exit status $got)"
}

for dir in "$fixtures"/*/; do
  case="$(basename "$dir")"
  case "$case" in
    base) expect base 0 ;;
    pass-*) expect "$case" 0 ;;
    fail-*)
      expect "$case" 1
      expect "$case" 0 --allow-destructive
      ;;
    error-*)
      expect "$case" 1
      expect "$case" 1 --allow-destructive
      ;;
    *)
      echo "FAIL: $dialect $case: a case is named pass-*, fail-*, or error-*" >&2
      failed=1
      ;;
  esac
done
exit "$failed"
