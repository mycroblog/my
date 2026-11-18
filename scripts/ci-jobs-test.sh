#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/ci-jobs.sh: that its rules and the jobs of .github/workflows/ci.yml name the same jobs, that `gate`
# needs every one of them (and the slow suite's `result` every job of the suite), and which jobs made-up lists of
# changed paths run. It reads no git history and changes no
# file.
#
#   scripts/ci-jobs-test.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/scripts/ci-jobs.sh"
workflow="$root/.github/workflows/ci.yml"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

failed=0
fail() {
  echo "FAIL: $*" >&2
  failed=1
}

# The jobs of the workflow: the keys two spaces deep under `jobs:`. `select` picks the jobs and `gate` sums them up,
# so neither has a rule.
sed -n '/^jobs:/,$p' "$workflow" | sed -n -E 's/^  ([a-z][a-z0-9-]*):.*/\1/p' | grep -vx -e select -e gate | sort > "$tmp/workflow"
"$script" --list | sort > "$tmp/rules"
if dup="$(uniq -d "$tmp/rules")" && [ -n "$dup" ]; then fail "scripts/ci-jobs.sh has two rules for: $dup"; fi
if ! diff "$tmp/rules" "$tmp/workflow" > "$tmp/diff"; then
  fail "the rules of scripts/ci-jobs.sh (<) and the jobs of ci.yml (>) differ: give each job a rule and each rule a job"
  cat "$tmp/diff" >&2
fi

# `gate` needs `select` and every job with a rule, or a failed job would not stop a merge.
sed -n '/^  gate:/,$p' "$workflow" | sed -n -E 's/^    needs: \[(.*)\]$/\1/p' | tr -d ' ' | tr ',' '\n' | grep -vx select | sort > "$tmp/needs"
if ! diff "$tmp/rules" "$tmp/needs" > "$tmp/diff"; then
  fail "the rules of scripts/ci-jobs.sh (<) and \`needs\` of gate in ci.yml (>) differ: gate must need every job"
  cat "$tmp/diff" >&2
fi

# Each job with a rule runs only when `select` names it.
while IFS= read -r name; do
  if ! sed -n "/^  $name:/,/^  [a-z]/p" "$workflow" | grep -qF "contains(fromJSON(needs.select.outputs.jobs), '$name')"; then
    fail "the job $name of ci.yml does not ask select: its \`if\` must be contains(fromJSON(needs.select.outputs.jobs), '$name')"
  fi
done < "$tmp/workflow"

# The slow suite's `result` needs every job of the suite, or a failed job would not fail a release or the Nightly.
suite="$root/.github/workflows/slow-suite.yml"
sed -n '/^jobs:/,$p' "$suite" | sed -n -E 's/^  ([a-z][a-z0-9-]*):.*/\1/p' | grep -vx result | sort > "$tmp/suite"
sed -n '/^  result:/,$p' "$suite" | sed -n -E 's/^    needs: \[(.*)\]$/\1/p' | tr -d ' ' | tr ',' '\n' | sort > "$tmp/suite-needs"
if ! diff "$tmp/suite" "$tmp/suite-needs" > "$tmp/diff"; then
  fail "the jobs of slow-suite.yml (<) and \`needs\` of its result (>) differ: result must need every job"
  cat "$tmp/diff" >&2
fi

# jobs EVENT PATH... prints the jobs of the event (pr, a pull request; full, a merge group; both, a pull request
# labelled allow:full-ci) for the paths, on one line.
jobs() {
  local event="$1" args=()
  shift
  case "$event" in
    pr) args=(--event pull_request) ;;
    full) args=(--event merge_group) ;;
    both) args=(--event pull_request --full) ;;
  esac
  printf '%s\n' "$@" > "$tmp/files"
  "$script" "${args[@]}" --files "$tmp/files" | tr '\n' ' ' | sed 's/ $//'
}

# expect NAME EVENT WANT PATH... passes when the paths run exactly the jobs WANT.
expect() {
  local name="$1" event="$2" want="$3" got
  shift 3
  got="$(jobs "$event" "$@")"
  if [ "$got" != "$want" ]; then fail "$name: want '$want', got '$got'"; fi
}

# has NAME EVENT JOB PATH... passes when the paths run JOB; lacks, when they do not.
has() {
  local name="$1" event="$2" job="$3"
  shift 3
  case " $(jobs "$event" "$@") " in *" $job "*) ;; *) fail "$name: want $job among '$(jobs "$event" "$@")'" ;; esac
}
lacks() {
  local name="$1" event="$2" job="$3"
  shift 3
  case " $(jobs "$event" "$@") " in *" $job "*) fail "$name: want no $job among '$(jobs "$event" "$@")'" ;; esac
}

