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

# Shared configuration selects everything; the tasks of one area select that area and the areas that call its tasks;
# the workflows that run no check of a pull request or a merge group select nothing but agentkit (its context map).
expect "the mtimes of the Go test cache" "go agentkit" scripts/stable-mtime.py
expect "the root Taskfile" "docs go i18n mail api ui web docker shell schema agentkit mykit tofu site" Taskfile.yml
expect "the workflow of the checks" "docs go i18n mail api ui web docker shell schema agentkit mykit tofu site" .github/workflows/ci.yml
expect "the workflow of the story tests" "docs go i18n mail api ui web docker shell schema agentkit mykit tofu site" .github/workflows/ui-test.yml
expect "an action" "docs go i18n mail api ui web docker shell schema agentkit mykit tofu site" .github/actions/setup-go/action.yml
expect "the tasks of tofu, which no other area calls" "agentkit tofu" tasks/tofu.yml
expect "the tasks of db" "schema agentkit" tasks/db.yml
expect "the tasks of go, which web calls (go:bin)" "go web agentkit" tasks/go.yml
expect "the tasks of the emoji tables, which no check runs" "agentkit" tasks/emoji.yml
expect "the web E2E workflow" "agentkit" .github/workflows/web-e2e.yml
expect "the Nightly" "agentkit" .github/workflows/nightly.yml
expect "the tasks of web and the web E2E workflow (no tofu, docker, shell, or schema)" "docs go i18n mail api ui web agentkit mykit site" tasks/web.yml .github/workflows/web-e2e.yml

# The documentation site: app/docs selects site and no client context.
expect "a change under app/docs alone" "agentkit site" app/docs/src/foo.ts
expect "a Markdown file of app/docs" "docs agentkit site" app/docs/README.md
expect "app/docs and a published document" "docs agentkit site" app/docs/astro.config.ts docs/operations/backup.md
expect "app/docs and app/web" "web agentkit site" app/docs/src/foo.ts app/web/src/main.tsx
# The workspace files the site's checks read keep selecting the client contexts and site.
expect "the workspace manifest" "mail ui web agentkit site" app/package.json
expect "the workspace lockfile" "mail ui web agentkit site" app/bun.lock
expect "the workspace's bunfig" "mail ui web agentkit site" app/bunfig.toml
expect "the knip configuration" "ui web agentkit site" app/knip.json
expect "the Oxlint configuration" "ui web agentkit site" app/.oxlintrc.json
expect "the base tsconfig" "ui web agentkit site" app/tsconfig.base.json
# The client and the rest.
expect "a change under app/web alone" "web agentkit" app/web/src/main.tsx
expect "a change under app/ui" "ui web agentkit" app/ui/src/Button.tsx
expect "the shared configuration" "docs go i18n mail api ui web docker shell schema agentkit mykit tofu site" Taskfile.yml
expect "an unknown path" "docs go i18n mail api ui web docker shell schema agentkit mykit tofu site" unknown/file.xyz

if [ "$failed" -ne 0 ]; then
  exit 1
fi
echo "changed-contexts.sh: all cases passed"
