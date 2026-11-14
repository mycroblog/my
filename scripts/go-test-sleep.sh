#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Refuses time.Sleep in the Go tests (ADR-0170 §7): a test that waits a fixed time is slow, and flaky under load.
# `scripts/task.sh go:lint` and `agentkit check` run it.
#
#   scripts/go-test-sleep.sh [options]
#     --base REF   the baseline may only shrink from the merge base of REF and HEAD (default: origin/redesign-web)
#     --root DIR   the repository (default: the one this script is in)
#
# The uses that predate the check are in scripts/go-test-sleep.baseline, one "<path> <count>" per file. It fails when
# a *_test.go file outside the baseline uses time.Sleep, when a file of the baseline uses it more often than its line
# says, when it uses it less often than its line says (lower the count, or delete the line at 0), and when the baseline
# lists a file or a count its version at the base did not. The baseline only shrinks.
#
# Exit status: 0 when the uses match the baseline, 1 otherwise, 2 on a usage error.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
base="origin/redesign-web"
while [ "$#" -gt 0 ]; do
  case "$1" in
    --base) base="${2:-}" && shift ;;
    --root) root="${2:-}" && shift ;;
    *) sed -n '7,9p' "$0" | sed 's/^# \{0,1\}//' >&2 && exit 2 ;;
  esac
  shift
done
list="scripts/go-test-sleep.baseline"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# "<path> <count>" of every Go test that uses time.Sleep now, and of the baseline now and at the base.
git -C "$root" ls-files -co --exclude-standard -- '*_test.go' |
  while IFS= read -r f; do
    if [ -f "$root/$f" ]; then
      n="$(grep -cE '(^|[^A-Za-z0-9_])time\.Sleep([^A-Za-z0-9_]|$)' "$root/$f" || true)"
      if [ "$n" -gt 0 ]; then printf '%s %s\n' "$f" "$n"; fi
    fi
  done > "$work/now"
entries() { grep -vE '^[[:space:]]*(#|$)' || true; }
entries < "$root/$list" > "$work/listed"
: > "$work/before"
if git -C "$root" rev-parse --verify --quiet "$base^{commit}" > /dev/null; then
  mb="$(git -C "$root" merge-base "$base" HEAD 2> /dev/null || echo "$base")"
  if git -C "$root" cat-file -e "$mb:$list" 2> /dev/null; then
    git -C "$root" show "$mb:$list" | entries > "$work/before"
    echo "has-base" > "$work/has-base"
  fi
fi

status=0
awk -v hasbase="$([ -f "$work/has-base" ] && echo 1 || echo 0)" '
  FILENAME == ARGV[1] { before[$1] = $2; next }
  FILENAME == ARGV[2] {
    listed[$1] = $2
    if (hasbase && (!($1 in before) || $2 + 0 > before[$1] + 0))
      printf "%s: the baseline lists %d uses, more than the %d of its base: the baseline only shrinks; use testing/synctest instead\n", $1, $2, before[$1]
    next
  }
  {
    now[$1] = $2
    if (!($1 in listed))
      printf "%s: %d uses of time.Sleep in a test: wait with testing/synctest (synctest.Test, synctest.Wait), a channel, or a fake clock\n", $1, $2
    else if ($2 + 0 > listed[$1] + 0)
      printf "%s: %d uses of time.Sleep, more than the %d of the baseline: wait with testing/synctest (synctest.Test, synctest.Wait), a channel, or a fake clock\n", $1, $2, listed[$1]
  }
  END {
    for (f in listed)
      if (!(f in now) || now[f] + 0 < listed[f] + 0)
        printf "%s: %d uses of time.Sleep, fewer than the %d of the baseline: lower its count in scripts/go-test-sleep.baseline, or delete its line at 0\n", f, now[f] + 0, listed[f]
  }
' "$work/before" "$work/listed" "$work/now" > "$work/report"
if [ -s "$work/report" ]; then
  sort "$work/report" >&2
  status=1
fi
exit "$status"
