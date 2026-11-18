#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Prints the container contracts (ADR-0079 §5) that the changed paths listed in FILE, one per line, can break: one
# name per line, each the suffix of its task `scripts/task.sh go:test-<name>` and of its workflow. The merge queue runs
# the contracts it prints; the Nightly runs all of them, which also catches the ones a dependency update breaks
# (go.mod and go.sum select none here).
#
#   scripts/changed-contracts.sh FILE
#   scripts/changed-contracts.sh --all   every contract (the test compares it with the workflows and the tasks)
#
# A contract is selected by its adapter and by the contract suite and the test server of pkg/testkit it uses; a change
# to the tasks that define them (tasks/go.yml), to the workflow that runs them (contract.yml), or to what selects and
# starts the jobs (ci.yml, the actions, scripts/ci-jobs.sh, this script) selects every contract.
set -euo pipefail

[ "$#" -eq 1 ] || {
  echo "usage: scripts/changed-contracts.sh FILE | --all" >&2
  exit 2
}

ALL=(postgres stream mail blob voice search acme)
if [ "$1" = --all ]; then
  printf '%s\n' "${ALL[@]}"
  exit 0
fi
declare -A selected=()

while IFS= read -r path; do
  case "$path" in
    tasks/go.yml | .github/workflows/contract.yml | .github/workflows/ci.yml | .github/actions/* | \
      scripts/ci-jobs.sh | scripts/changed-contracts.sh)
      for c in "${ALL[@]}"; do selected[$c]=1; done
      ;;
    internal/adapter/db/postgres/* | internal/adapter/kv/db/* | internal/adapter/queue/db/* | \
      internal/adapter/stream/postgres/* | schema/*/postgres.sql | queries/postgres/* | pkg/testkit/pgtest/*)
      selected[postgres]=1
      ;;&
    internal/adapter/stream/* | pkg/testkit/streamtest/* | \
      pkg/testkit/awstest/* | pkg/testkit/valkeytest/*)
      selected[stream]=1
      ;;&
    internal/adapter/mail/smtp/* | pkg/testkit/kerneltest/mailer_contract.go | pkg/testkit/mailpittest/*)
      selected[mail]=1
      ;;&
    internal/adapter/blob/s3/* | pkg/testkit/blob.go | pkg/testkit/blobtest/* | pkg/testkit/s3test/*)
      selected[blob]=1
      ;;&
    internal/adapter/voice/livekit/* | pkg/testkit/voicetest/voice_transport_contract.go | pkg/testkit/livekittest/*)
      selected[voice]=1
      ;;&
    internal/adapter/search/* | pkg/testkit/searchtest/search_index_contract.go | pkg/testkit/elasticsearchtest/* | \
      pkg/testkit/meilisearchtest/*)
      selected[search]=1
      ;;&
    internal/adapter/acme/* | pkg/testkit/instancetest/acme_contract.go | pkg/testkit/pebbletest/* | \
      internal/app/instance/tls_*.go | internal/platform/serve_tls*.go | internal/platform/tls_acme.go | \
      internal/platform/tlsserve/*)
      selected[acme]=1
      ;;
  esac
done < "$1"

for c in "${ALL[@]}"; do
  if [ -n "${selected[$c]:-}" ]; then echo "$c"; fi
done
