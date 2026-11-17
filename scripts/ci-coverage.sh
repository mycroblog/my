#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Fails when a check of `scripts/task.sh check` runs in no CI step (ADR-0079). The CI jobs are workflow steps that each
# run one task, so a check added to a `<context>:check` task must also get a step, or an entry in EXCLUDED below with
# the reason CI does not run it.
#
# The checks are the tasks with a command of their own that `check` reaches through tasks without one (the
# `<context>:check` groups); their own dependencies (installs, generators) are not checks. A task that calls an
# internal task, which no step can run, is a check itself (web:test). A step runs a task when it runs
# `scripts/task.sh <task>`, or, for an action that runs the same tool (golangci-lint-action), when it is named after
# the task.
#
# It reads the tasks from `--summary`, which leaves out what it cannot list (a loop over a `sh:` variable), so it also
# asks `--dry` what `check` would run and fails when that names a task the walk did not reach.
#
# Usage: scripts/ci-coverage.sh [workflows dir]   (default: .github/workflows)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
dir="${1:-$root/.github/workflows}"

# EXCLUDED: the checks of `check` that CI does not run, each "<task> <reason>" (as "web:foo needs a GPU runner").
EXCLUDED=()

declare -A seen=() checks=() below=() summaries=()

# summary TASK sets `summaries[TASK]` to the lines of `--summary`, or to `internal` for an internal task, which
# `--summary` refuses. NO_COLOR keeps the colour codes go-task adds in CI out of the lines.
summary() {
  local out
  [ -z "${summaries[$1]:-}" ] || return 0
  if out="$(NO_COLOR=1 "$root/scripts/task.sh" --summary "$1" 2>&1)"; then
    summaries[$1]="$out"
  elif [[ "$out" == *"is internal"* ]]; then
    summaries[$1]=internal
  else
    printf '%s\n' "$out" >&2
    exit 1
  fi
}

# read_task TASK sets `children` to the tasks TASK depends on and calls, and `own` to whether it has a command of its
# own or calls an internal task, which no step can run for it. `--summary` lists a task's dependencies and its
# commands, one per " - " line, a call of another task as " - Task: <name>".
read_task() {
  local section="" line child
  children=()
  own=0
  summary "$1"
  while IFS= read -r line; do
    case "$line" in
      dependencies:) section=deps ;;
      commands:) section=cmds ;;
      vars:) section="" ;;
      " - Task: "*) [ "$section" = cmds ] && children+=("${line# - Task: }") ;;
      " - "*)
        case "$section" in
          deps) children+=("${line# - }") ;;
          cmds) own=1 ;;
        esac
        ;;
    esac
  done <<< "${summaries[$1]}"
  for child in "${children[@]}"; do
    summary "$child"
    [ "${summaries[$child]}" != internal ] || own=1
  done
}

# under TASK adds everything TASK depends on and calls to `below`: what a check runs besides its own commands.
under() {
  local child children own
  read_task "$1"
  for child in "${children[@]}"; do
    [ -z "${below[$child]:-}" ] || continue
    below[$child]=1
    under "$child"
  done
}

# walk TASK adds the checks TASK reaches to `checks`.
walk() {
  local task="$1" child children own
  [ -z "${seen[$task]:-}" ] || return 0
  seen[$task]=1
  read_task "$task"
  if [ "$own" = 1 ]; then
    checks[$task]=1
    under "$task"
    return 0
  fi
  for child in "${children[@]}"; do
    walk "$child"
  done
}

walk check
[ "${#checks[@]}" -gt 0 ] || { echo "ci-coverage.sh: found no checks under \`check\`" >&2; exit 1; }

# What `check` would run: every task with a command, a loop's included. One the walk did not reach is a check, or a
# group of checks, that `--summary` does not list; an internal task is its caller's, which the walk counted.
status=0
while IFS= read -r task; do
  summary "$task"
  if [ -z "${seen[$task]:-}" ] && [ -z "${below[$task]:-}" ] && [ "${summaries[$task]}" != internal ]; then
    echo "error: \`scripts/task.sh --dry check\` runs $task, which scripts/ci-coverage.sh did not find under \`check\`: \`--summary\` lists no dependency that leads to it" >&2
    status=1
  fi
done < <("$root/scripts/task.sh" --dry check 2>&1 | sed -nE 's/^task: \[([^]]+)\] .*/\1/p' | sort -u)

# The tasks the steps run: `scripts/task.sh <task>` and the names of the steps.
mapfile -t ran < <(
  cat "$dir"/*.yml |
    grep -oE 'scripts/task\.sh [a-z0-9-]+:[a-z0-9:-]+|^ *-? *name: [a-z0-9-]+:[a-z0-9:-]+ *$' |
    sed -E 's/^scripts\/task\.sh //; s/^ *-? *name: //; s/ *$//' | sort -u
)
declare -A covered=()
for task in "${ran[@]}"; do
  covered[$task]=1
done

declare -A excluded=()
for entry in "${EXCLUDED[@]}"; do
  task="${entry%% *}"
  excluded[$task]=1
  if [ -z "${checks[$task]:-}" ]; then
    echo "error: EXCLUDED in scripts/ci-coverage.sh names $task, which is not a check of \`check\`" >&2
    status=1
  elif [ "$entry" = "$task" ]; then
    echo "error: EXCLUDED in scripts/ci-coverage.sh gives no reason for $task" >&2
    status=1
  fi
done

missing=0
for task in $(printf '%s\n' "${!checks[@]}" | sort); do
  if [ -z "${covered[$task]:-}" ] && [ -z "${excluded[$task]:-}" ]; then
    echo "error: $task is a check of \`scripts/task.sh check\` that no step in ${dir#"$root"/} runs" >&2
    missing=1
  fi
done
if [ "$missing" != 0 ]; then
  echo "Add a step that runs it (\`scripts/task.sh <task>\`) to a job, or list it in EXCLUDED with the reason (ADR-0079)." >&2
fi
if [ "$status" != 0 ] || [ "$missing" != 0 ]; then
  exit 1
fi
echo "every one of the ${#checks[@]} checks of \`check\` runs in a CI step"
