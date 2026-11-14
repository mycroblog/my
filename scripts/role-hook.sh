#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# The PreToolUse hook of the roles (ADR-0169 §2, ADR-0170 §2), which their agent definitions in .claude/agents/ name.
# It reads the tool call as JSON on stdin and refuses it with exit 2, the reason on stderr.
#
#   role-hook.sh builder            a role that builds never reads the visual evaluator's criteria: a call that names
#                                   them is refused
#   role-hook.sh visual-evaluator   the evaluator reads the rendered page, its own knowledge, and the visual role's
#                                   decisions, which are never findings: a Read of an image, of
#                                   .claude/knowledge/visual-evaluator/ or of an area file of the visual decisions,
#                                   .claude/knowledge/visual/decisions/<area>.md, and nothing else (never the log of
#                                   the decider's picks, .claude/picks/, nor the exemplars, the lessons of those picks);
#                                   besides, the SubagentHandback call that delivers its answer, which reads nothing
#   role-hook.sh go-test-developer  a test role never reads the implementation (ADR-0170 §2): it reads and greps the
#   role-hook.sh ts-test-developer  open paths only, and edits the test paths and its own knowledge only.
#       test paths  go: *_test.go, testdata/, pkg/testkit/, internal/flowtest/, test/;
#                   ts: *.test.ts(x), *.spec.ts(x), and the test/, fixtures/ and e2e/ directories under web/ and pkg/
#       open paths  the test paths, its knowledge, docs/adr/, AGENTS.md, CLAUDE.md, the OpenAPI spec (api/spec/), the
#                   generated types of pkg/mykit (*.gen.go, *.gen.ts), and for ts the stories (*.stories.tsx)
#     Glob lists names only and is allowed. Bash runs only these commands, joined by && || ; | & or a newline, with no
#     command substitution and no redirection but to /dev/null or between streams: cd, ls, pwd, mkdir; head, tail, wc,
#     sort, uniq and grep given no file, as filters of a pipe; git status, add,
#     commit, rev-parse; gh issue view, list; go test, vet, doc (the exported API: not -u, not -src); gofmt; bun test,
#     run; bunx or bun --bun vitest, playwright; scripts/task.sh; scripts/agentkit.sh outline (the declarations, not
#     their bodies), check, grep --only-tests, show of open paths. Quoted strings and heredoc bodies are data. Any other
#     tool is refused, Agent included.
set -euo pipefail

input=$(cat)

# test_path reports whether the path is one the test role of the language writes.
test_path() {
  local lang=$1 p=$2
  [[ $p != *..* ]] || return 1
  case $lang in
  go) [[ $p =~ _test\.go$ || $p =~ (^|/)(testdata|pkg/testkit|internal/flowtest|test)(/|$) ]] ;;
  ts) [[ $p =~ \.(test|spec)\.tsx?$ || $p =~ (^|/)(web|pkg)/(.+/)?(test|fixtures|e2e)(/|$) ]] ;;
  *) return 1 ;;
  esac
}

# open_path reports whether the test role may read the path.
open_path() {
  local role=$1 lang=$2 p=$3
  [[ -n $p && $p != *..* ]] || return 1
  test_path "$lang" "$p" && return 0
  [[ $p =~ (^|/)\.claude/knowledge/$role/ || $p =~ (^|/)docs/adr/[^/]+$ || $p =~ (^|/)(AGENTS|CLAUDE)\.md$ ||
    $p =~ (^|/)api/spec/ || $p =~ (^|/)pkg/mykit/(.+/)?[^/]+\.gen\.(go|ts)$ ]] && return 0
  [[ $lang == ts && $p =~ \.stories\.tsx$ ]]
}

