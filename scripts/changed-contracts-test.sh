#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/changed-contracts.sh on made-up lists of changed paths: which container contracts each list selects.
#
#   scripts/changed-contracts-test.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/scripts/changed-contracts.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

failed=0

# expect NAME WANT PATH... passes when the paths select exactly the contracts WANT (space-separated, in the order the
# script prints them).
expect() {
  local name="$1" want="$2" got
  shift 2
  printf '%s\n' "$@" > "$tmp/files"
  got="$("$script" "$tmp/files" | tr '\n' ' ')"
  got="${got% }"
  if [ "$got" != "$want" ]; then
    echo "FAIL: $name: want '$want', got '$got'" >&2
    failed=1
  fi
}

expect "the s3 adapter" "blob" internal/adapter/blob/s3/s3.go
expect "the s3 test server" "blob" pkg/testkit/s3test/s3test.go
expect "the BlobStore contract" "blob" pkg/testkit/blobtest/blob_contract.go
expect "the postgres adapter" "postgres" internal/adapter/db/postgres/account/account.go
expect "the postgres schema" "postgres" schema/09-instance/postgres.sql
expect "the postgres queries" "postgres" queries/postgres/hook/hook.sql
expect "the stream adapter on postgres" "postgres stream" internal/adapter/stream/postgres/bus.go
expect "the valkey stream" "stream" internal/adapter/stream/valkey/bus.go
expect "the smtp mailer" "mail" internal/adapter/mail/smtp/smtp.go
expect "a search backend" "search" internal/adapter/search/meilisearch/index.go
expect "the workflow of the contracts" "postgres stream mail blob search acme" .github/workflows/contract.yml
expect "the workflow of the checks, which starts them" "postgres stream mail blob search acme" .github/workflows/ci.yml
expect "the acme adapter" "acme" internal/adapter/acme/acme.go
expect "the TLS manager" "acme" internal/app/instance/tls_manager.go
expect "the Pebble harness" "acme" pkg/testkit/pebbletest/pebbletest.go
expect "the tasks of the contracts" "postgres stream mail blob search acme" tasks/go.yml
expect "the sqlite adapter" "" internal/adapter/db/sqlite/account/account.go
expect "the sqlite schema" "" schema/09-instance/sqlite.sql
expect "a module of the application" "" internal/app/content/status.go
expect "the composition root" "" internal/platform/app.go
expect "a dependency update" "" go.mod go.sum
expect "two adapters" "mail blob" internal/adapter/blob/s3/s3.go internal/adapter/mail/smtp/smtp.go
expect "an in-memory fake beside the adapters" "" internal/adapter/blob/fs/fs.go

# A contract is named in four places, which must agree: ALL of the script, the options of contract.yml, the matrix of
# the slow suite, and a task go:test-<name> for each.
all="$("$script" --all | sort | tr '\n' ' ')"
names() { tr -d ' ' | tr ',' '\n' | sort | tr '\n' ' '; }
options="$(sed -n -E 's/^ +options: \[(.*)\]$/\1/p' "$root/.github/workflows/contract.yml" | names)"
matrix="$(sed -n '/^  server-contract:/,/^  [a-z]/p' "$root/.github/workflows/slow-suite.yml" | sed -n -E 's/^ +contract: \[(.*)\]$/\1/p' | names)"
if [ "$options" != "$all" ]; then
  echo "FAIL: the options of contract.yml are '$options', and ALL of scripts/changed-contracts.sh is '$all': a contract is in both" >&2
  failed=1
fi
if [ "$matrix" != "$all" ]; then
  echo "FAIL: the matrix of slow-suite.yml's server-contract is '$matrix', and ALL of scripts/changed-contracts.sh is '$all': a contract is in both" >&2
  failed=1
fi
for contract in $all; do
  if ! grep -q "^  test-$contract:\$" "$root/tasks/go.yml"; then
    echo "FAIL: tasks/go.yml has no task test-$contract, which contract.yml runs for the contract $contract" >&2
    failed=1
  fi
  # A contract with no case of its own would run in the slow suite alone.
  if ! grep -qF "selected[$contract]=1" "$script"; then
    echo "FAIL: scripts/changed-contracts.sh has no case that selects $contract: add one with the paths of its adapter, its contract suite, and its test server" >&2
    failed=1
  fi
done

if [ "$failed" -ne 0 ]; then
  exit 1
fi
echo "changed-contracts: ok"
