#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Fail when internal/interface/clientapi/features.txt, the record of every identifier Instance.features has issued,
# lost or reordered a line of the latest release from v0.1.0 on: an identifier a release carried is never removed or
# reused while /api/v1 lasts. A pre-release issues nothing, and before the first release there is nothing to compare
# with. A shallow clone may lack the tag, so it is fetched by name.
#
#   scripts/features-issued.sh         compare with the latest release tag on origin
#   scripts/features-issued.sh REF     compare with REF, which the local repository has
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

record=internal/interface/clientapi/features.txt

ref="${1:-}"
if [ -z "$ref" ]; then
  ref="$(git ls-remote --tags --refs origin 'refs/tags/v*' |
    sed -n 's|.*refs/tags/\(v[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\)$|\1|p' |
    grep -v '^v0\.0\.' | sort -V | tail -n 1 || true)"
  if [ -z "$ref" ]; then
    echo "no release tag from v0.1.0 on: no identifier of Instance.features is issued yet"
    exit 0
  fi
  if ! git rev-parse -q --verify "refs/tags/$ref" >/dev/null; then
    git fetch -q --depth=1 origin "refs/tags/$ref:refs/tags/$ref"
  fi
  ref="refs/tags/$ref"
fi

ids() { grep -v -e '^#' -e '^[[:space:]]*$' || true; }

if ! git cat-file -e "$ref:$record" 2>/dev/null; then
  echo "$ref has no $record: nothing issued to compare with"
  exit 0
fi
issued="$(git show "$ref:$record" | ids)"
now="$(ids <"$record")"
count="$(printf '%s\n' "$issued" | grep -c . || true)"
if [ "$(printf '%s\n' "$now" | head -n "$count")" != "$issued" ]; then
  echo "$record must start with the identifiers ${ref#refs/tags/} issued, in their order; append new ones after them:" >&2
  diff <(printf '%s\n' "$issued") <(printf '%s\n' "$now" | head -n "$count") >&2 || true
  exit 1
fi
echo "$record keeps the $count identifiers ${ref#refs/tags/} issued"
