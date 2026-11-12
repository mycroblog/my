#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Holds the prose that agents read and maintain to its kind (ADR-0000 §6): `scripts/task.sh docs:growth` runs it.
#
#   scripts/docs-growth.sh [options]
#     --base REF       compare with the merge base of REF and HEAD (default: origin/main)
#     --allow-growth   report the growth but do not fail on it
#     --root DIR       the repository (default: the one this script is in)
#
# Decisions and the instructions every session loads need the owner's approval to grow, the allow:docs-growth label
# on the pull request, as a destructive schema change needs allow:db-destructive (scripts/db-lint.sh). It adds up the
# bytes of each group at the base and in the working tree (untracked files included):
#
#   group            files                                     may grow by
#   docs/adr         docs/adr/**                               256 bytes
#   architecture     docs/architecture/**                      256
#   agentkit docs    tools/agentkit/docs/**                    256
#   context map      context-map.yaml, in any directory        256
#   instructions     AGENTS.md and CLAUDE.md, in any directory 0
#
# 256 bytes is a sentence of Japanese or a corrected link, so a fix passes and a new paragraph does not.
#
# What is loaded only when needed, and the requirements, grow without approval within a budget, which the label does
# not lift:
#
#   budget           files                                     at most
#   .claude          each .claude/**/*.md                      4096 bytes per file
#   requirements     docs/requirements/**                      98304 bytes in all
#
# The documents for users, operators, developers and plugin authors are in neither: their growth informs the reader.
#
# Exit status: 0 when no group grew beyond its allowance (or with --allow-growth) and no budget is exceeded, 1
# otherwise, 2 on a usage error.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

usage() {
  sed -n '6,9p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 2
}

base="origin/main" allow=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    --base) base="${2:-}" && shift ;;
    --allow-growth) allow=1 ;;
    --root) root="${2:-}" && shift ;;
    *) usage ;;
  esac
  shift
done
if [ -z "$base" ] || [ ! -d "$root" ]; then usage; fi

if ! git -C "$root" rev-parse --verify --quiet "$base^{commit}" > /dev/null; then
  echo "docs:growth: $base is not a commit here; pass the base (scripts/task.sh docs:growth BASE=main)" >&2
  exit 2
fi
ref="$base"
# The merge base, so that prose merged into BASE after the branch started is not read as this branch deleting it. A
# shallow clone without the history compares with BASE itself.
base="$(git -C "$root" merge-base "$base" HEAD 2> /dev/null || echo "$base")"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# "bytes<TAB>path" of every file, at the base and now.
git -C "$root" -c core.quotePath=false ls-tree -r -l "$base" |
  awk -F '\t' '{ n = split($1, f, " "); if (f[n] != "-") printf "%s\t%s\n", f[n], $2 }' > "$work/before"
git -C "$root" -c core.quotePath=false ls-files -co --exclude-standard |
  grep -E '(^|/)(AGENTS\.md|CLAUDE\.md|context-map\.yaml)$|^(docs/(adr|architecture|requirements)|tools/agentkit/docs)/|^\.claude/.*\.md$' |
  while IFS= read -r f; do
    if [ -f "$root/$f" ]; then printf '%s\t%s\n' "$(wc -c < "$root/$f" | tr -d ' ')" "$f"; fi
  done > "$work/after"

echo "docs:growth: $ref (merge base $(git -C "$root" rev-parse --short "$base^{commit}")) -> the working tree"
status=0
awk -F '\t' '
  function group(p,    b) {
    b = p
    sub(/^.*\//, "", b)
    if (b == "AGENTS.md" || b == "CLAUDE.md") return "instructions"
    if (b == "context-map.yaml") return "context map"
    if (p ~ /^docs\/adr\//) return "docs/adr"
    if (p ~ /^docs\/architecture\//) return "architecture"
    if (p ~ /^tools\/agentkit\/docs\//) return "agentkit docs"
    return ""
  }
  BEGIN {
    n = split("docs/adr,architecture,agentkit docs,context map,instructions", order, ",")
    for (i = 1; i <= n; i++) allowed[order[i]] = 256
    allowed["instructions"] = 0
  }
  { g = group($2) }
  g == "" { next }
  NR == FNR { before[g] += $1; old[$2] = $1; next }
  {
    after[g] += $1
    new = !($2 in old)
    if ($1 > old[$2]) grew[g] = grew[g] sprintf("%d\t%s%s\n", $1 - old[$2], $2, new ? " (new)" : "")
  }
  END {
    printf "%-14s %9s %9s %8s  %s\n", "group", "before", "after", "delta", "may grow by"
    for (i = 1; i <= n; i++) {
      g = order[i]
      d = after[g] - before[g]
      over = d > allowed[g]
      printf "%-14s %9d %9d %+8d  %d%s\n", g, before[g], after[g], d, allowed[g], over ? "  <- grew" : ""
      if (!over) continue
      failed = 1
      cmd = "sort -t \"\t\" -k1,1nr -k2 | head -n 5"
      printf "%s", grew[g] | cmd
      close(cmd)
    }
    exit failed
  }
' "$work/before" "$work/after" > "$work/report" || status=1
# The files that grew most follow their group, indented.
sed -E $'s/^([0-9]+)\t(.*)$/    +\\1  \\2/' "$work/report"

# The budgets, which no label lifts.
over=0
awk -F '\t' '
  $2 ~ /^\.claude\/.*\.md$/ && $1 > 4096 { printf "budget .claude: %s is %d bytes, over 4096\n", $2, $1; over = 1 }
  $2 ~ /^docs\/requirements\// { req += $1 }
  END {
    if (req > 98304) { printf "budget requirements: %d bytes in all, over 98304\n", req; over = 1 }
    exit over
  }
' "$work/after" || over=1
if [ "$over" = 1 ]; then
  cat >&2 << 'EOF'
docs:growth: over a budget. Say it in fewer words, split a skill into what it always needs and a file it reads when
needed, or delete what the change makes obsolete. A budget changes only with the owner, in this script.
EOF
fi

if [ "$status" = 0 ] || [ "$allow" = 1 ]; then
  if [ "$status" = 1 ]; then
    echo "docs:growth: growth allowed (--allow-growth, the allow:docs-growth label)"
  fi
  exit "$over"
fi
cat >&2 << 'EOF'
docs:growth: the prose grew. Shorten what you touched, or delete what the change makes obsolete, until no group grows.
If the growth is a new decision the owner wants recorded, ask for the allow:docs-growth label on the pull request.
EOF
exit 1
