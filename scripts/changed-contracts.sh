#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Prints the container contracts (ADR-0079 §5) that the changed paths listed in FILE, one per line, can break: one
# name per line, each the suffix of its task `scripts/task.sh go:test-<name>` and of its workflow. The merge queue runs
# the contracts it prints; the Nightly runs all of them, which also catches the ones a dependency update breaks
# (go.mod and go.sum select none here).
#
#   scripts/changed-contracts.sh FILE
#
# A contract is selected by its adapter, the contract suite and the test server of pkg/testkit it uses, and its own
# workflow; a change to the tasks that define them (tasks/go.yml) selects every contract.
set -euo pipefail

[ "$#" -eq 1 ] || {
  echo "usage: scripts/changed-contracts.sh FILE" >&2
  exit 2
}

ALL=(postgres stream mail blob voice search)
declare -A selected=()

while IFS= read -r path; do
  case "$path" in
    tasks/go.yml)
      for c in "${ALL[@]}"; do selected[$c]=1; done
      ;;
    internal/adapter/db/postgres/* | internal/adapter/kv/db/* | internal/adapter/queue/db/* | \
      internal/adapter/stream/postgres/* | schema/*/postgres.sql | queries/postgres/* | pkg/testkit/pgtest/* | \
      .github/workflows/postgres-contract.yml)
      selected[postgres]=1
      ;;&
    internal/adapter/stream/* | pkg/testkit/streamtest/* | \
      pkg/testkit/awstest/* | pkg/testkit/valkeytest/* | .github/workflows/stream-contract.yml)
      selected[stream]=1
      ;;&
    internal/adapter/mail/smtp/* | pkg/testkit/kerneltest/mailer_contract.go | pkg/testkit/mailpittest/* | \
      .github/workflows/mail-integration.yml)
      selected[mail]=1
      ;;&
    internal/adapter/blob/s3/* | pkg/testkit/blob.go | pkg/testkit/blobtest/* | pkg/testkit/s3test/* | \
      .github/workflows/blob-contract.yml)
      selected[blob]=1
      ;;&
    internal/adapter/voice/livekit/* | pkg/testkit/voicetest/voice_transport_contract.go | pkg/testkit/livekittest/* | \
      .github/workflows/voice-contract.yml)
      selected[voice]=1
      ;;&
    internal/adapter/search/* | pkg/testkit/searchtest/search_index_contract.go | pkg/testkit/elasticsearchtest/* | \
      pkg/testkit/meilisearchtest/* | .github/workflows/search-contract.yml)
      selected[search]=1
      ;;
  esac
done < "$1"

for c in "${ALL[@]}"; do
  if [ -n "${selected[$c]:-}" ]; then echo "$c"; fi
done
