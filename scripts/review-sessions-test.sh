#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/review-sessions.sh on transcripts and picks of a temporary directory.
set -euo pipefail

script="$(cd "$(dirname "$0")" && pwd)/review-sessions.sh"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fail=0

check() {
  local name=$1 want=$2 got=$3
  if [[ $got != "$want" ]]; then
    printf 'FAIL: %s\nwant:\n%s\ngot:\n%s\n' "$name" "$want" "$got" >&2
    fail=1
  fi
}

has() {
  local name=$1 want=$2 file=$3
  if ! grep -qF -- "$want" "$file"; then
    printf 'FAIL: %s: %s does not hold %s\n' "$name" "$file" "$want" >&2
    fail=1
  fi
}

hasnt() {
  local name=$1 want=$2 file=$3
  if grep -qF -- "$want" "$file"; then
    printf 'FAIL: %s: %s holds %s\n' "$name" "$file" "$want" >&2
    fail=1
  fi
}

main=$tmp/src/my
projects=$tmp/projects
mkdir -p "$main" "$projects"
slug=${main//[^A-Za-z0-9]/-}
export REVIEW_SESSIONS_MAIN=$main REVIEW_SESSIONS_PROJECTS=$projects REVIEW_SESSIONS_PICKS=$tmp/picks.jsonl

check "since: the acceptance of ADR-0169" "2026-11-12T03:16:42Z" "$("$script" since)"
check "nothing to read" "nothing to read since 2026-11-12T03:16:42Z: no session or pick after it" "$("$script" digest)"

# A session of the main checkout, one of a worktree with a subagent, and a project that is not this repository's.
mkdir -p "$projects/$slug" "$projects/$slug--claude-worktrees-x/s2/subagents" "$projects/-other"
cat >"$projects/$slug/s1.jsonl" <<'EOF'
{"type":"user","origin":{"kind":"human"},"timestamp":"2026-11-11T10:00:00.000Z","gitBranch":"main","message":{"content":"before the point"}}
{"type":"assistant","timestamp":"2026-11-13T10:00:00.000Z","message":{"content":[{"type":"text","text":"Option A keeps the header; option B drops it."}]}}
{"type":"user","origin":{"kind":"human"},"timestamp":"2026-11-13T10:01:00.000Z","gitBranch":"feat-1","message":{"content":"B, the nav already shows the place"}}
{"type":"user","origin":{"kind":"human"},"timestamp":"2026-11-13T10:02:00.000Z","message":{"content":"<command-name>/clear</command-name>\n<command-args></command-args>"}}
{"type":"user","origin":{"kind":"human"},"timestamp":"2026-11-13T10:03:00.000Z","message":{"content":"<command-message>goal</command-message>\n<command-name>/goal</command-name>\n<command-args>ship the menu today</command-args>"}}
{"type":"user","origin":{"kind":"human"},"timestamp":"2026-11-13T10:04:00.000Z","message":{"content":"<local-command-stdout>Set model</local-command-stdout>"}}
{"type":"user","origin":{"kind":"task-notification"},"timestamp":"2026-11-13T10:05:00.000Z","message":{"content":"a task finished"}}
{"type":"user","timestamp":"2026-11-13T10:06:00.000Z","message":{"content":"Another Claude session sent this"}}
{"type":"user","origin":{"kind":"human"},"isMeta":true,"timestamp":"2026-11-13T10:07:00.000Z","message":{"content":"meta"}}
{"type":"user","origin":{"kind":"human"},"timestamp":"2026-11-13T10:08:00.000Z","message":{"content":[{"type":"text","text":"no jokes in errors"}]}}
EOF
cat >"$projects/$slug--claude-worktrees-x/s2.jsonl" <<'EOF'
{"type":"assistant","timestamp":"2026-11-14T09:00:00.000Z","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"scripts/agentkit.sh feedback wait --timeout 0"}},{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"git status"}}]}}
{"type":"user","timestamp":"2026-11-14T09:05:00.000Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"batch 20261114-180500-ab12: 1. the gap under the title is too wide"},{"type":"tool_result","tool_use_id":"t2","content":"clean"}]}}
EOF
cat >"$projects/$slug--claude-worktrees-x/s2/subagents/agent-a1.jsonl" <<'EOF'
{"type":"assistant","timestamp":"2026-11-14T09:10:00.000Z","message":{"content":[{"type":"tool_use","id":"t3","name":"Bash","input":{"command":"agentkit feedback show 20261114-180500-ab12"}},{"type":"tool_use","id":"t4","name":"Bash","input":{"command":"agentkit feedback show 20261114-180500-ab12"}}]}}
{"type":"user","timestamp":"2026-11-14T09:11:00.000Z","message":{"content":[{"type":"tool_result","tool_use_id":"t3","content":[{"type":"text","text":"note 20261114-180500-ab12.1 on the story post"}]}]}}
{"type":"user","timestamp":"2026-11-14T09:12:00.000Z","message":{"content":[{"type":"tool_result","tool_use_id":"t4","content":[{"type":"text","text":"note 20261114-180500-ab12.1 on the story post"}]}]}}
{"type":"user","origin":{"kind":"human"},"timestamp":"2026-11-14T09:13:00.000Z","message":{"content":"the orchestrator's brief"}}
EOF
cp "$projects/$slug/s1.jsonl" "$projects/-other/s3.jsonl"
cat >"$tmp/picks.jsonl" <<'EOF'
{"options":"old","variants":["a","b"],"pick":"a","reason":null,"context":null,"evaluator":"ship a","from":"b0","shots":null,"date":"2026-11-11"}
{"options":"badge","variants":["dot","pill"],"pick":"dot","reason":"a pill reads as a button","context":null,"evaluator":"fix pill","from":"b1.1","shots":null,"date":"2026-11-13"}
{"options":"relayed","variants":["one","two"],"pick":"one","reason":null,"context":null,"evaluator":"relayed by the orchestrator","from":"b1.2","shots":null,"date":"2026-11-13"}
{"options":"rule","variants":["line","none"],"pick":null,"reason":"retune both","context":null,"evaluator":"ship line","from":"n2","shots":null,"date":"2026-11-14"}
EOF

got=$("$script" digest)
check "digest: counts" "since 2026-11-12T03:16:42Z until" "${got:0:32}"
check "digest: what it read" "2 sessions, 3 turns of the decider, 2 batches of notes, 3 picks" "$(head -1 <<<"$got" | sed 's/.*Z: //')"
check "digest: its files" "$main/.tmp/review-sessions/run/picks.md
$main/.tmp/review-sessions/run/s1.md
$main/.tmp/review-sessions/run/s2.md" "$(tail -n +2 <<<"$got")"

s1=$main/.tmp/review-sessions/run/s1.md
has "a turn of the decider, with its branch" "## 2026-11-13T10:01:00Z the decider (feat-1)" "$s1"
has "a turn of the decider" "B, the nav already shows the place" "$s1"
has "the agent's words before it" "> Option A keeps the header; option B drops it." "$s1"
has "a turn of text parts" "no jokes in errors" "$s1"
has "a slash command with arguments" "/goal ship the menu today" "$s1"
hasnt "a turn before the point" "before the point" "$s1"
hasnt "a slash command without arguments" "/clear" "$s1"
hasnt "what a command printed" "Set model" "$s1"
hasnt "a task notification" "a task finished" "$s1"
hasnt "another session's message" "Another Claude session" "$s1"
hasnt "a meta turn" "meta" "$s1"

s2=$main/.tmp/review-sessions/run/s2.md
has "notes that feedback wait printed" "the gap under the title is too wide" "$s2"
has "notes that a subagent's feedback show printed" "note 20261114-180500-ab12.1 on the story post" "$s2"
check "a batch printed twice is read once" "1" "$(grep -c 'on the story post' "$s2")"
hasnt "another command's output" "clean" "$s2"
hasnt "a subagent's user turn" "the orchestrator's brief" "$s2"

picks=$main/.tmp/review-sessions/run/picks.md
has "a pick the evaluator differs from" "- badge (2026-11-13, from b1.1): dot, pill; the decider: dot — a pill reads as a button; the evaluator: fix pill — differs" "$picks"
has "a note instead of a pick" "- rule (2026-11-14, from n2): line, none; the decider: a note, no pick — retune both; the evaluator: ship line" "$picks"
hasnt "a pick before the point" "- old" "$picks"
hasnt "an evaluator who names no variant does not differ" "relayed by the orchestrator — differs" "$picks"
hasnt "a note differs from no evaluator" "ship line — differs" "$picks"

# mark records the time of the digest; the next digest reads from it.
until=$(cat "$main/.tmp/review-sessions/run/until")
check "mark" "the next run reads from $until" "$("$script" mark)"
check "since: the last run's" "$until" "$("$script" since)"
touch -d 2026-11-13 "$projects/$slug/s1.jsonl" "$projects/$slug--claude-worktrees-x/s2.jsonl" \
  "$projects/$slug--claude-worktrees-x/s2/subagents/agent-a1.jsonl"
check "nothing new since the mark" "nothing to read since $until: no session or pick after it" \
  "$(REVIEW_SESSIONS_PICKS=$tmp/none.jsonl "$script" digest)"

if "$script" digest 2026-11-12 2>/dev/null; then
  echo "FAIL: digest took a point that is no time" >&2
  fail=1
fi
rm -rf "$main/.tmp/review-sessions/run"
if "$script" mark 2>/dev/null; then
  echo "FAIL: mark marked without a digest" >&2
  fail=1
fi

exit "$fail"
