#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Check the license scope of ADR-0093 §2: every file of api/spec/, api/openapi.yaml, and api/plugin/ carries
# SPDX-License-Identifier: MIT, the files of pkg/mykit/ and pkg/mybot/ that carry an identifier carry MIT, and every
# other tracked file that carries one carries AGPL-3.0-only. Only the first 5 lines are looked at, and files with no
# identifier outside api/spec/, api/openapi.yaml and api/plugin/ are not checked here. Each file decides alone, so a
# pull request's own files decide the result.
#
#   scripts/license-scope-check.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

# One `git grep` over the tracked files finds every identifier with its line, and one awk keeps the first of each
# file's first five lines and compares it: the tree has thousands of files, and a process or three for each took
# minutes. Binary files are left out (-I): none carries an identifier.
#
# The identifiers found and the files that must carry one, as "<path>\t<identifier>" with an empty identifier for the
# latter, so that awk reads what was found first and then what is required.
failed=0
report="$(
  {
    git grep -I -n -o -E 'SPDX-License-Identifier: [A-Za-z0-9.-]*' -- . |
      awk -F: '$2 <= 5 && !seen[$1]++ { id = $0; sub(/^[^:]*:[^:]*:SPDX-License-Identifier: /, "", id); print $1 "\t" id }'
    git ls-files -- api/spec api/openapi.yaml api/plugin | sed 's/$/\t/'
  } | awk -F '\t' '
    function skipped(file) {
      return file == "LICENSE" || file ~ /\/LICENSE$/ || file ~ /^docs\// || file == "scripts/license-scope-check.sh" ||
        file ~ /\/testdata\// || file ~ /\/node_modules\//
    }
    function want(file) {
      if (file ~ /^api\/spec\// || file == "api/openapi.yaml" || file ~ /^api\/plugin\// ||
          file ~ /^pkg\/mykit\// || file ~ /^pkg\/mybot\//) return "MIT"
      return "AGPL-3.0-only"
    }
    skipped($1) { next }
    $2 != "" {
      found[$1] = 1
      if ($2 != want($1)) printf "%s has SPDX-License-Identifier: %s (expected %s)\n", $1, $2, want($1)
      next
    }
    !($1 in found) { printf "%s has no SPDX-License-Identifier (expected %s)\n", $1, want($1) }
  '
)"
if [ -n "$report" ]; then
  printf '%s\n' "$report" >&2
  failed=1
fi

if [ "$failed" -ne 0 ]; then
  echo "MIT scope: api/spec/, api/openapi.yaml, api/plugin/, pkg/mykit/, pkg/mybot/ (ADR-0093 §2); everything else is AGPL-3.0-only (ADR-0011)" >&2
  exit 1
fi
echo "SPDX identifiers match the license scope"
