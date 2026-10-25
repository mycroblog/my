#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Fails when the go.mod of pkg/mykit requires or replaces the server module github.com/mycroblog/my (ADR-0093 §1, §8):
# the library is a module of its own so that its users' dependency graph never takes in the server's. go.mod lists
# the indirect requirements too, so a dependency that pulls the server in fails as well. It also fails when the root
# go.mod (the server module) names pkg/mykit or its test module pkg/mykit/integration, which only that test module joins
# with the server. It reads the go.mod files only, offline.
#
#   scripts/mykit-mod-check.sh [GO_MOD [ROOT_GO_MOD]]   (default: pkg/mykit/go.mod and go.mod)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
gomod="${1:-$root/pkg/mykit/go.mod}"
rootmod="${2:-$root/go.mod}"
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
# The server module must not take in the library (or its test module) either: go.mod names only the modules it requires.
while IFS= read -r p; do
  case "$p" in
    "$server"/pkg/mykit | "$server"/pkg/mykit/*)
      echo "$rootmod names $p: the server module must not require pkg/mykit; only pkg/mykit/integration joins them (ADR-0093 §1)" >&2
      failed=1
      ;;
  esac
done < <(go mod edit -json "$rootmod" | grep -oE '"Path": "[^"]+"' | sed 's/"Path": "\(.*\)"/\1/' | sort -u)
[ "$failed" -eq 0 ] || exit 1
echo "$module does not depend on $server, which does not require it"
