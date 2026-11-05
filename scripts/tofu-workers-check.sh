#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# The workers input of deploy/opentofu checks job types at plan, with no server to ask, so it holds two lists of its own. This
# fails when they drift from where they come from:
#
#   - job_types.json holds the job types of internal/platform/job_types.go, which my job types prints;
#   - keep_together.json holds the table in "Splitting workers by job type" of docs/operations/running.md.
#
# Usage: scripts/tofu-workers-check.sh [--write]
#   --write rewrites job_types.json from the Go list. The table of the document stays the source of keep_together.json, by hand.
#
# TOFU_WORKERS_ROOT overrides the repository root, for the tests of this script.
set -euo pipefail

root="${TOFU_WORKERS_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
module="$root/deploy/opentofu/modules/workers"
source_list="$root/internal/platform/job_types.go"
document="$root/docs/operations/running.md"

# The strings of the literal list, sorted.
go_types() {
  awk '/^var jobTypes = \[\]string\{/ { in_list = 1; next } in_list && /^}/ { exit } in_list { print }' "$source_list" |
    grep -oE '"[^"]+"' | tr -d '"' | LC_ALL=C sort -u
}

if [ "${1:-}" = "--write" ]; then
  go_types | jq -R . | jq -s . > "$module/job_types.json"
  exit 0
fi

status=0

types="$(go_types)"
[ -n "$types" ] || {
  echo "tofu-workers-check: no job types found in $source_list" >&2
  exit 1
}
if [ "$(jq -r '.[]' "$module/job_types.json" | LC_ALL=C sort -u)" != "$types" ]; then
  echo "tofu-workers-check: deploy/opentofu/modules/workers/job_types.json and internal/platform/job_types.go differ: run scripts/tofu-workers-check.sh --write" >&2
  status=1
fi

# Each row of the table as its sorted types, one row per line.
# shellcheck disable=SC2016 # the backticks are the table's
table="$(awk '
  /^\| Types \| What they keep in order \|/ { in_table = 1; next }
  in_table && !/^\|/ { exit }
  in_table && !/^\| ---/ { print }
' "$document" | cut -d'|' -f2 | while IFS= read -r row; do
  grep -oE '`[^`]+`' <<< "$row" | tr -d '`' | LC_ALL=C sort | paste -sd' ' -
done | LC_ALL=C sort)"
[ -n "$table" ] || {
  echo "tofu-workers-check: no table of the types to keep together found in $document" >&2
  exit 1
}
held="$(jq -r '.[] | sort | join(" ")' "$module/keep_together.json" | LC_ALL=C sort)"
if [ "$held" != "$table" ]; then
  echo "tofu-workers-check: deploy/opentofu/modules/workers/keep_together.json and the table in docs/operations/running.md differ:" >&2
  diff <(echo "$held") <(echo "$table") >&2 || true
  status=1
fi

# A type named in the keep-together list must exist.
while IFS= read -r type; do
  if ! grep -qxF "$type" <<< "$types"; then
    echo "tofu-workers-check: keep_together.json names $type, which is no job type" >&2
    status=1
  fi
done < <(jq -r '.[][]' "$module/keep_together.json")

exit "$status"
