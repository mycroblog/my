#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Check the license scope of ADR-0093 §2: every file of api/spec/ and api/openapi.yaml carries
# SPDX-License-Identifier: MIT, the files of pkg/mykit/ and pkg/mybot/ that carry an identifier carry MIT, and every
# other tracked file that carries one carries AGPL-3.0-only. Only the first 5 lines are looked at, and files with no
# identifier outside api/spec/ are not checked here.
#
#   scripts/license-scope-check.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

failed=0
while IFS= read -r -d '' file; do
  [ -f "$file" ] || continue
  case "$file" in
    LICENSE | */LICENSE | docs/* | scripts/license-scope-check.sh | */testdata/* | */node_modules/*) continue ;;
  esac
  id="$(head -n 5 "$file" 2>/dev/null | grep -a -m1 -o 'SPDX-License-Identifier: [A-Za-z0-9.-]*' | sed 's/.*: //' || true)"
  case "$file" in
    api/spec/* | api/openapi.yaml) want=MIT required=1 ;;
    pkg/mykit/* | pkg/mybot/*) want=MIT required=0 ;;
    *) want=AGPL-3.0-only required=0 ;;
  esac
  if [ -z "$id" ]; then
    if [ "$required" -eq 1 ]; then
      echo "$file has no SPDX-License-Identifier (expected $want)" >&2
      failed=1
    fi
  elif [ "$id" != "$want" ]; then
    echo "$file has SPDX-License-Identifier: $id (expected $want)" >&2
    failed=1
  fi
done < <(git ls-files -z)

if [ "$failed" -ne 0 ]; then
  echo "MIT scope: api/spec/, api/openapi.yaml, pkg/mykit/, pkg/mybot/ (ADR-0093 §2); everything else is AGPL-3.0-only (ADR-0011)" >&2
  exit 1
fi
echo "SPDX identifiers match the license scope"
