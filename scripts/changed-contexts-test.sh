#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/changed-contexts.sh on made-up lists of changed paths: which check contexts each list selects. It reads
# no git history and changes no file.
#
#   scripts/changed-contexts-test.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/scripts/changed-contexts.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

failed=0

# expect NAME WANT PATH... passes when the paths select exactly the contexts WANT (space-separated, in the order the
# script prints them).
expect() {
  local name="$1" want="$2" got
  shift 2
  printf '%s\n' "$@" > "$tmp/files"
  got="$("$script" --files "$tmp/files" | tr '\n' ' ')"
  got="${got% }"
  if [ "$got" != "$want" ]; then
    echo "FAIL: $name: want '$want', got '$got'" >&2
    failed=1
  fi
}

all="docs go i18n api web docker shell schema agentkit mykit tofu site"

# Shared configuration selects everything; the tasks of one area select that area and the areas that call its tasks;
# the workflows that run no check of a pull request or a merge group select nothing but agentkit (its context map).
expect "the map files" "agentkit" context-map.yaml internal/app/content/context-map.yaml tasks/context-map.yaml tools/agentkit/context-map.yaml
expect "the mtimes of the Go test cache" "go agentkit" scripts/stable-mtime.py
expect "the root Taskfile" "$all" Taskfile.yml
expect "the workflow of the checks" "$all" .github/workflows/ci.yml
expect "an action" "$all" .github/actions/setup-go/action.yml
expect "the tasks of tofu, which no other area calls" "agentkit tofu" tasks/tofu.yml
expect "the tasks of db" "schema agentkit" tasks/db.yml
expect "the tasks of the emoji tables, which no check runs" "agentkit" tasks/emoji.yml
expect "the tasks of the email previews" "go agentkit" tasks/mail.yml
expect "the Nightly" "agentkit" .github/workflows/nightly.yml
expect "the ignore list of git, which no check reads" "agentkit" .gitignore
expect "the tasks of the workspace, which the other areas call (web:install)" "docs go i18n api web agentkit mykit site" tasks/web.yml

# The OpenTofu modules: the public module and the internal ones select tofu, and their Markdown docs as well.
expect "the public module" "agentkit tofu" deploy/opentofu/mycroblog-gcp/main.tf
expect "an internal module" "agentkit tofu" deploy/opentofu/modules/internal/kv-external/main.tf
expect "the contract of the internal modules" "docs agentkit tofu" deploy/opentofu/modules/internal/README.md
# tofu:coverage reads the settings list, which go:config-docs writes from the configuration schema, and each classification.
expect "the settings list" "agentkit tofu" deploy/opentofu/modules/internal/settings/settings.json
expect "the classification of a public module" "agentkit tofu" deploy/opentofu/mycroblog-gcp/coverage.json
expect "the configuration reference" "docs go agentkit tofu site" docs/reference/configuration.md

# The documentation site: web/docs selects site.
expect "a change under web/docs alone" "agentkit site" web/docs/src/foo.ts
expect "a Markdown file of web/docs" "docs agentkit site" web/docs/README.md
expect "web/docs and a published document" "docs agentkit site" web/docs/astro.config.ts docs/operations/backup.md
# The workspace files every member installs, type-checks, and lints with select the members' contexts (and go, which the i18n generators write to).
expect "the workspace manifest" "go i18n web agentkit mykit site" package.json
expect "the workspace lockfile" "go i18n web agentkit mykit site" bun.lock
expect "the workspace's bunfig" "go i18n web agentkit mykit site" bunfig.toml
expect "the knip configuration" "go i18n web agentkit mykit site" knip.json
expect "the Oxlint configuration" "go i18n web agentkit mykit site" .oxlintrc.json
expect "the base tsconfig" "go i18n web agentkit mykit site" tsconfig.base.json
# The catalogs and their tools, the brand assets, and a package under web/.
expect "a catalog" "go i18n agentkit" locales/en/server/auth.json
expect "the catalog tools" "go i18n agentkit" tools/i18n/src/check.ts
expect "the copy role's knowledge, which the copy lint reads" "docs go i18n agentkit" .claude/knowledge/copy/decisions.md
expect "a brand asset" "web docker agentkit" web/brand/icon.svg
expect "a package under web/" "web agentkit" web/app/src/main.tsx
expect "the shared configuration" "$all" Taskfile.yml
expect "an unknown path" "$all" unknown/file.xyz

if [ "$failed" -ne 0 ]; then
  exit 1
fi
echo "changed-contexts.sh: all cases passed"
