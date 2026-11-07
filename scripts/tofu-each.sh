#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Runs one tofu command in one module and prints its output in one piece, so that modules run in parallel
# (tasks/tofu.yml, xargs -P) do not interleave their lines. It fails as the command fails.
#
#   scripts/tofu-each.sh DIR COMMAND [ARG...]
set -uo pipefail

[ "$#" -ge 2 ] || {
  echo "usage: scripts/tofu-each.sh DIR COMMAND [ARG...]" >&2
  exit 2
}
dir="$1"
shift
out="$(tofu -chdir="$dir" "$@" -no-color 2>&1)"
status=$?
if [ "$status" -eq 0 ]; then
  echo "ok   $dir: tofu $*"
else
  printf 'FAIL %s: tofu %s\n%s\n' "$dir" "$*" "$out" >&2
fi
exit "$status"
