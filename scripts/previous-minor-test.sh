#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/previous-minor.sh on lists of tags.
#
#   scripts/previous-minor-test.sh
set -euo pipefail

script="$(cd "$(dirname "$0")" && pwd)/previous-minor.sh"

failed=0
# expect NAME WANT TAGS CANDIDATE: TAGS is a space-separated list.
expect() {
  local got
  got="$(tr ' ' '\n' <<<"$3" | "$script" ${4:+"$4"})"
  [[ "$got" == "$2" ]] || {
    echo "FAIL: $1: want '$2', got '$got'" >&2
    failed=1
  }
}

tags="v0.0.0-test.1 pkg/mykit/v0.1.0 v0.1.0 v0.1.1 v0.1.10 v0.1.2 v0.2.0-rc.1 v0.2.0 v0.2.1 v0.3.0-rc.1 v1.0.0"
expect "no tags" "" "" v0.1.0
expect "only pre-releases" "" "v0.0.0-test.1 v0.0.0-test.2" v0.0.0-test.3
expect "the first minor version" "" "$tags" v0.1.0
expect "a patch of the first minor version" "" "$tags" v0.1.11
expect "the latest patch of the previous minor version" v0.1.10 "$tags" v0.2.0
expect "a patch release upgrades from the previous minor version too" v0.1.10 "$tags" v0.2.2
expect "a pre-release of a minor version" v0.2.1 "$tags" v0.3.0-rc.2
expect "a minor version skipped in the tags" v0.2.1 "$tags" v0.5.0
expect "a major version" v0.2.1 "$tags" v1.0.0-rc.1
expect "after a major version" v1.0.0 "$tags" v1.1.0
expect "the latest release without a candidate" v1.0.0 "$tags" ""
expect "library tags are ignored" "" "pkg/mykit/v0.1.0 pkg/mybot/v0.1.0" v0.2.0

if "$script" 0.2.0 </dev/null >/dev/null 2>&1; then
  echo "FAIL: a version without v: want an error" >&2
  failed=1
fi

if [[ "$failed" -ne 0 ]]; then
  exit 1
fi
echo "previous-minor-test: ok"
