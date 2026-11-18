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

all="docs go i18n api web docker shell schema agentkit mykit tofu site e2e stories"

# Shared configuration selects everything; the tasks of one area select that area and the areas that call its tasks;
# the workflows that run no check of a pull request or a merge group select nothing but docs, which every path selects
# (the context maps cover it, and the documents name and link to what the code has).
expect "the map files" "docs" context-map.yaml internal/app/content/context-map.yaml tasks/context-map.yaml tools/agentkit/context-map.yaml
expect "the mtimes of the Go test cache" "docs go" scripts/stable-mtime.py
expect "the root Taskfile" "$all" Taskfile.yml
expect "the workflow of the checks" "$all" .github/workflows/ci.yml
expect "an action" "$all" .github/actions/setup-go/action.yml
expect "the tasks of tofu, which no other area calls" "docs tofu" tasks/tofu.yml
expect "the tasks of db" "docs schema" tasks/db.yml
expect "the tasks of the emoji tables, which no check runs" "docs" tasks/emoji.yml
expect "the tasks of the email previews" "docs go" tasks/mail.yml
expect "the Nightly" "docs" .github/workflows/nightly.yml
expect "the ignore list of git, which no check reads" "docs" .gitignore
expect "the tasks of the workspace, which the other areas call (web:install)" "docs go i18n api web mykit site e2e stories" tasks/web.yml

# The OpenTofu modules: the public module and the internal ones select tofu, and their Markdown docs as well.
expect "the public module" "docs tofu" deploy/opentofu/mycroblog-gcp/main.tf
expect "an internal module" "docs tofu" deploy/opentofu/modules/internal/kv-external/main.tf
expect "the contract of the internal modules" "docs tofu" deploy/opentofu/modules/internal/README.md
# tofu:coverage reads the settings list, which go:config-docs writes from the configuration schema, and each classification.
expect "the settings list" "docs tofu" deploy/opentofu/modules/internal/settings/settings.json
expect "the classification of a public module" "docs tofu" deploy/opentofu/mycroblog-gcp/coverage.json
expect "the configuration reference" "docs go tofu site" docs/reference/configuration.md

# The documentation site: web/docs selects site.
expect "a change under web/docs alone" "docs site" web/docs/src/foo.ts
expect "a Markdown file of web/docs" "docs site" web/docs/README.md
expect "web/docs and a published document" "docs site" web/docs/astro.config.ts docs/operations/backup.md
# The workspace files every member installs, type-checks, and lints with select the members' contexts (and go, which the i18n generators write to).
expect "the workspace manifest" "docs go i18n web mykit site" package.json
expect "the workspace lockfile" "docs go i18n web mykit site e2e stories" bun.lock
expect "the workspace's bunfig" "docs go i18n web mykit site" bunfig.toml
expect "the knip configuration" "docs go i18n web mykit site" knip.json
expect "the Oxlint configuration" "docs go i18n web mykit site" .oxlintrc.json
expect "the base tsconfig" "docs go i18n web mykit site" tsconfig.base.json
# The catalogs and their tools, the brand assets, and a package under web/.
expect "a catalog" "docs go i18n" locales/en/server/auth.json
expect "the catalog tools" "docs go i18n" tools/i18n/src/check.ts
expect "a web package's catalog, which @my/i18n compiles for it" "docs go i18n web e2e stories" web/design-system/locales/en/disclosure.json
expect "@my/i18n" "docs go i18n web" web/i18n/src/compile.ts
expect "the copy role's knowledge, which the copy lint reads" "docs go i18n" .claude/knowledge/copy/decisions.md
expect "a brand asset" "docs web docker" web/brand/icon.svg
expect "a package under web/" "docs web e2e stories" web/app/src/main.tsx
expect "the selection of the packages web:test runs" "docs web shell" scripts/changed-web-packages.sh
# The E2E tests run when what they run changes: a workspace package @my/app or @my/e2e depends on, or the server. The
# tests on the built catalog run when a workspace package the Storybook host depends on changes.
expect "@my/e2e, whose fake server the screens' stories render on" "docs web e2e stories" web/e2e/specs/sign-in.spec.ts
expect "a package the app depends on, through @my/app-data" "docs mykit e2e stories" pkg/mykit/ts/src/client.ts
expect "the Storybook host, which the E2E tests do not run" "docs web stories" web/storybook/shot.ts
expect "a package neither depends on" "docs go i18n web" web/i18n/src/format.ts
expect "a package outside the graph" "docs mykit" pkg/mybot/ts/src/index.ts
expect "the server's code" "docs go e2e" internal/app/content/text.go
expect "the vectors of the text rules, which a test of web/app-features reads too" "docs go web" pkg/testkit/fixtures/text/tokens.json
expect "the tasks of bin/my, which web:e2e calls" "docs go web e2e stories" tasks/go.yml
expect "the shared configuration" "$all" Taskfile.yml
# tools/agentkit is a Go module of its own: its code, its tasks, the script that starts it, and the server's go.sum,
# which bounds its dependencies, select its checks; nothing else does.
expect "the code of agentkit" "docs agentkit" tools/agentkit/internal/ops/pr.go
expect "a document of agentkit" "docs" tools/agentkit/README.md
expect "the tasks of agentkit, whose build the tasks of the documents and their site call" "docs agentkit site" tasks/agentkit.yml
expect "the script that builds and starts agentkit" "docs shell agentkit" scripts/agentkit.sh
expect "the server's go.sum" "docs go agentkit mykit e2e" go.sum
expect "an unknown path" "$all" unknown/file.xyz

if [ "$failed" -ne 0 ]; then
  exit 1
fi
echo "changed-contexts.sh: all cases passed"
