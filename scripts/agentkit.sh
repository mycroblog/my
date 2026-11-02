#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Runs agentkit (tools/agentkit), the context and workflow tools for coding agents, building bin/agentkit
# first when it is missing or older than its sources. `scripts/agentkit.sh help` lists the commands.
set -euo pipefail

# go.work exists for Dependabot alone (scripts/task.sh).
export GOWORK=off

root="$(cd "$(dirname "$0")/.." && pwd)"
bin="$root/bin/agentkit"
src="$root/tools/agentkit"

if [ ! -x "$bin" ] || [ "$src/go.mod" -nt "$bin" ] || [ "$src/go.sum" -nt "$bin" ] ||
  [ -n "$(find "$src" -name '*.go' -newer "$bin" -print -quit)" ]; then
  (cd "$src" && CGO_ENABLED=0 go build -o "$bin" ./cmd/agentkit) >&2
fi
exec "$bin" "$@"
