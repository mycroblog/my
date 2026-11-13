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
{"options":"swatch-color","variants":["cool","warm"],"pick":null,"reason":"retune warm","context":null,"evaluator":"ship warm","from":"n1","shots":".tmp/visual/swatch-color","date":"2026-11-12"}
EOF
got=$(VISUAL_PICKS="$tmp/note-only.jsonl" VISUAL_MAIN="$tmp" "$script" scenes)
check "a note is no pick" "no picks to replay" "${got%%:*}"

cat >"$tmp/picks.jsonl" <<'EOF'
{"options":"swatch-shape","variants":["pill","round","square"],"pick":"square","reason":"round read as a button","context":null,"evaluator":"fix round","from":"b1.2","shots":".tmp/visual/swatch-shape","date":"2026-11-12"}
{"options":"swatch-color","variants":["cool","warm"],"pick":null,"reason":"retune warm","context":null,"evaluator":"ship warm","from":"n1","shots":".tmp/visual/swatch-color","date":"2026-11-12"}
{"options":"type-scale","variants":["loose","tight"],"pick":"tight","reason":null,"context":"loose split the specimen across the fold","evaluator":"ship tight","from":"b2.1","shots":".tmp/visual/type-scale","date":"2026-11-13"}
{"options":"type-weight","variants":["bold","light"],"pick":"bold","reason":null,"context":null,"evaluator":"ship light","from":"b2.2","shots":".tmp/visual/type-weight","date":"2026-11-13"}
EOF
mkdir -p "$tmp/.tmp/visual/swatch-shape"
export VISUAL_PICKS="$tmp/picks.jsonl" VISUAL_MAIN="$tmp"

missing="(missing: take them again from the commit that held the options)"
check "scenes" "swatch-shape: pill round square; shots: $tmp/.tmp/visual/swatch-shape
type-scale: loose tight; shots: $tmp/.tmp/visual/type-scale $missing
type-weight: bold light; shots: $tmp/.tmp/visual/type-weight $missing" "$("$script" scenes)"

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
