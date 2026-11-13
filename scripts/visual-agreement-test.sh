#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/visual-agreement.sh on recorded picks of a temporary log.
set -euo pipefail

script="$(cd "$(dirname "$0")" && pwd)/visual-agreement.sh"
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

got=$(VISUAL_PICKS="$tmp/none.jsonl" VISUAL_MAIN="$tmp" "$script" scenes)
check "no log" "no picks to replay" "${got%%:*}"

cat >"$tmp/note-only.jsonl" <<'EOF'
{"options":"swatch-color","variants":["cool","warm"],"pick":null,"reason":"retune warm","context":null,"evaluator":"ship warm","from":"n1","shots":".tmp/picks/visual/swatch-color","date":"2026-11-12"}
{"options":"swatch-lost","variants":["cool","warm"],"pick":"warm","reason":null,"context":null,"evaluator":"ship warm","from":"n2","shots":null,"date":"2026-11-12"}
EOF
got=$(VISUAL_PICKS="$tmp/note-only.jsonl" VISUAL_MAIN="$tmp" "$script" scenes)
check "a note, or a pick without shots, is no pick to replay" "no picks to replay" "${got%%:*}"

# keep copies the shots aside and points the record there; it keeps no pick twice, and none without shots.
keep=$tmp/keep
mkdir -p "$keep/.tmp/visual/badge"
echo shot >"$keep/.tmp/visual/badge/dot-mobile-light.png"
rec='{"options":"badge","variants":["dot","pill"],"pick":"dot","reason":null,"context":null,"evaluator":"ship dot","from":"b9","shots":".tmp/visual/badge","date":"2026-11-14"}'
check "keep" "kept $keep/.tmp/picks/visual/badge" \
  "$(VISUAL_PICKS="$keep/picks.jsonl" VISUAL_MAIN="$keep" "$script" keep <<<"$rec")"
check "keep: the record" "${rec/.tmp\/visual\/badge/.tmp/picks/visual/badge}" "$(cat "$keep/picks.jsonl")"
check "keep: the shots" "shot" "$(cat "$keep/.tmp/picks/visual/badge/dot-mobile-light.png")"
if VISUAL_PICKS="$keep/picks.jsonl" VISUAL_MAIN="$keep" "$script" keep <<<"$rec" 2>/dev/null; then
  echo "FAIL: keep kept a pick twice" >&2
  fail=1
fi
if VISUAL_PICKS="$keep/picks.jsonl" VISUAL_MAIN="$keep" "$script" keep <<<"${rec//badge/gone}" 2>/dev/null; then
  echo "FAIL: keep kept a pick without shots" >&2
  fail=1
fi
if VISUAL_PICKS="$keep/picks.jsonl" VISUAL_MAIN="$keep" "$script" keep <<<"${rec/\"badge\"/\"..\/x\"}" 2>/dev/null; then
  echo "FAIL: keep took an options id that is not a name" >&2
  fail=1
fi
check "keep: one record" "1" "$(wc -l <"$keep/picks.jsonl" | tr -d ' ')"

cat >"$tmp/picks.jsonl" <<'EOF'
{"options":"swatch-shape","variants":["pill","round","square"],"pick":"square","reason":"round read as a button","context":null,"evaluator":"fix round","from":"b1.2","shots":".tmp/picks/visual/swatch-shape","date":"2026-11-12"}
{"options":"swatch-color","variants":["cool","warm"],"pick":null,"reason":"retune warm","context":null,"evaluator":"ship warm","from":"n1","shots":".tmp/picks/visual/swatch-color","date":"2026-11-12"}
{"options":"type-scale","variants":["loose","tight"],"pick":"tight","reason":null,"context":"loose split the specimen across the fold","evaluator":"ship tight","from":"b2.1","shots":".tmp/picks/visual/type-scale","date":"2026-11-13"}
{"options":"type-weight","variants":["bold","light"],"pick":"bold","reason":null,"context":null,"evaluator":"ship light","from":"b2.2","shots":".tmp/picks/visual/type-weight","date":"2026-11-13"}
EOF
mkdir -p "$tmp/.tmp/picks/visual/swatch-shape"
export VISUAL_PICKS="$tmp/picks.jsonl" VISUAL_MAIN="$tmp"

missing="(missing: take them again from the commit that held the options)"
check "scenes" "swatch-shape: pill round square; shots: $tmp/.tmp/picks/visual/swatch-shape
type-scale: loose tight; shots: $tmp/.tmp/picks/visual/type-scale $missing
type-weight: bold light; shots: $tmp/.tmp/picks/visual/type-weight $missing" "$("$script" scenes)"

check "score" "agreement: 0 of 3 (0%)
misses:
- swatch-shape: the evaluator picked round, the decider square — round read as a button
- type-scale: the evaluator picked loose, the decider tight — loose split the specimen across the fold
- type-weight: the evaluator picked light, the decider bold — no reason given" \
  "$("$script" score swatch-shape=round type-scale=loose type-weight=light)"

check "score, unanswered" "agreement: 1 of 1 (100%)
not answered: swatch-shape type-weight" "$("$script" score type-scale=tight)"

if "$script" score nothing=round 2>/dev/null; then
  echo "FAIL: score accepted an unknown pick" >&2
  fail=1
fi
if "$script" score swatch-color=warm 2>/dev/null; then
  echo "FAIL: score accepted a record without a pick" >&2
  fail=1
fi

exit "$fail"
