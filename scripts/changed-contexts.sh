#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Prints the check contexts that the changes since BASE touch, one per line, including the contexts that
# depend on them. `scripts/task.sh check:changed` runs the matching `<context>:check` tasks, and the `changes` job of
# .github/workflows/ci.yml picks from them the jobs of the full check (server-static, client-test, docker, ...).
#
#   scripts/changed-contexts.sh BASE          changes from the merge base of BASE and HEAD to the working tree,
#                                             plus untracked files (local use)
#   scripts/changed-contexts.sh --all         every context
#   scripts/changed-contexts.sh --files FILE  the paths listed in FILE, one per line (CI and tests)
#
# Contexts:
#   docs    Markdown lint and the ADR numbers                   (docs:check)
#   go      the Go module, sqlc, and the Go tests                (go:check)
#   i18n    the catalogs and their generated accessors           (i18n:check)
#   api     the OpenAPI spec and the code generated from it      (api:check)
#   ui      app/ui (@my/ui)                                      (ui:check, the story tests)
#   web     app/web and the workspace-wide client checks         (web:check)
#   docker  the Dockerfile and the deployments                   (docker:check, the Docker smoke test)
#   shell   the shell scripts                                    (shell:check)
#   schema  the desired-state schema files                       (db:check-all)
#   agentkit tools/agentkit and its context map, which covers every path (agentkit:check)
#
# A change to shared configuration (Taskfile.yml and tasks/, .tool-versions, tools/go.mod, the workflows and their
# actions) selects every context, and so does any path no rule below knows: a new kind of file runs everything until a
# rule is added for it. A dependency update selects the contexts of its toolchain only (go.mod: go; app/bun.lock: ui
# and web), so a Dependabot pull request runs the jobs that use the dependency.
set -euo pipefail

ALL=(docs go i18n api ui web docker shell schema agentkit)

root="$(cd "$(dirname "$0")/.." && pwd)"

usage() {
  echo "usage: scripts/changed-contexts.sh BASE | --all | --files FILE" >&2
  exit 2
}

[ "$#" -eq 1 ] || [ "$#" -eq 2 ] || usage
case "$1" in
  --all)
    printf '%s\n' "${ALL[@]}"
    exit 0
    ;;
  --files)
    [ "$#" -eq 2 ] || usage
    files="$(cat "$2")"
    ;;
  -*) usage ;;
  *)
    [ "$#" -eq 1 ] || usage
    base="$(git -C "$root" merge-base "$1" HEAD)"
    files="$(
      git -C "$root" diff --name-only "$base" --
      git -C "$root" ls-files --others --exclude-standard
    )"
    ;;
esac

declare -A selected=()
select_contexts() {
  local c
  for c in "$@"; do
    selected[$c]=1
  done
}

# classify PATH selects the contexts of one changed path. Rules add up: app/ui/README.md is docs, ui, and web.
classify() {
  local path="$1" known=0
  # tools/agentkit/context-map.yaml must cover every path, so any change can break `agentkit map check`.
  select_contexts agentkit
  case "$path" in
    # Shared configuration, which every check runs through: the task runner and its tasks, the tool versions (go-task,
    # sqlc, oapi-codegen, and go-licenses are pinned in tools/go.mod), and the workflows. Everything.
    Taskfile.yml | tasks/* | scripts/task.sh | .tool-versions | .github/workflows/* | .github/actions/* | scripts/changed-contexts.sh | \
      tools/go.mod | tools/go.sum)
      select_contexts "${ALL[@]}"
      return
      ;;
    # The dependencies of one toolchain: the contexts that build with them.
    go.mod | go.sum)
      select_contexts go
      return
      ;;
    app/bun.lock | app/bunfig.toml | app/package.json)
      select_contexts ui web
      return
      ;;
    # The spec; its dependents (go, web) are added below.
    api/*)
      select_contexts api
      return
      ;;
    # The catalogs and their tools: the Go accessors (go, added below as i18n's dependent) and the web client, which
    # bundles the client catalogs and the generated TypeScript accessors. @my/ui reads neither.
    locales/* | tools/i18n/*)
      select_contexts i18n web
      return
      ;;
  esac
  case "$path" in
    # A Go module of its own: its Go files are not the go context's.
    tools/agentkit/*.md)
      select_contexts docs
      return
      ;;
    tools/agentkit/*)
      return
      ;;
    scripts/agentkit*.sh)
      select_contexts shell
      return
      ;;
  esac
  case "$path" in
    *.md | .markdownlint-cli2.jsonc | docs/* | scripts/adr-new.sh | scripts/adr-numbers.sh)
      select_contexts docs
      known=1
      ;;
  esac
  case "$path" in
    # go test fails when the generated configuration reference is stale (go:config-docs).
    *.go | cmd/* | internal/* | pkg/* | queries/* | sqlc.yaml | .golangci.yml | scripts/sqlc.sh | scripts/go-build-tags.sh | \
      docs/reference/configuration.md)
      select_contexts go
      known=1
      ;;
  esac
  case "$path" in
    schema/* | scripts/install-atlas.sh)
      select_contexts schema go
      known=1
      ;;
  esac
  case "$path" in
    internal/i18n/*)
      select_contexts i18n
      known=1
      ;;
  esac
  case "$path" in
    # The shared text token vectors, which the web client's lib/textTokens.ts test reads too (ADR-0092 §4).
    pkg/testkit/fixtures/text/*)
      select_contexts web
      known=1
      ;;
  esac
  case "$path" in
    internal/interface/clientapi/* | app/web/src/lib/api/* | scripts/generate.sh | scripts/api-spec-check.sh | scripts/license-scope-check.sh)
      select_contexts api
      known=1
      ;;
  esac
  case "$path" in
    app/ui/* | app/tsconfig.base.json | app/.oxlintrc.json | app/oxlint/* | app/knip.json | scripts/ui-test-docker.sh)
      select_contexts ui
      known=1
      ;;
  esac
  case "$path" in
    # web:tokens fails when the token list generated from app/ui/src/theme.css is stale.
    app/* | scripts/playwright-image-check.sh | scripts/web-icons.sh | internal/domain/instance/theme_tokens.gen.go)
      select_contexts web
      known=1
      ;;
  esac
  case "$path" in
    Dockerfile | .dockerignore | deploy/* | scripts/docker-smoke.sh | scripts/federation-live.sh)
      select_contexts docker
      known=1
      ;;
  esac
  case "$path" in
    scripts/*.sh)
      select_contexts shell
      known=1
      ;;
  esac
  case "$path" in
    # Files no check reads.
    LICENSE | .editorconfig | .claude/* | .github/*)
      known=1
      ;;
  esac
  if [ "$known" -eq 0 ]; then
    select_contexts "${ALL[@]}"
  fi
}

while IFS= read -r path; do
  [ -n "$path" ] && classify "$path"
done <<< "$files"

# Dependents: @my/ui is compiled into app/web, the API code is generated into Go and app/web,
# the Go i18n accessors are Go, and the schema is embedded into Go.
[ -n "${selected[ui]:-}" ] && select_contexts web
[ -n "${selected[api]:-}" ] && select_contexts go web
[ -n "${selected[i18n]:-}" ] && select_contexts go

for c in "${ALL[@]}"; do
  if [ -n "${selected[$c]:-}" ]; then
    echo "$c"
  fi
done
