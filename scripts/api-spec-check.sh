#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Check the layout of the split client API spec in api/spec/ (ADR-0066): the root api/spec/openapi.yaml
# lists every file under api/spec/paths/ and api/spec/components/ (redocly bundle silently leaves out a
# component that nothing refers to) and no file that does not exist, and every file starts with the
# SPDX header.
#
#   scripts/api-spec-check.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
spec="$root/api/spec"
spdx='# SPDX-License-Identifier: MIT'

failed=0

# The files that the root refers to with "$ref: ./<file>", and the files on disk.
mapfile -t listed < <(sed -n "s|^ *\\\$ref: \\./||p" "$spec/openapi.yaml" | sort)
mapfile -t present < <(cd "$spec" && find paths components -type f | sort)

mapfile -t unlisted < <(comm -13 <(printf '%s\n' "${listed[@]}") <(printf '%s\n' "${present[@]}"))
if [ "${#unlisted[@]}" -ne 0 ]; then
  echo "api/spec/openapi.yaml does not list these files (add a \$ref to each under paths or components):" >&2
  printf '  api/spec/%s\n' "${unlisted[@]}" >&2
  failed=1
fi

mapfile -t missing < <(comm -23 <(printf '%s\n' "${listed[@]}") <(printf '%s\n' "${present[@]}"))
if [ "${#missing[@]}" -ne 0 ]; then
  echo "api/spec/openapi.yaml lists files that do not exist:" >&2
  printf '  api/spec/%s\n' "${missing[@]}" >&2
  failed=1
fi

for file in openapi.yaml "${present[@]}"; do
  if [ "$(head -n 1 "$spec/$file")" != "$spdx" ]; then
    echo "api/spec/$file does not start with '$spdx'" >&2
    failed=1
  fi
done

if [ "$failed" -ne 0 ]; then
  exit 1
fi
echo "api/spec/openapi.yaml lists every file of api/spec"
