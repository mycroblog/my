#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Installs the Bun of .tool-versions from its npm package, for CI jobs in a container image that has Node.js and
# npm but not the unzip that oven-sh/setup-bun extracts Bun with (the Playwright image). The package holds the same
# binary as the release archive, and the Bun package cache stays in ~/.bun/install/cache.
#
#   scripts/ci-bun-npm.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

version="$(awk '$1 == "bun" { print $2 }' "$root/.tool-versions")"
if [[ -z "$version" ]]; then
  echo "ci-bun-npm: .tool-versions names no bun" >&2
  exit 1
fi

npm install --global --no-fund --no-audit "bun@$version"

got="$(bun --version)"
if [[ "$got" != "$version" ]]; then
  echo "ci-bun-npm: bun on PATH is $got, want $version" >&2
  exit 1
fi
echo "ci-bun-npm: installed bun $got"
