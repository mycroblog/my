#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/role-hook.sh: which tool calls each UI role may make.
set -euo pipefail

hook="$(dirname "$0")/role-hook.sh"
fail=0

expect() {
  local want=$1 role=$2 call=$3 got=0
  "$hook" "$role" <<<"$call" 2>/dev/null || got=$?
  if [[ $got != "$want" ]]; then
    echo "FAIL: role-hook.sh $role, exit $got, want $want: $call" >&2
    fail=1
  fi
}

expect 0 builder '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/knowledge/visual/index.md"}}'
expect 2 builder '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/knowledge/visual-evaluator/index.md"}}'
expect 2 builder '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/agents/visual-evaluator.md"}}'
expect 2 builder '{"tool_name":"Bash","tool_input":{"command":"scripts/agentkit.sh show .claude/knowledge/visual-evaluator/decisions.md"}}'
expect 0 visual-evaluator '{"tool_name":"Read","tool_input":{"file_path":"/r/.tmp/visual/page/page-mobile.png"}}'
expect 0 visual-evaluator '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/knowledge/visual-evaluator/decisions.md"}}'
expect 0 visual-evaluator '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/knowledge/visual/decisions.md"}}'
expect 2 visual-evaluator '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/knowledge/visual/exemplars.md"}}'
expect 2 visual-evaluator '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/picks/visual.jsonl"}}'
expect 2 visual-evaluator '{"tool_name":"Read","tool_input":{"file_path":"/r/web/catalog/src/Catalog.tsx"}}'
expect 2 visual-evaluator '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/knowledge/visual-evaluator/../../agents/visual.md"}}'
expect 2 visual-evaluator '{"tool_name":"Bash","tool_input":{"command":"cat web/catalog/src/Catalog.tsx"}}'
expect 2 nobody '{}'

exit "$fail"
