#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Fails when the go.mod of a client library, pkg/mykit or pkg/mybot, requires or replaces the server module
# github.com/mycroblog/my (ADR-0093 §1, §8; ADR-0094 §1): the libraries are modules of their own so that their
# users' dependency graph never takes in the server's. go.mod lists the indirect requirements too, so a dependency
# that pulls the server in fails as well. The libraries' own modules (github.com/mycroblog/my/pkg/mykit and
# .../pkg/mybot) are not the server's: mybot requires mykit, and replaces it with its sibling directory so that both
# are built from the same commit; no other replacement is allowed. It reads go.mod only, offline.
#
#   scripts/mykit-mod-check.sh [GO_MOD]   (default: pkg/mykit/go.mod)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
gomod="${1:-$root/pkg/mykit/go.mod}"
server=github.com/mycroblog/my
libraries=("$server/pkg/mykit" "$server/pkg/mybot")

is_library() {
  local l
  for l in "${libraries[@]}"; do
    [ "$1" = "$l" ] && return 0
  done
  return 1
}

paths_of() { grep -oE '"Path": "[^"]+"' | sed 's/"Path": "\(.*\)"/\1/'; }

json="$(go mod edit -json "$gomod")"
module="$(printf '%s\n' "$json" | paths_of | head -n1)"
# Every module path that go.mod names: the module itself, the requirements, and both sides of the replacements.
mapfile -t paths < <(printf '%s\n' "$json" | paths_of | sort -u)
# The replacements, as old and new paths one after the other.
mapfile -t replaced < <(printf '%s\n' "$json" | sed -n '/"Replace": \[/,/^\t\]/p' | paths_of)

failed=0
# A replacement would reach code by another path (such as `=> ../..`); only a library's sibling library may be
# replaced, by its directory (such as `github.com/mycroblog/my/pkg/mykit => ../mykit`), and the test modules of
# pkg/*/integration, which this script does not check, may have others.
for ((i = 0; i + 1 < ${#replaced[@]}; i += 2)); do
  old="${replaced[i]}" new="${replaced[i + 1]}"
  if ! is_library "$old" || [ "$new" != "../${old##*/}" ]; then
    echo "$gomod replaces $old with $new: a published library may replace only a sibling library with its directory" >&2
    failed=1
  fi
done
for p in "${paths[@]}"; do
  [ "$p" = "$module" ] && continue
  is_library "$p" && continue
  case "$p" in
    "$server" | "$server"/*)
      echo "$gomod names $p: $module must not depend on the server module $server (ADR-0093 §1)" >&2
      failed=1
      ;;
  esac
done
[ "$failed" -eq 0 ] || exit 1
echo "$module does not depend on $server"
