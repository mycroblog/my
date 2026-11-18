#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Prints the jobs of .github/workflows/ci.yml that a change runs, one per line: its `select` job asks, and each other
# job runs when it is named (ADR-0079 §5, §11). The rules below are the one place that says which job runs when.
#
#   scripts/ci-jobs.sh --event pull_request|merge_group [--full] --files FILE
#                         the jobs for the changed paths listed in FILE, one per line
#   scripts/ci-jobs.sh --list
#                         every job that has a rule (scripts/ci-jobs-test.sh compares it with the jobs of ci.yml)
#
# A job belongs to a stage:
#
#   pr    a pull request alone. What the pull request's own files decide, so that stacking pull requests in the merge
#         queue cannot change the result and the queue does not run it again (ADR-0079 §2), and what only a pull
#         request has: its base and its labels.
#   full  the merge queue, and a pull request labelled `allow:full-ci` (--full). What compares files across the tree,
#         compiles, or runs tests: two pull requests can pass it apart and fail it together (ADR-0079 §1).
#   both  a pull request and the merge queue: the lint and the tests of a Go module (ADR-0079 §3). A pull request
#         runs them so that a change that fails does not enter the queue, and the queue runs the tests again on the
#         merged tree; the job itself skips the lint there.
#
# Within its stage a job runs when the change can change its result:
#
#   ctx CONTEXT...   the change selects one of the contexts of scripts/changed-contexts.sh. Use it for a job that runs
#                    the checks of a context; a path no rule there knows selects every context.
#   changed GLOB...  a changed path matches one of the globs (`*` also matches `/`). Use it for a job whose result
#                    fewer paths decide than a context has: name every path the check reads.
#   always           any change.
#
# A change to the workflow itself, an action, or this script runs every job of the stage: nothing else shows that the
# jobs still work.
#
# To add a job: add its rule here, in the section of its stage, and the job to ci.yml with
# `if: contains(fromJSON(needs.select.outputs.jobs), '<name>')` and its name in `needs` of `gate`. The test fails
# while one of the two is missing.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

usage() {
  sed -n '7,10p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 2
}

event="" full=false files="" list=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    --event) event="${2:-}" && shift ;;
    --full) full=true ;;
    --files) files="${2:-}" && shift ;;
    --list) list=1 ;;
    *) usage ;;
  esac
  shift
done
if [ "$list" = 0 ]; then
  case "$event" in pull_request | merge_group) ;; *) usage ;; esac
  [ -f "$files" ] || usage
  # A merge group always runs the full check.
  if [ "$event" = merge_group ]; then full=true; fi
  mapfile -t paths < <(grep -v '^$' "$files" || true)
  contexts=" $("$root/scripts/changed-contexts.sh" --files "$files" | tr '\n' ' ')"
  contracts="$("$root/scripts/changed-contracts.sh" "$files")"
  web_packages="$("$root/scripts/changed-web-packages.sh" --files "$files" 2> /dev/null)"
fi

changed() {
  local path glob
  for path in "${paths[@]}"; do
    for glob in "$@"; do
      # shellcheck disable=SC2254 # the glob is a pattern
      case "$path" in $glob) return 0 ;; esac
    done
  done
  return 1
}

ctx() {
  local c
  for c in "$@"; do
    case "$contexts" in *" $c "*) return 0 ;; esac
  done
  return 1
}

always() { return 0; }

# either RULE [ARG...] -- RULE [ARG...]: one of the two rules holds.
either() {
  local first=()
  while [ "$#" -gt 0 ] && [ "$1" != -- ]; do
    first+=("$1")
    shift
  done
  shift
  "${first[@]}" || "$@"
}

everything=0
if [ "$list" = 0 ] && changed .github/workflows/ci.yml '.github/actions/*' scripts/ci-jobs.sh; then everything=1; fi

# job NAME STAGE RULE [ARG...] prints NAME when the event has the stage and the rule holds.
job() {
  local name="$1" stage="$2"
  shift 2
  if [ "$list" = 1 ]; then
    echo "$name"
    return 0
  fi
  case "$stage" in
    pr) [ "$event" = pull_request ] || return 0 ;;
    full) [ "$full" = true ] || return 0 ;;
    both) ;;
    *) echo "ci-jobs.sh: $name has the unknown stage $stage" >&2 && exit 1 ;;
  esac
  if [ "$everything" = 1 ] || "$@"; then echo "$name"; fi
}

# --- The rules that need more than a list of globs ------------------------------------------------------------------

