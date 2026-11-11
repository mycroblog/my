#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Edits and queries the message catalogs of locales/ (tools/i18n/src/cli.ts): add, set, mv, rm, show, find, check,
# unused, prune-baseline. `scripts/i18n.sh help` prints the usage of each. Catalogs are changed through it alone.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

if [ ! -d "$root/tools/i18n/node_modules" ]; then
  "$root/scripts/task.sh" i18n:install >&2
fi
cd "$root/tools/i18n"
exec bun run src/cli.ts "$@"
