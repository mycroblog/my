#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Generate the Go code of the plugin protocol from api/plugin/ into internal/adapter/plugin/gen, with buf (pinned in
# tools/buf/go.mod) and protoc-gen-go and protoc-gen-connect-go (pinned in tools/go.mod). The generators copy the leading
# comment of a .proto, whose license line is MIT; the generated code is AGPL-3.0-only (ADR-0146 §8), so this rewrites it.
#
#   scripts/plugin-generate.sh          regenerate in place
#   scripts/plugin-generate.sh --check  generate into a scratch directory and fail if the committed code differs
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
out="internal/adapter/plugin/gen"

scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT
go tool -modfile=tools/buf/go.mod buf generate api/plugin --template api/plugin/buf.gen.yaml -o "$scratch"
find "$scratch/$out" -name '*.go' -exec sed -i.bak '1s|^// SPDX-License-Identifier: MIT$|// SPDX-License-Identifier: AGPL-3.0-only|' {} +
find "$scratch/$out" -name '*.bak' -delete

if [ "${1:-}" = "--check" ]; then
  if ! diff -r "$out" "$scratch/$out"; then
    echo "$out is stale: run scripts/task.sh api:plugin-generate" >&2
    exit 1
  fi
  exit 0
fi
rm -rf "$out"
mkdir -p "$(dirname "$out")"
mv "$scratch/$out" "$out"
