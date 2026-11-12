#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# The agreement check of the visual evaluator with the decider (ADR-0169 §7), run when the decider asks for it. It
# replays the decider's picks among live options, which the visual role records in .claude/knowledge/visual/exemplars.md
# as `- pick <options id>: <winner> over <other>, <other> — <why> — <from> — <date>`, with the shots that
# `scripts/task.sh web:shot -- --options <id>` left in .tmp/visual/<id>/ of the main checkout.
#
#   visual-agreement.sh scenes          each recorded pick with the outcome hidden: its id, its variants in name order,
#                                       and the directory of its shots
#   visual-agreement.sh score <id>=<variant>...
#                                       the share of the evaluator's picks that match the decider's, and each miss with
#                                       the reason the pick recorded
#
# The procedure: run scenes; give a fresh visual-evaluator each scene's shots and its options' names, never the
# outcome; then run score with its picks. A miss is a signal for the knowledge of either role (ADR-0169 §4).
set -euo pipefail

root=$(git rev-parse --show-toplevel)
main=$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")
exemplars=${VISUAL_EXEMPLARS:-$root/.claude/knowledge/visual/exemplars.md}
shots=${VISUAL_SHOTS:-$main/.tmp/visual}

# One line per recorded pick: id, winner, the others (", "-separated), why, by the fields of an exemplar line.
picks() {
  awk -F ' — ' '$1 ~ /^- pick [^: ]+: [^ ]+ over / {
    h = substr($1, 8); c = index(h, ": ")
    id = substr(h, 1, c - 1); rest = substr(h, c + 2); o = index(rest, " over ")
    printf "%s\t%s\t%s\t%s\n", id, substr(rest, 1, o - 1), substr(rest, o + 6), $2
  }' "$exemplars"
}

all=$(picks)
if [[ -z $all ]]; then
  echo "no picks to replay: ${exemplars#"$root"/} records none yet. The visual role records one line, - pick <id>: <winner> over <others> — ..., for each pick the decider makes among live options (ADR-0169 §9)."
  exit 0
fi

case "${1:-}" in
scenes)
  while IFS=$'\t' read -r id win others _; do
    variants=$(printf '%s\n' "$win" "${others//, /$'\n'}" | sort | paste -sd' ')
    dir="$shots/$id"
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
  while IFS=$'\t' read -r id win _ why; do
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
