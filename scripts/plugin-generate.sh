#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Generate the Go code of the plugin protocol from api/plugin/ into internal/adapter/plugin/gen, with buf (pinned in
# tools/buf/go.mod) and protoc-gen-go and protoc-gen-connect-go (pinned in tools/go.mod). The generators copy the leading
# comment of a .proto, whose license line is MIT; the generated code is AGPL-3.0-only (ADR-0146 §8), so this rewrites it.
# It also writes the reference pages of docs/plugins, one per package, from the comments of the .proto files
# (tools/protodoc). The hand-written pages of docs/plugins, which lack the generated marker, stay as they are.
#
#   scripts/plugin-generate.sh          regenerate in place
#   scripts/plugin-generate.sh --check  generate into a scratch directory and fail if the committed files differ
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
export GOWORK=off
out="internal/adapter/plugin/gen"
docs="docs/plugins"
marker='<!-- Code generated from api/plugin by "scripts/task.sh api:plugin-generate". DO NOT EDIT. -->'

scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT
go tool -modfile=tools/buf/go.mod buf generate api/plugin --template api/plugin/buf.gen.yaml -o "$scratch"
find "$scratch/$out" -name '*.go' -exec sed -i.bak '1s|^// SPDX-License-Identifier: MIT$|// SPDX-License-Identifier: AGPL-3.0-only|' {} +
find "$scratch/$out" -name '*.bak' -delete

# generated_pages lists the pages of dir that the generator wrote, by their first line.
generated_pages() {
  local f
  for f in "$1"/*.md; do
    [ -e "$f" ] || continue
    if [ "$(head -n 1 "$f")" = "$marker" ]; then
      basename "$f"
    fi
  done
}

if [ "${1:-}" = "--check" ]; then
  stale=0
  if ! diff -r "$out" "$scratch/$out"; then
    stale=1
  fi
  if ! diff <(generated_pages "$docs") <(generated_pages "$scratch/$docs"); then
    stale=1
  fi
  for page in $(generated_pages "$scratch/$docs"); do
    if ! diff "$docs/$page" "$scratch/$docs/$page"; then
      stale=1
    fi
  done
  if [ "$stale" = 1 ]; then
    echo "$out or $docs is stale: run scripts/task.sh api:plugin-generate" >&2
    exit 1
  fi
  exit 0
fi
rm -rf "$out"
mkdir -p "$(dirname "$out")" "$docs"
mv "$scratch/$out" "$out"
for page in $(generated_pages "$docs"); do
  rm "$docs/$page"
done
for page in $(generated_pages "$scratch/$docs"); do
  mv "$scratch/$docs/$page" "$docs/$page"
done
