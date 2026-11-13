#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# The PreToolUse hook of the UI roles (ADR-0169 §2), which their agent definitions in .claude/agents/ name. It reads
# the tool call as JSON on stdin and refuses it with exit 2, the reason on stderr.
#
#   role-hook.sh builder            a role that builds never reads the visual evaluator's criteria: a call that names
#                                   them is refused
#   role-hook.sh visual-evaluator   the evaluator reads the rendered page, its own knowledge, and the visual role's
#                                   decisions, which are never findings: a Read of an image, of
#                                   .claude/knowledge/visual-evaluator/ or of an area file of the visual decisions,
#                                   .claude/knowledge/visual/decisions/<area>.md, and nothing else (never the log of
#                                   the decider's picks, .claude/picks/, nor the exemplars, the lessons of those picks);
#                                   besides, the SubagentHandback call that delivers its answer, which reads nothing
set -euo pipefail

input=$(cat)
case "${1:-}" in
builder)
  if [[ $input == *visual-evaluator* ]]; then
    echo "a role that builds does not read the visual evaluator's criteria (ADR-0169 §2): work from your own knowledge" >&2
    exit 2
  fi
  ;;
visual-evaluator)
  tool=$(jq -r '.tool_name // ""' <<<"$input")
  path=$(jq -r '.tool_input.file_path // ""' <<<"$input")
  if [[ $tool == SubagentHandback ]]; then
    exit 0
  fi
  if [[ $tool == Read && $path != *..* ]]; then
    case "$path" in
    *.png | *.jpg | *.jpeg | *.webp) exit 0 ;;
    */.claude/knowledge/visual-evaluator/*.md | .claude/knowledge/visual-evaluator/*.md) exit 0 ;;
    esac
    if [[ $path =~ (^|/)\.claude/knowledge/visual/decisions/[^/]+\.md$ ]]; then
      exit 0
    fi
  fi
  echo "the visual evaluator sees the rendered page, the task and the constraints only (ADR-0169 §2): read the shots you were given, .claude/knowledge/visual-evaluator/ and .claude/knowledge/visual/decisions/<area>.md, not ${path:-$tool}" >&2
  exit 2
  ;;
*)
  echo "usage: role-hook.sh builder|visual-evaluator < tool-call.json" >&2
  exit 2
  ;;
esac
