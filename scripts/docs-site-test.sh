#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/docs-site.sh without an account or the network: how `resolve` turns app/docs/versions.json and the
# published releases into the versions the deploy builds.
#
#   scripts/docs-site-test.sh
set -euo pipefail

script="$(cd "$(dirname "$0")" && pwd)/docs-site.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

failed=0
fail() {
  echo "FAIL: $*" >&2
  failed=1
}

# expect NAME VERSIONS RELEASES WANT runs resolve on the versions list (the JSON of .versions) and the releases (one
# tag per line) and compares the compact output with WANT.
expect() {
  local name="$1" got
  printf '{"versions":%s}\n' "$2" >"$tmp/versions.json"
  printf '%b' "$3" >"$tmp/releases"
  if ! got="$("$script" resolve "$tmp/versions.json" "$tmp/releases" 2>/dev/null | jq -c .)"; then
    fail "$name: resolve failed"
    return
  fi
  [[ "$got" == "$4" ]] || fail "$name: got $got, want $4"
}

# refuse NAME VERSIONS RELEASES expects resolve to fail.
refuse() {
  printf '{"versions":%s}\n' "$2" >"$tmp/versions.json"
  printf '%b' "$3" >"$tmp/releases"
  if "$script" resolve "$tmp/versions.json" "$tmp/releases" >/dev/null 2>&1; then
    fail "$1: resolve passed"
  fi
}

expect "before the first release" '[]' '' '{"latest":null,"versions":[]}'
expect "prereleases do not count" '[{"minor":"v0.1","status":"supported"}]' 'v0.1.0-rc.1\nv0.1.0-rc.2\n' \
  '{"latest":null,"versions":[]}'
expect "the newest patch" '[{"minor":"v0.1","status":"supported"}]' 'v0.1.2\nv0.1.10\nv0.1.9\nv0.1.11-rc.1\n' \
  '{"latest":"v0.1","versions":[{"minor":"v0.1","tag":"v0.1.10","status":"supported"}]}'
expect "an ended version stays" \
  '[{"minor":"v0.2","status":"supported"},{"minor":"v0.1","status":"ended"}]' 'v0.1.3\nv0.2.0\nv0.1.4\n' \
  '{"latest":"v0.2","versions":[{"minor":"v0.2","tag":"v0.2.0","status":"supported"},{"minor":"v0.1","tag":"v0.1.4","status":"ended"}]}'
expect "a listed version before its release" \
  '[{"minor":"v0.2","status":"supported"},{"minor":"v0.1","status":"supported"}]' 'v0.1.4\n' \
  '{"latest":"v0.1","versions":[{"minor":"v0.1","tag":"v0.1.4","status":"supported"}]}'
expect "v0.1 is not v0.10" '[{"minor":"v0.1","status":"supported"},{"minor":"v0.10","status":"supported"}]' \
  'v0.10.0\nv0.1.5\n' \
  '{"latest":"v0.1","versions":[{"minor":"v0.1","tag":"v0.1.5","status":"supported"},{"minor":"v0.10","tag":"v0.10.0","status":"supported"}]}'
refuse "a release of an unlisted minor version" '[{"minor":"v0.1","status":"supported"}]' 'v0.1.0\nv0.2.0\n'
refuse "an unknown status" '[{"minor":"v0.1","status":"old"}]' 'v0.1.0\n'
refuse "not a minor version" '[{"minor":"v0.1.0","status":"supported"}]' 'v0.1.0\n'

if [[ "$failed" -ne 0 ]]; then
  exit 1
fi
echo "docs-site: all tests passed"
