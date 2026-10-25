#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Picks the release that a candidate version must upgrade from (docs/operations/releasing.md, "The upgrade test"):
# the latest patch of the latest minor version before the candidate's, among the release tags read from stdin (one
# per line; anything other than vX.Y.Z, such as a pre-release or a library's pkg/mykit/vX.Y.Z, is ignored).
# Without a candidate, the latest release: the upgrade that the head of main has to support whatever it becomes.
# Prints the tag, or nothing when there is none (the first minor version).
#
#   git tag -l 'v[0-9]*' | scripts/previous-minor.sh v0.3.0     # the latest v0.2.Z, or else the latest v0.1.Z, ...
#   git tag -l 'v[0-9]*' | scripts/previous-minor.sh             # the latest release
set -euo pipefail

candidate="${1:-}"
semver='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?$'
if [[ -n "$candidate" && ! "$candidate" =~ $semver ]]; then
  echo "previous-minor: the version must be vX.Y.Z or vX.Y.Z-<pre-release>: $candidate" >&2
  exit 2
fi
major="${BASH_REMATCH[1]:-}"
minor="${BASH_REMATCH[2]:-}"

best=""
best_key=""
while IFS= read -r tag; do
  [[ "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || continue
  m="${BASH_REMATCH[1]}" n="${BASH_REMATCH[2]}" p="${BASH_REMATCH[3]}"
  # Only minor versions before the candidate's: a patch of the candidate's own minor version is not the previous one.
  if [[ -n "$candidate" ]] && ! ((m < major || (m == major && n < minor))); then
    continue
  fi
  key="$(printf '%09d.%09d.%09d' "$m" "$n" "$p")"
  if [[ "$key" > "$best_key" ]]; then
    best="$tag" best_key="$key"
  fi
done

if [[ -n "$best" ]]; then
  echo "$best"
fi
