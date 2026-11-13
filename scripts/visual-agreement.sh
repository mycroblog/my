#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# The agreement check of the visual evaluator with the decider (ADR-0169 §7), run when the decider asks for it. It
# replays the decider's picks among live options, which the visual role records in .claude/picks/visual.jsonl, one
# JSON object per pick (`agentkit docs lint` holds its shape), with the shots that
# `scripts/task.sh web:shot -- --options <id>` left in the record's shots folder of the main checkout. A record without
# a pick (the decider sent a note instead) is not replayed.
#
#   visual-agreement.sh scenes          each recorded pick with the outcome hidden: its id, its variants in name order,
#                                       and the directory of its shots
#   visual-agreement.sh score <id>=<variant>...
#                                       the share of the evaluator's picks that match the decider's, and each miss with
#                                       the reason the record gives
#
# The procedure: run scenes; give a fresh visual-evaluator each scene's shots and its options' names, never the
# outcome; then run score with its picks. A miss is a signal for the knowledge of either role (ADR-0169 §4).
set -euo pipefail

root=$(git rev-parse --show-toplevel)
main=$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")
log=${VISUAL_PICKS:-$root/.claude/picks/visual.jsonl}
main=${VISUAL_MAIN:-$main}

# One line per replayable pick: id, pick, the variants in name order, why, the shots folder.
picks() {
  [[ -f $log ]] || return 0
  jq -r 'select(.pick != null)
    | [.options, .pick, (.variants | sort | join(" ")), (.reason // .context // "no reason given"), .shots] | @tsv' "$log"
}

all=$(picks)
if [[ -z $all ]]; then
  echo "no picks to replay: ${log#"$root"/} records none yet. The visual role records one line for each pick the decider makes among live options (ADR-0169 §9)."
  exit 0
fi

case "${1:-}" in
scenes)
  while IFS=$'\t' read -r id _ variants _ shots; do
    dir="$main/$shots"
    [[ -d $dir ]] || dir="$dir (missing: take them again from the commit that held the options)"
    echo "$id: $variants; shots: $dir"
  done <<<"$all"
  ;;
score)
  shift
  declare -A answer=()
  for a in "$@"; do
    id=${a%%=*}
    if [[ $a != *=* ]] || ! cut -f1 <<<"$all" | grep -qxF "$id"; then
      echo "visual-agreement: $a is not <id>=<variant> of a recorded pick (scenes lists them)" >&2
      exit 1
    fi
    answer[$id]=${a#*=}
  done
  n=0 hits=0 misses="" unanswered=""
  while IFS=$'\t' read -r id win _ why _; do
    if [[ -z ${answer[$id]:-} ]]; then
      unanswered+=" $id"
      continue
    fi
    n=$((n + 1))
    if [[ ${answer[$id]} == "$win" ]]; then
      hits=$((hits + 1))
    else
      misses+="- $id: the evaluator picked ${answer[$id]}, the decider $win — $why"$'\n'
    fi
  done <<<"$all"
  if ((n == 0)); then
    echo "visual-agreement: no pick answered (scenes lists them)" >&2
    exit 1
  fi
  echo "agreement: $hits of $n ($((hits * 100 / n))%)"
  [[ -z $misses ]] || printf 'misses:\n%s' "$misses"
  [[ -z $unanswered ]] || echo "not answered:$unanswered"
  ;;
*)
  echo "usage: visual-agreement.sh scenes | score <id>=<variant>..." >&2
  exit 2
  ;;
esac