# command_ok reports whether one simple command, its operators gone, is one the test role may run.
command_ok() {
  local role=$1 lang=$2 a
  local -a w
  read -r -a w <<<"$3"
  while [[ ${w[0]:-} =~ ^[A-Za-z_][A-Za-z0-9_]*= ]]; do w=("${w[@]:1}"); done
  ((${#w[@]})) || return 0
  local c=${w[0]#./} sub=${w[1]:-}
  [[ $c == */scripts/* ]] && c=scripts/${c##*/scripts/}
  case $c in
  cd | ls | pwd | mkdir | gofmt | scripts/task.sh) return 0 ;;
  head | tail | wc | sort | uniq | grep)
    # A filter of the output before it: no file, only flags, counts and, for grep, the pattern.
    local pattern=0
    for a in "${w[@]:1}"; do
      [[ $a == -* || $a =~ ^[0-9]+$ ]] && continue
      [[ $c == grep && $pattern == 0 ]] && pattern=1 && continue
      return 1
    done
    return 0
    ;;
  git) [[ $sub =~ ^(status|add|commit|rev-parse)$ ]] ;;
  gh) [[ $sub == issue && ${w[2]:-} =~ ^(view|list)$ ]] ;;
  go)
    case $sub in
    test | vet) return 0 ;;
    doc) [[ " ${w[*]} " != *" -u "* && " ${w[*]} " != *" -src "* ]] ;;
    *) return 1 ;;
    esac
    ;;
  bun) [[ $sub =~ ^(test|run)$ || ($sub == --bun && ${w[2]:-} =~ ^(vitest|playwright)$) ]] ;;
  bunx) [[ $sub =~ ^(vitest|playwright)$ ]] ;;
  scripts/agentkit.sh)
    case $sub in
    outline | check) return 0 ;;
    grep) [[ " ${w[*]} " == *" --only-tests "* ]] ;;
    show)
      ((${#w[@]} > 2)) || return 1
      for a in "${w[@]:2}"; do
        [[ $a == -* ]] && continue
        open_path "$role" "$lang" "${a%%:*}" || return 1
      done
      return 0
      ;;
    *) return 1 ;;
    esac
    ;;
  *) return 1 ;;
  esac
}

# bash_ok reports whether every command of a Bash call is one the test role may run.
bash_ok() {
  local role=$1 lang=$2 cmd=$3 out="" term="" line seg op
  # A heredoc's body is data: drop it, and its operator.
  while IFS= read -r line; do
    if [[ -n $term ]]; then
      [[ ${line//[[:space:]]/} == "$term" ]] && term=""
      continue
    fi
    if [[ $line =~ \<\<-?[[:space:]]*[\'\"]?([A-Za-z_][A-Za-z0-9_]*)[\'\"]? ]]; then
      term=${BASH_REMATCH[1]}
      line=${line/"${BASH_REMATCH[0]}"/}
    fi
    out+=$line$'\n'
  done <<<"$cmd"
  cmd=$out
  while [[ $cmd =~ \'[^\']*\' ]]; do cmd=${cmd/"${BASH_REMATCH[0]}"/Q}; done
  [[ $cmd != *"\$("* && $cmd != *"\`"* ]] || return 1
  while [[ $cmd =~ \"([^\"\\]|\\.)*\" ]]; do cmd=${cmd/"${BASH_REMATCH[0]}"/Q}; done
  cmd=$(sed -E 's#[0-9]*>&[0-9]##g; s#[0-9&]*>>? *(/dev/null)##g' <<<"$cmd")
  [[ $cmd != *'>'* && $cmd != *'<'* ]] || return 1
  for op in '&&' '||' ';' '|' '&'; do cmd=${cmd//"$op"/$'\n'}; done
  while IFS= read -r seg; do
    command_ok "$role" "$lang" "$seg" || return 1
  done <<<"$cmd"
}

# test_role decides a call of the test role of a language (ADR-0170 §2).
test_role() {
  local role=$1 lang=$2 tool path
  tool=$(jq -r '.tool_name // ""' <<<"$input")
  path=$(jq -r '.tool_input.file_path // .tool_input.notebook_path // .tool_input.path // ""' <<<"$input")
  case $tool in
  Glob | SubagentHandback | SendMessage | ToolSearch | Skill | WebFetch | WebSearch) exit 0 ;;
  Read | Grep) open_path "$role" "$lang" "$path" && exit 0 ;;
  Edit | Write | NotebookEdit)
    if test_path "$lang" "$path" || [[ $path =~ (^|/)\.claude/knowledge/$role/ && $path != *..* ]]; then
      exit 0
    fi
    echo "a test role edits only the tests, their helpers and fixtures, and its knowledge (ADR-0170 §2), not $path: the developer of the language changes the implementation" >&2
    exit 2
    ;;
  Bash) bash_ok "$role" "$lang" "$(jq -r '.tool_input.command // ""' <<<"$input")" && exit 0 ;;
  esac
  echo "a test role does not read the implementation (ADR-0170 §2): write the test from the issue, the ADRs, the tests and their helpers, and the public interface (scripts/agentkit.sh outline, go doc, api/spec/, the generated types of pkg/mykit); scripts/role-hook.sh lists what it may run. Not: ${path:-$tool}" >&2
  exit 2
}

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
go-test-developer) test_role go-test-developer go ;;
ts-test-developer) test_role ts-test-developer ts ;;
*)
  echo "usage: role-hook.sh builder|visual-evaluator|go-test-developer|ts-test-developer < tool-call.json" >&2
  exit 2
  ;;
esac
