#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/visual-agreement.sh on recorded picks of a temporary exemplars file.
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

cat >"$tmp/empty.md" <<'EOF'
# visual: exemplars

One line each: `- <the model screen, or the options that lost> — <why> — <from: the decider's choice> — <YYYY-MM-DD>`.

- the catalog's type specimen — one sample per role — the decider's choice — 2026-11-12
EOF
got=$(VISUAL_EXEMPLARS="$tmp/empty.md" VISUAL_SHOTS="$tmp/shots" "$script" scenes)
check "no picks" "no picks to replay" "${got%%:*}"

cat >"$tmp/picks.md" <<'EOF'
# visual: exemplars

One line each: `- <the model screen, or the options that lost> — <why> — <from: the decider's choice> — <YYYY-MM-DD>`.

- pick swatch-shape: square over round, pill — round read as a button — the decider's pick (b1.2); the evaluator: round — 2026-11-12
- pick type-scale: tight over loose — loose split the specimen across the fold — the decider's pick (b2.1); the evaluator: tight — 2026-11-13
EOF
mkdir -p "$tmp/shots/swatch-shape"
export VISUAL_EXEMPLARS="$tmp/picks.md" VISUAL_SHOTS="$tmp/shots"

check "scenes" "swatch-shape: pill round square; shots: $tmp/shots/swatch-shape
type-scale: loose tight; shots: $tmp/shots/type-scale (missing: take them again from the commit that held the options)" "$("$script" scenes)"

check "score" "agreement: 1 of 2 (50%)
misses:
- swatch-shape: the evaluator picked round, the decider square — round read as a button" \
  "$("$script" score swatch-shape=round type-scale=tight)"

check "score, one unanswered" "agreement: 1 of 1 (100%)
not answered: swatch-shape" "$("$script" score type-scale=tight)"

if "$script" score nothing=round 2>/dev/null; then
  echo "FAIL: score accepted an unknown pick" >&2
  fail=1
fi

exit "$fail"
