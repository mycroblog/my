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
expect "the BlobStore contract" "blob" pkg/testkit/blob_contract.go
expect "the postgres adapter" "postgres" internal/adapter/db/postgres/account.go
expect "the postgres schema" "postgres" schema/postgres/09-instance.sql
expect "the postgres queries" "postgres" queries/postgres/hook.sql
expect "the stream adapter on postgres" "postgres stream" internal/adapter/stream/postgres/bus.go
expect "the valkey stream" "stream" internal/adapter/stream/valkey/bus.go
expect "the smtp mailer" "mail" internal/adapter/mail/smtp/smtp.go
expect "the livekit transport" "voice" internal/adapter/voice/livekit/transport.go
expect "a search backend" "search" internal/adapter/search/meilisearch/index.go
expect "a contract workflow" "voice" .github/workflows/voice-contract.yml
expect "the tasks of the contracts" "postgres stream mail blob voice search" tasks/go.yml
expect "the sqlite adapter" "" internal/adapter/db/sqlite/account.go
expect "the sqlite schema" "" schema/sqlite/09-instance.sql
expect "a module of the application" "" internal/app/content/status.go
expect "the composition root" "" internal/platform/app.go
expect "a dependency update" "" go.mod go.sum
expect "two adapters" "mail blob" internal/adapter/blob/s3/s3.go internal/adapter/mail/smtp/smtp.go
expect "an in-memory fake beside the adapters" "" internal/adapter/blob/fs/fs.go

if [ "$failed" -ne 0 ]; then
  exit 1
fi
echo "changed-contracts: ok"
