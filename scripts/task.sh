#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Runs go-task (ADR-0081) on Taskfile.yml with the version pinned in tools/go.mod, building bin/task first when it is
# missing or was built from another tools/go.mod / tools/go.sum (bin/task.sum holds their checksum, which survives a
# checkout or a CI cache restore, unlike modification times). Never the `task` on PATH: Taskwarrior's command has the
# same name, and another go-task may be another version. `scripts/task.sh --list` lists the tasks.
#
# The exit code is the failed command's own (--exit-code), not go-task's 201. Locally, TASK_CONCURRENCY defaults to
# half the CPUs, since several agents run checks on one machine; set it to override. In CI (CI set) the concurrency
# is unlimited and the output is interleaved, with no `[task]` prefix: each workflow step runs one task and has its own
# log (ADR-0079), and the problem matchers of the setup actions find `file:line` only at the start of a line.
#
# Every task runs with GOWORK=off: go.work exists for Dependabot alone. Each module is checked as it stands alone, the
# way its users and the image build see it; workspace mode would also refuse `-modfile=tools/go.mod` and the modules
# that go.work leaves out.
set -euo pipefail

export GOWORK=off

root="$(cd "$(dirname "$0")/.." && pwd)"
bin="$root/bin/task"
sum="$(cat "$root/tools/go.mod" "$root/tools/go.sum" | sha256sum)"

if [ ! -x "$bin" ] || [ "$(cat "$bin.sum" 2>/dev/null)" != "$sum" ]; then
  (cd "$root/tools" && CGO_ENABLED=0 go build -o "$bin" github.com/go-task/task/v3/cmd/task) >&2
  printf '%s\n' "$sum" > "$bin.sum"
fi

if [ -z "${CI:-}" ] && [ -z "${TASK_CONCURRENCY:-}" ]; then
  cpus="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2)"
  TASK_CONCURRENCY=$((cpus / 2))
  [ "$TASK_CONCURRENCY" -ge 1 ] || TASK_CONCURRENCY=1
  export TASK_CONCURRENCY
fi

output=()
if [ -n "${CI:-}" ] && [ -z "${TASK_OUTPUT:-}" ]; then
  output=(--output interleaved)
fi

exec "$bin" --exit-code "${output[@]}" --dir "$root" "$@"
