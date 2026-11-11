#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/docs-growth.sh on a repository it makes in a temporary directory.
#
#   scripts/docs-growth-test.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/scripts/docs-growth.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

failed=0
repo="$tmp/repo"
g() { git -C "$repo" -c user.name=test -c user.email=test@example.com -c commit.gpgsign=false "$@"; }

# bytes N prints N bytes.
bytes() { head -c "$1" /dev/zero | tr '\0' 'x'; }

# reset puts the repository back at its base: one file of 1,000 bytes in each group, and a guide outside them.
files=(docs/adr/0001-a.md docs/architecture/a.md docs/requirements/a.md docs/agents/a.md tools/agentkit/docs/a.md
  tools/agentkit/context-map.yaml AGENTS.md .claude/skills/a/SKILL.md docs/guide/a.md)
reset() {
  rm -rf "$repo"
  mkdir -p "$repo"
  g init -q -b main
  local f
  for f in "${files[@]}"; do
    mkdir -p "$repo/$(dirname "$f")"
    bytes 1000 > "$repo/$f"
  done
  g add -A
  g commit -q -m base
}

# expect NAME STATUS PATTERN [OPTION...] passes when the script exits with STATUS and prints a line matching PATTERN.
expect() {
  local name="$1" want="$2" pattern="$3" got=0
  shift 3
  "$script" --root "$repo" --base main "$@" > "$tmp/out" 2>&1 || got=$?
  if [ "$got" != "$want" ] || ! grep -Eq -- "$pattern" "$tmp/out"; then
    echo "FAIL: $name: want status $want and /$pattern/, got status $got and:" >&2
    sed 's/^/  /' "$tmp/out" >&2
    failed=1
  fi
}

reset
expect "no change" 0 '^docs/adr +1000 +1000 +\+0 '

bytes 1256 > "$repo/docs/adr/0001-a.md"
expect "a fix within the allowance" 0 '^docs/adr +1000 +1256 +\+256 +256$'

bytes 1257 > "$repo/docs/adr/0001-a.md"
expect "one byte over the allowance" 1 '^docs/adr .* <- grew$'
expect "the file that grew" 1 '^    \+257  docs/adr/0001-a\.md$'
expect "the fix" 1 '^docs:growth: the prose grew\. Shorten what you touched'
expect "the label" 1 'ask for the docs:grow label'
expect "growth the owner allowed" 0 'growth allowed' --allow-growth

reset
bytes 300 > "$repo/docs/adr/0002-b.md"
expect "a new ADR, untracked" 1 '^    \+300  docs/adr/0002-b\.md \(new\)$'
g rm -q docs/adr/0001-a.md
expect "a new ADR that replaces a longer one" 0 '^docs/adr +1000 +300 +-700 '

reset
bytes 1500 > "$repo/docs/architecture/a.md"
bytes 400 > "$repo/docs/architecture/b.md"
expect "two files grow, with the label" 0 '^architecture +1000 +1900 +\+900 ' --allow-growth
bytes 100 > "$repo/docs/architecture/a.md"
expect "a new file that a shorter one pays for" 0 '^architecture +1000 +500 +-500 '

reset
bytes 1001 > "$repo/AGENTS.md"
expect "the instructions have no allowance" 1 '^instructions +1000 +1001 +\+1 +0  <- grew$'

reset
mkdir -p "$repo/internal/app/x" "$repo/.claude/skills/b"
bytes 10 > "$repo/internal/app/x/CLAUDE.md"
expect "a CLAUDE.md in any directory" 1 '^    \+10  internal/app/x/CLAUDE\.md \(new\)$'
rm "$repo/internal/app/x/CLAUDE.md"
bytes 600 > "$repo/internal/app/x/context-map.yaml"
expect "a context-map.yaml in any directory" 1 '^context map +1000 +1600 '
rm "$repo/internal/app/x/context-map.yaml"
bytes 1 > "$repo/.claude/skills/b/SKILL.md"
expect "a skill" 1 '^\.claude +1000 +1001 '

reset
bytes 90000 > "$repo/docs/guide/a.md"
bytes 90000 > "$repo/README.md"
bytes 90000 > "$repo/.claude/settings.json"
expect "the documents for users and operators are in no group" 0 '^docs/adr +1000 +1000 '

# A branch behind its base: what main gained since the merge base is not the branch's to pay for, nor its credit.
reset
g checkout -q -b topic
g checkout -q main
bytes 5000 > "$repo/docs/adr/0001-a.md"
g commit -q -am "main grows"
g checkout -q topic
expect "the merge base, not the tip of the base" 0 '^docs/adr +1000 +1000 '

expect "an unknown base" 2 'is not a commit here' --base no-such-ref
expect "an unknown option" 2 '^usage|--base REF' --frobnicate

if [ "$failed" != 0 ]; then
  exit 1
fi
echo "docs-growth-test.sh: ok"
