#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Fails when the go.mod of pkg/mykit requires or replaces the server module github.com/mycroblog/my (ADR-0093 §1, §8):
# the library is a module of its own so that its users' dependency graph never takes in the server's. go.mod lists
# the indirect requirements too, so a dependency that pulls the server in fails as well. It reads go.mod only, offline.
#
#   scripts/mykit-mod-check.sh [GO_MOD]   (default: pkg/mykit/go.mod)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
gomod="${1:-$root/pkg/mykit/go.mod}"
server=github.com/mycroblog/my

json="$(go mod edit -json "$gomod")"
module="$(printf '%s\n' "$json" | grep -m1 -oE '"Path": "[^"]+"' | sed 's/"Path": "\(.*\)"/\1/')"
# Every module path that go.mod names: the module itself, the requirements, and both sides of the replacements.
mapfile -t paths < <(printf '%s\n' "$json" | grep -oE '"Path": "[^"]+"' | sed 's/"Path": "\(.*\)"/\1/' | sort -u)

failed=0
# A replacement would reach code by another path (such as `=> ../..`); only the test module pkg/mykit/integration
# may have one.
if printf '%s\n' "$json" | grep -q '"Replace": \['; then
  echo "$gomod has a replace directive: the published module must not have one" >&2
  failed=1
fi
for p in "${paths[@]}"; do
  [ "$p" = "$module" ] && continue
  case "$p" in
    "$server" | "$server"/*)
      echo "$gomod names $p: pkg/mykit must not depend on the server module $server (ADR-0093 §1)" >&2
      failed=1
      ;;
  esac
done
[ "$failed" -eq 0 ] || exit 1
echo "$module does not depend on $server"
