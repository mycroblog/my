#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# Fails when a workflow's Playwright image differs from PLAYWRIGHT_IMAGE in Taskfile.yml, or when the image's
# Playwright version differs from the one the client workspace pins. The browser in the image must be the one
# the pinned playwright package drives, and every screenshot baseline comes from this one image (ADR-0110).
# Usage: PLAYWRIGHT_IMAGE=<image@digest> scripts/playwright-image-check.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
image="${PLAYWRIGHT_IMAGE:?set PLAYWRIGHT_IMAGE (scripts/task.sh web:playwright-image-check does)}"
status=0

# mcr.microsoft.com/playwright:v1.63.0-noble@sha256:... -> 1.63.0
want="$(sed -E 's/^[^:]+:v([0-9.]+)-.*/\1/' <<<"$image")"
for pkg in app/web/package.json app/ui/package.json; do
	while read -r name version; do
		if [[ "$version" != "$want" ]]; then
			echo "$pkg: $name is $version but PLAYWRIGHT_IMAGE is Playwright $want" >&2
			status=1
		fi
	done < <(sed -nE 's/^ *"(@playwright\/test|playwright)": "([^"]+)".*/\1 \2/p' "$root/$pkg")
done

while IFS=: read -r file _ ref; do
	ref="${ref#"${ref%%[![:space:]]*}"}"
	ref="${ref#image: }"
	if [[ "$ref" != "$image" ]]; then
		echo "${file#"$root/"}: Playwright image $ref differs from PLAYWRIGHT_IMAGE $image" >&2
		status=1
	fi
done < <(grep -n 'mcr.microsoft.com/playwright:' "$root"/.github/workflows/*.yml)

exit "$status"
