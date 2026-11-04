#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Creates a new ADR from docs/adr/template.md. Its number is one more than the largest number of the ADRs in the
# working tree, of the retired numbers in docs/adr/aliases.yaml, and of the ADR files the open pull requests of
# mycroblog/my add, so that parallel pull requests do not pick the same number and no number is used twice
# (ADR-0081). Without a working gh, it uses the working tree only and says so.
# Usage: scripts/adr-new.sh "Title of the decision" [kebab-slug]
set -euo pipefail

if [[ $# -lt 1 || -z "$1" ]]; then
  echo "usage: $0 \"Title\" [slug]" >&2
  exit 2
fi

root="$(cd "$(dirname "$0")/.." && pwd)"
dir="$root/docs/adr"
repo="mycroblog/my"
title="$1"
# Non-ASCII titles cannot be slugified reliably; pass an explicit slug for them.
slug="${2:-$(printf '%s' "$title" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+|-+$//g')}"
if [[ -z "$slug" ]]; then
  echo "error: could not derive a slug from the title; pass one explicitly" >&2
  exit 2
fi

# open_pr_adrs prints the ADR file names the open pull requests change, one per line; it fails when listing the pull
# requests fails. It reads each pull request's file list (up to 3,000 files) rather than its diff, which GitHub refuses
# for a large pull request, and warns about a pull request whose files it cannot read instead of ignoring them all.
open_pr_adrs() {
  local prs
  prs="$(gh pr list -R "$repo" --state open --limit 500 --json number --jq '.[].number')" || return 1
  [[ -z "$prs" ]] && return 0
  # One gh call per pull request, eight at a time. The script is single-quoted on purpose: sh -c expands $1 and $2.
  # shellcheck disable=SC2016
  printf '%s\n' "$prs" | xargs -P 8 -I{} sh -c \
    'gh api --paginate "repos/$1/pulls/$2/files" --jq ".[].filename" || echo "warning: could not read the files of pull request #$2; the number ignores its ADRs" >&2' \
    _ "$repo" {} |
    sed -n 's|^docs/adr/\([0-9]\{4\}-[^/]*\.md\)$|\1|p'
}

names="$(find "$dir" -maxdepth 1 -name '[0-9][0-9][0-9][0-9]-*.md' -printf '%f\n')"
if [[ -f "$dir/aliases.yaml" ]]; then
  names+=$'\n'"$(sed -n -E 's/^[[:space:]]*ADR-([0-9]{4}):.*/\1-retired/p' "$dir/aliases.yaml")"
fi
if ! command -v gh >/dev/null 2>&1; then
  echo "warning: gh is not installed; the number ignores the ADRs of open pull requests" >&2
elif pr_names="$(open_pr_adrs)"; then
  names+=$'\n'"$pr_names"
else
  echo "warning: could not list the open pull requests with gh; the number ignores their ADRs" >&2
fi

last="$(printf '%s\n' "$names" | grep -E '^[0-9]{4}-' | cut -c1-4 | sort | tail -n1 || true)"
next="$(printf '%04d' $((10#${last:-0} + 1)))"
file="$dir/${next}-${slug}.md"

sed -e "1s/.*/# ADR-${next}: ${title//\//\\/}/" \
    -e "s/^- Status: .*/- Status: Proposed/" \
    -e "s/^- Date: .*/- Date: $(date +%F)/" \
    "$dir/template.md" >"$file"

echo "$file"
