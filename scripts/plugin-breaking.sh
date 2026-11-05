#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Fail when api/plugin/ breaks the v1 of the plugin protocol against the latest release from v0.1.0 on (ADR-0146 §9):
# `buf breaking` with the FILE rules, against a copy of the tag's api/plugin/. A pre-release does not freeze the protocol,
# and before the first such tag there is nothing to compare with. A shallow clone may lack the tag, so it is fetched by
# name, and the comparison is with a copy in a directory.
#
#   scripts/plugin-breaking.sh         compare with the latest release tag on origin
#   scripts/plugin-breaking.sh REF     compare with REF, which the local repository has
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

ref="${1:-}"
if [ -z "$ref" ]; then
  # A release is vX.Y.Z, and no v0.0.z was ever one of the protocol's.
  ref="$(git ls-remote --tags --refs origin 'refs/tags/v*' |
    sed -n 's|.*refs/tags/\(v[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\)$|\1|p' |
    grep -v '^v0\.0\.' | sort -V | tail -n 1 || true)"
  if [ -z "$ref" ]; then
    echo "no release tag from v0.1.0 on: nothing to compare the plugin protocol with"
    exit 0
  fi
  if ! git rev-parse -q --verify "refs/tags/$ref" >/dev/null; then
    git fetch -q --depth=1 origin "refs/tags/$ref:refs/tags/$ref"
  fi
  ref="refs/tags/$ref"
fi

scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT
git archive "$ref" api/plugin | tar -x -C "$scratch"
go tool -modfile=tools/buf/go.mod buf breaking api/plugin --against "$scratch/api/plugin"
