#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Fails when two files in docs/adr share a number, as two pull requests that each add an ADR can (ADR-0081),
# or when a file uses a number docs/adr/aliases.yaml retired, which old issues and pull requests still mean.
# The pull request that entered the merge queue later renumbers its ADR.
# Usage: scripts/adr-numbers.sh [dir]   (default: docs/adr)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
dir="${1:-$root/docs/adr}"

dups="$(find "$dir" -maxdepth 1 -name '[0-9][0-9][0-9][0-9]-*.md' -printf '%f\n' | cut -c1-4 | sort | uniq -d)"
if [[ -n "$dups" ]]; then
  for n in $dups; do
    echo "error: ADR-$n is used by more than one file:" >&2
    find "$dir" -maxdepth 1 -name "$n-*.md" -printf '  %f\n' | sort >&2
  done
  echo "Renumber the ADR the later pull request adds (the next free number: scripts/task.sh docs:adr-new picks one)." >&2
  exit 1
fi

if [[ -f "$dir/aliases.yaml" ]]; then
  bad=0
  while read -r n; do
    while read -r f; do
      echo "error: $f uses ADR-$n, which aliases.yaml retired" >&2
      bad=1
    done < <(find "$dir" -maxdepth 1 -name "$n-*.md" -printf '%f\n')
  done < <(sed -n -E 's/^[[:space:]]*ADR-([0-9]{4}):.*/\1/p' "$dir/aliases.yaml")
  if [[ "$bad" -ne 0 ]]; then
    echo "Merge it into the ADR that holds the number now, or pick the next free number (scripts/task.sh docs:adr-new)." >&2
    exit 1
  fi
fi
