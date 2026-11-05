#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Fails when a file names a MY_ variable that the configuration does not have, so that a setting renamed in the schema
# cannot leave a deployment setting a variable the server ignores. docs/reference/configuration.md, which a Go test
# keeps equal to the schema, lists each variable and, for a secret setting, its _FILE form.
#
# Usage: scripts/tofu-env-check.sh [-r REFERENCE] PATH...
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
reference="$root/docs/reference/configuration.md"
if [ "${1:-}" = "-r" ]; then
  reference="$2"
  shift 2
fi
[ "$#" -gt 0 ] || {
  echo "usage: scripts/tofu-env-check.sh [-r REFERENCE] PATH..." >&2
  exit 2
}

known="$(mktemp)"
trap 'rm -f "$known"' EXIT
{
  # MY_CONFIG names the config file, which is no setting.
  echo MY_CONFIG
  grep -E '^- Environment variable:' "$reference" | grep -oE '\bMY_[A-Z0-9_]+\b'
} | sort -u > "$known"

[ "$(wc -l < "$known")" -gt 1 ] || {
  echo "tofu-env-check: no variables found in $reference" >&2
  exit 1
}

status=0
while IFS= read -r line; do
  file="${line%%:*}"
  name="${line#*:}"
  if ! grep -qxF "$name" "$known"; then
    echo "$file: $name is not a variable of the configuration ($reference)" >&2
    status=1
  fi
done < <(grep -roE --exclude-dir=.terraform '\bMY_[A-Z0-9_]+\b' "$@" | sort -u)
exit "$status"