# gofmt reads the Go files outside tools/ (go:fmt-check); golangci-lint formats those of tools/agentkit.
go_files() {
  local path
  for path in "${paths[@]}"; do
    case "$path" in
      tools/*) ;;
      *.go) return 0 ;;
    esac
  done
  return 1
}

# bun fmt reads the TypeScript and JavaScript of the workspace's members, by the [fmt] section of bunfig.toml.
ts_files() {
  local path
  for path in "${paths[@]}"; do
    case "$path" in
      bunfig.toml | .editorconfig) return 0 ;;
      web/*.ts | web/*.tsx | web/*.js | web/*.mjs) return 0 ;;
      pkg/mykit/ts/*.ts | pkg/mykit/ts/*.js | pkg/mykit/ts/*.mjs) return 0 ;;
      pkg/mybot/ts/*.ts | pkg/mybot/ts/*.js | pkg/mybot/ts/*.mjs) return 0 ;;
      tools/i18n/*.ts | tools/i18n/*.js | tools/i18n/*.mjs) return 0 ;;
    esac
  done
  return 1
}

contracts_selected() { [ -n "$contracts" ]; }
web_packages_selected() { [ -n "$web_packages" ]; }

# --- pr: what a pull request's own files, base, and labels decide ---------------------------------------------------

# Formatting: each file decides alone.
job go-format pr go_files
job ts-format pr ts_files
job tofu-format pr changed 'deploy/opentofu/*.tf' 'deploy/opentofu/*.tofu' 'deploy/opentofu/*.tfvars' 'deploy/opentofu/*.hcl' tasks/tofu.yml
# The SPDX identifier at the head of a file (api:license-scope): any file can carry the wrong one.
job source-license pr always
# The licenses of the dependencies: the manifests and lockfiles of each toolchain, the allowlists (Taskfile.yml,
# tasks/mykit.yml, tasks/web.yml), and the checkers. Dependabot's pull requests, which no one checks by hand, are why
# this runs in CI at all; the slow suite checks them all again before a release.
job deps-license pr changed go.mod go.sum 'pkg/mykit/go.*' 'pkg/mybot/go.*' bun.lock package.json '*/package.json' \
  Taskfile.yml tasks/go.yml tasks/mykit.yml tasks/web.yml tools/go.mod tools/go.sum tools/i18n/src/licenses.ts
job docs-lint pr changed '*.md' .markdownlint-cli2.jsonc tasks/docs.yml
# What the change adds, against its base, which the decider's label admits: the size of the prose agents maintain
# (docs:growth), the waivers of the copy lint (i18n:waivers), a schema change that loses data (db:lint).
job docs-change pr changed 'docs/adr/*' 'docs/architecture/*' 'docs/requirements/*' 'tools/agentkit/docs/*' \
  context-map.yaml '*/context-map.yaml' AGENTS.md '*/AGENTS.md' CLAUDE.md '*/CLAUDE.md' '.claude/*.md' \
  scripts/docs-growth.sh tasks/docs.yml
job i18n-change pr changed tools/i18n/waivers.json tools/i18n/src/waivers.ts tasks/i18n.yml
job schema-change pr ctx schema
# The tests of the schema lint itself, on its fixtures.
job schema-test pr changed scripts/db-lint.sh scripts/db-lint-test.sh 'schema/testdata/*' scripts/install-atlas.sh \
  tasks/db.yml
job shell-lint pr ctx shell
# time.Sleep in a Go test of any module, against the baseline, which may only shrink from the pull request's base.
job go-tests-lint pr changed '*_test.go' scripts/go-test-sleep.sh scripts/go-test-sleep.baseline tasks/go.yml
# The image (ADR-0079 §4): what builds it, what it starts from, and what the smoke test runs.
job docker-static pr changed Dockerfile .tool-versions tasks/docker.yml
job docker-smoke pr changed Dockerfile .dockerignore 'deploy/docker/*' 'web/brand/*' scripts/docker-smoke.sh \
  scripts/web-sign-in.sh scripts/check-release-build.sh tasks/docker.yml
job monitoring-test pr changed 'deploy/monitoring/*' tasks/docker.yml

# --- both: a Go module's lint, static checks, and tests ------------------------------------------------------------

job server-test both ctx go
# Its tests read the rules of the documents (docs/lint.yaml) too.
job agentkit-test both either ctx agentkit -- changed docs/lint.yaml
job mykit-test both ctx mykit

# --- full: what the merged tree decides -----------------------------------------------------------------------------

# go:mod-check also reads go.work and the go line of every module it lists, which are outside the go context.
job server-static full either ctx go -- changed go.work '*/go.mod'
# The contracts of the adapters the change touches, on the real service (scripts/changed-contracts.sh).
job server-contract full contracts_selected
# The schema applies and leaves no diff, applies on top of the base, and the sqlc code is current: the schema, the
# queries, and the code that applies the schema.
job schema-static full either ctx schema -- changed 'queries/*' sqlc.yaml scripts/sqlc.sh scripts/schema-files.sh \
  'internal/adapter/db/*' 'internal/adapter/queue/db/*' 'internal/adapter/kv/db/*' 'cmd/my/schema*' tasks/go.yml
# The spec and the code generated from it; api:live-calls also reads the scenarios and scripts that call the API, and
# api:plugin-docs-test tests the generator of the plugin protocol's reference.
job api-test full either ctx api -- changed 'test/federation-live/*' scripts/upgrade-test.sh scripts/restore-drill.sh \
  scripts/docker-smoke.sh 'tools/protodoc/*'
# What the documents say of the tree: any change can make a link, a context map, or an inventory wrong.
job docs-static full always
# That every check of `scripts/task.sh check` runs in a step: the tasks and the workflows.
job ci-static full changed Taskfile.yml 'tasks/*' '.github/workflows/*' scripts/ci-coverage.sh scripts/task.sh
job mykit-ts-test full ctx mykit
job i18n-test full ctx i18n
job site-test full ctx site
job site-build full ctx site
job tofu-static full ctx tofu
# The tests of the scripts read the files the scripts work on (the Dockerfile, the manifests, the tasks, the OpenTofu
# modules, the workspace), so more than a change of a script selects them.
job shell-test full either ctx shell tofu web -- changed Dockerfile package.json '*/package.json' 'tasks/*' \
  pkg/mybot/go.mod internal/platform/job_types.go internal/platform/setup_dbupgrade.go \
  internal/platform/config/retired.go docs/reference/configuration.md docs/operations/running.md api/openapi.yaml \
  'tools/agentkit/cmd/releasenotes/*' 'tools/agentkit/cmd/releaseprose/*' 'web/docs/worker/*'
job web-static full ctx web
job web-build full ctx web
# The tests of the web packages the change reaches (scripts/changed-web-packages.sh).
job web-test full web_packages_selected
# Not a check: the image the two browser jobs below run in.
job web-browser-image full ctx stories e2e
job web-stories-test full ctx stories
job web-e2e-build full ctx e2e
job web-e2e full ctx e2e
