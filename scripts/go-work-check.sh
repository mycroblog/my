#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Fails when go.work, the workspace that Dependabot's Go updates run in, no longer loads or lists the wrong modules:
# it lists every Go module of the repository except the ones named below, which stay out on purpose (see go.work).
# Loading it fails when a module's go line is newer than the go line of go.work, which would stop Dependabot's updates
# and which no other check sees, since every check runs with GOWORK=off. It runs offline.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

outside='^(pkg/mykit|pkg/mybot|tools)/go\.mod$'
want="$(git ls-files '*go.mod' | grep -v -E "$outside" | sed -e 's|/\{0,1\}go\.mod$||' -e 's|^$|.|' | sort)"
have="$(GOWORK="$root/go.work" go list -m -f '{{.Dir}}' | sed -e "s|^$root/\{0,1\}||" -e 's|^$|.|' | sort)"

if [ "$want" != "$have" ]; then
  echo "go.work should list these modules:" >&2
  echo "$want" >&2
  echo "It lists:" >&2
  echo "$have" >&2
  exit 1
fi
