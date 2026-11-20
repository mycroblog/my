#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# The signals of the review of sessions (ADR-0169 §4, §6), which the decider starts by hand with the review-sessions
# skill. It reads, since the point of the last run, the Claude Code transcripts of this repository (the projects
# directory of the main checkout, of its worktrees and of the checkouts beside it, whose names start with its own),
# and the picks of .claude/picks/visual.jsonl, and writes a digest of them to .tmp/review-sessions/run of the main
# checkout: one file per session, with each turn the decider typed and the agent's last words before it, and each
# batch of on-screen notes (ADR-0168) that `agentkit feedback wait` or `show` printed in the session or its
# subagents, the only place a batch outlives its checkout. The digest quotes the decider: it stays in .tmp, and
# what the review proposes paraphrases it and cites the session id and the date.
#
#   review-sessions.sh since            the point the next run reads from: the last run's, else the acceptance of
#                                       ADR-0169
#   review-sessions.sh digest [<since>] the digest since the point (or <since>, an ISO 8601 time in UTC); prints
#                                       its counts and files, or that there is nothing to read
#   review-sessions.sh mark             records the time the last digest was taken as the point of the next run
set -euo pipefail

root=$(git rev-parse --show-toplevel)
main=${REVIEW_SESSIONS_MAIN:-$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")}
projects=${REVIEW_SESSIONS_PROJECTS:-${CLAUDE_CONFIG_DIR:-$HOME/.claude}/projects}
picks=${REVIEW_SESSIONS_PICKS:-$root/.claude/picks/visual.jsonl}
state=$main/.tmp/review-sessions
run=$state/run
# ADR-0169 was accepted by 3fe6855cd.
accepted=2026-11-12T03:16:42Z

since() {
  if [[ -s $state/last ]]; then cat "$state/last"; else echo "$accepted"; fi
}

# The signals of one transcript after $since, one Markdown section each. A main transcript gives the decider's turns
# and the notes; a subagent's gives only the notes, its user turns being the orchestrator's.
signals() {
  jq -nr --arg since "$1" --argjson main "$2" '
    def text: if type == "string" then . elif type == "array" then map(select(.type == "text") | .text) | join("\n") else "" end;
    def at: ((.timestamp // "") | .[0:19]) + "Z";
    def clip($n): if length > $n then .[0:$n] + " […]" else . end;
    def tail($n): if length > $n then "[…] " + .[length - $n:] else . end;
    def quote: split("\n") | map("> " + .) | join("\n");
    # A slash command keeps its name and arguments; one without arguments, and what a command printed, say nothing.
    def typed:
      if test("^\\s*<(local-command|bash-)") or test("^\\[Request interrupted") then null
      elif test("<command-name>") then
        ((capture("<command-name>(?<n>[^<]*)</command-name>").n) as $n
         | ((capture("<command-args>(?<a>[\\s\\S]*?)</command-args>") // {a: ""}).a) as $a
         | if ($a | test("\\S")) then $n + " " + $a else null end)
      else . end;
    foreach inputs as $e ({cmds: {}, seen: {}, before: "", out: []};
      .out = []
      | if $e.type == "assistant" and ($e.message.content | type) == "array" then
          reduce $e.message.content[] as $b (.;
            if $b.type == "text" then .before = $b.text
            elif $b.type == "tool_use" and (($b.input.command // "") | test("agentkit(\\.sh)? feedback (wait|show)"))
            then .cmds[$b.id] = $b.input.command
            else . end)
        elif $e.type == "user" and ($e | at) > $since then
          if $main and ($e.origin.kind // "") == "human" and ($e.isMeta | not) then
            (($e.message.content | text) | typed) as $t
            | if $t == null or ($t | test("\\S") | not) then .
              else .out = ["## \($e | at) the decider (\($e.gitBranch // "-"))\n\n\($t | clip(4000))\n\nThe agent before it:\n\n\(.before | tail(800) | quote)\n"]
              end
          elif ($e.message.content | type) == "array" then
            # A batch printed again (wait, then show) is read once.
            reduce ($e.message.content[] | select(.type == "tool_result")) as $r (.;
              if .cmds[$r.tool_use_id] == null then .
              else (($r.content | text | clip(4000))) as $t
                | if ($t | test("\\S") | not) or .seen[$t] then .
                  else .seen[$t] = true
                    | .out += ["## \($e | at) notes (\(.cmds[$r.tool_use_id] | split("\n")[0] | clip(160)))\n\n```text\n\($t)\n```\n"]
                  end
              end)
          else . end
        else . end;
      .out[])' "$3"
}

digest() {
  local since=$1 until slug n=0 turns=0 notes=0 files=() f id out
  until=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  rm -rf "$run"
  mkdir -p "$run"
  echo "$until" >"$run/until"
  slug=${main//[^A-Za-z0-9]/-}
  # Every transcript written to after the point, grouped by the session it belongs to.
  declare -A sessions=()
  if [[ -d $projects ]]; then
    while IFS= read -r f; do
      if [[ $f == */subagents/* ]]; then id=$(basename "$(dirname "$(dirname "$f")")"); else id=$(basename "$f" .jsonl); fi
      sessions[$id]+="$f"$'\n'
    done < <(find "$projects" -path "$projects/$slug*" -name '*.jsonl' -newermt "$since" 2>/dev/null | sort)
  fi
  for id in "${!sessions[@]}"; do
    out=""
    while IFS= read -r f; do
      [[ -n $f ]] || continue
      if [[ $f == */subagents/* ]]; then out+=$(signals "$since" false "$f"); else out+=$(signals "$since" true "$f"); fi
      out+=$'\n'
    done <<<"${sessions[$id]}"
    [[ $out =~ [^[:space:]] ]] || continue
    n=$((n + 1))
    turns=$((turns + $(grep -c '^## .* the decider (' <<<"$out" || true)))
    notes=$((notes + $(grep -c '^## .* notes (' <<<"$out" || true)))
    printf '# Session %s\n\n%s\n' "$id" "$out" >"$run/$id.md"
    files+=("$run/$id.md")
  done
  local p=0
  if [[ -f $picks ]]; then
    jq -r --arg day "${since:0:10}" 'select((.date // "") >= $day)
      | ((.evaluator // "") | split(" ")[1] // "") as $ev
      | "- \(.options) (\(.date), from \(.from // "-")): \(.variants | join(", ")); the decider: \(.pick // "a note, no pick")\(if .reason then " — " + .reason else "" end); the evaluator: \(.evaluator // "not run")\(if .pick != null and $ev != .pick and (.variants | index($ev)) != null then " — differs" else "" end)"' \
      "$picks" >"$run/picks.md"
    p=$(wc -l <"$run/picks.md" | tr -d ' ')
    if ((p == 0)); then rm "$run/picks.md"; else files+=("$run/picks.md"); fi
  fi
  if ((n == 0 && p == 0)); then
    echo "nothing to read since $since: no session or pick after it"
    return 0
  fi
  echo "since $since until $until: $n sessions, $turns turns of the decider, $notes batches of notes, $p picks"
  printf '%s\n' "${files[@]}" | sort
}

case "${1:-}" in
since) since ;;
digest)
  s=${2:-$(since)}
  if ! [[ $s =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]]; then
    echo "review-sessions: $s is no time: write it as 2026-11-12T03:16:42Z (UTC)" >&2
    exit 1
  fi
  digest "$s"
  ;;
mark)
  if [[ ! -s $run/until ]]; then
    echo "review-sessions: no digest to mark: run digest first" >&2
    exit 1
  fi
  cp "$run/until" "$state/last"
  echo "the next run reads from $(cat "$state/last")"
  ;;
*)
  echo "usage: review-sessions.sh since | digest [<since>] | mark" >&2
  exit 2
  ;;
esac