# A pull request runs what its files decide; the merge queue runs the rest and none of it again.
expect "a document, on a pull request" pr "source-license docs-lint" CONTRIBUTING.md
expect "a document, in the merge queue" full "docs-static" CONTRIBUTING.md
expect "a published document, in the merge queue" full "docs-static site-test site-build" docs/guide/posting.md
expect "a decision, on a pull request" pr "source-license docs-lint docs-change" docs/adr/0079-ci-single-gate-merge-queue.md
expect "a context map" pr "source-license docs-change" internal/app/content/context-map.yaml
expect "the server's code, on a pull request" pr "go-format source-license server-test" internal/app/content/text.go
expect "the server's code, in the merge queue" full "server-test server-static docs-static web-browser-image web-e2e-build web-e2e" internal/app/content/text.go
expect "the server's code, on a pull request labelled allow:full-ci" both \
  "go-format source-license server-test server-static docs-static web-browser-image web-e2e-build web-e2e" internal/app/content/text.go
expect "a web package, on a pull request" pr "ts-format source-license" web/app/src/main.tsx
expect "a script, on a pull request" pr "source-license shell-lint" scripts/adr-new.sh

# A job runs only for the paths its checks read.
has "an adapter selects its contract" full server-contract internal/adapter/blob/s3/s3.go
lacks "the application selects no contract" full server-contract internal/app/content/text.go
lacks "the server's code runs no check of the spec" full api-test internal/app/content/text.go
lacks "the server's code runs no check of the schema" full schema-static internal/app/content/text.go
has "the schema" full schema-static schema/09-instance/postgres.sql
has "a query" full schema-static queries/postgres/hook/hook.sql
has "the code that applies the schema" full schema-static internal/adapter/db/postgres/schema_apply.go
has "the spec" full api-test api/spec/openapi.yaml
has "a script whose calls of the API api:live-calls reads" full api-test scripts/restore-drill.sh
lacks "the server's code runs no check of agentkit" full agentkit-test internal/app/content/text.go
has "agentkit" full agentkit-test tools/agentkit/internal/ops/pr.go
has "agentkit, on a pull request" pr agentkit-test tools/agentkit/internal/ops/pr.go
has "the server's go.sum, which bounds agentkit's dependencies" full agentkit-test go.sum
has "the client libraries' Go modules, on a pull request" pr mykit-test pkg/mykit/client.go
lacks "the npm packages of the client libraries wait for the merge queue" pr mykit-ts-test pkg/mykit/ts/src/client.ts
has "the npm packages of the client libraries" full mykit-ts-test pkg/mykit/ts/src/client.ts
has "a dependency, on a pull request" pr deps-license bun.lock
has "a Go dependency, on a pull request" pr deps-license go.sum
lacks "the server's code changes no dependency" pr deps-license internal/app/content/text.go
has "the Dockerfile" pr docker-smoke Dockerfile
has "the Compose files of a deployment" pr docker-smoke deploy/docker/compose.yaml
lacks "a Go tool builds no image" pr docker-smoke tools/go.mod
lacks "the development Compose file is not the deployment's" pr docker-smoke compose.yaml
lacks "the alerting rules build no image" pr docker-smoke deploy/monitoring/prometheus/alerts.yml
has "the alerting rules" pr monitoring-test deploy/monitoring/prometheus/alerts.yml
has "the versions the Dockerfile must agree with" pr docker-static .tool-versions
has "the waivers of the copy lint" pr i18n-change tools/i18n/waivers.json
has "a schema change, which the decider's label admits" pr schema-change schema/09-instance/postgres.sql
lacks "a schema change does not test the lint" pr schema-test schema/09-instance/postgres.sql
has "the schema lint" pr schema-test scripts/db-lint.sh
has "a task" full ci-static tasks/web.yml
has "another workflow" full ci-static .github/workflows/slow-suite.yml
lacks "the server's code changes no workflow" full ci-static internal/app/content/text.go
has "the OpenTofu modules" full tofu-static deploy/opentofu/mycroblog-gcp/main.tf
has "the OpenTofu modules, on a pull request" pr tofu-format deploy/opentofu/mycroblog-gcp/main.tf
has "a file the tests of the scripts read" full shell-test Dockerfile
has "a script" full shell-test scripts/sqlc.sh
has "a web package's tests" full web-test web/app/src/main.tsx
lacks "the documentation site has no tests among the web packages" full web-test web/docs/src/foo.ts
has "the documentation site" full site-build web/docs/src/foo.ts

# The workflow, an action, and the rules themselves run every job of the stage.
"$script" --list | tr '\n' ' ' | sed 's/ $//' > "$tmp/all"
expect "the workflow, on a pull request labelled allow:full-ci" both "$(cat "$tmp/all")" .github/workflows/ci.yml
expect "an action" both "$(cat "$tmp/all")" .github/actions/setup-go/action.yml
expect "the rules" both "$(cat "$tmp/all")" scripts/ci-jobs.sh
lacks "the workflow, in the merge queue, runs no job of a pull request" full docker-smoke .github/workflows/ci.yml
has "the workflow, in the merge queue" full server-contract .github/workflows/ci.yml

if [ "$failed" -ne 0 ]; then
  exit 1
fi
echo "ci-jobs: ok"
