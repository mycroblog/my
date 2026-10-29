#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Checks the build tags of the server (docs/reference/build-tags.md): the tags that leave adapters out (NFR-PORT-4,
# ADR-0010, ADR-0015 Build matrix) and the test-only tag e2e_fakes, which adds the fakes (ADR-0117 §2, ADR-0118 §1). For each
# representative set of tags, it builds every package with CGO_ENABLED=0, runs go vet, and fails when cmd/my still
# depends on a package that a tag of the set leaves out. It fails as well when cmd/my without e2e_fakes (the default
# build with no tags among them) depends on a fake: a fake that records payments as paid or accepts any push message must never reach a released
# binary. The set with e2e_fakes also runs the tests that only that build has (named *E2EFakes*).
#
#   scripts/go-build-tags.sh                  every set below
#   scripts/go-build-tags.sh SET...           the given comma-separated sets of tags
#   scripts/go-build-tags.sh check-deps SET   checks the packages on stdin (`go list -deps ./cmd/my` built with SET)
#                                             without building anything; scripts/go-build-tags-test.sh tests it
set -euo pipefail

# The representative sets: together they use every tag.
sets=(
  # A self-hosted instance on SQLite with the fs blob store, without a broker or a metrics exporter.
  "no_postgres,no_valkey,no_sns,no_ses,no_s3,no_pubsub,no_gcpkms,no_otel,no_billing_aws,no_billing_gcp,no_payments_stripe"
  # AWS: PostgreSQL, the SNS broker, SES, the EMF metrics, the Stripe payments, without the Sora voice chat.
  "no_sqlite,no_valkey,no_pubsub,no_gcpkms,no_otel,no_billing_gcp,no_payments_payjp,no_voice_sora"
  # Google Cloud: PostgreSQL, the Pub/Sub broker, without the LiveKit voice chat.
  "no_sqlite,no_valkey,no_sns,no_ses,no_billing_aws,no_voice_livekit"
  # The test build of the end-to-end tests and the demonstration.
  "e2e_fakes"
)

# excluded_packages TAG SET prints the packages that TAG of SET leaves out of cmd/my: the adapters and their client
# library. The Pub/Sub client serves the broker and the budget method of the Google Cloud billing import, so only
# no_pubsub and no_billing_gcp together leave it out.
excluded_packages() {
  local m=github.com/mycroblog/my/internal/adapter
  case "$1" in
    no_pubsub | no_billing_gcp)
      if [[ ",$2," == *,no_pubsub,* && ",$2," == *,no_billing_gcp,* ]]; then
        echo "cloud.google.com/go/pubsub/v2"
      fi
      ;;
  esac
  case "$1" in
    no_sqlite) echo "$m/db/sqlite modernc.org/sqlite" ;;
    no_postgres) echo "$m/db/postgres $m/stream/postgres github.com/jackc/pgx/v5" ;;
    no_valkey) echo "$m/valkey $m/kv/valkey $m/stream/valkey github.com/valkey-io/valkey-go" ;;
    no_sns) echo "$m/stream/sns github.com/aws/aws-sdk-go-v2/service/sns" ;;
    no_ses) echo "$m/mail/ses github.com/aws/aws-sdk-go-v2/service/sesv2" ;;
    no_s3) echo "$m/blob/s3 github.com/aws/aws-sdk-go-v2/service/s3" ;;
    no_pubsub) echo "$m/stream/pubsub" ;;
    no_gcpkms) echo "$m/keymanager/gcpkms cloud.google.com/go/kms/apiv1" ;;
    no_billing_aws) echo "$m/billing/aws github.com/aws/aws-sdk-go-v2/service/costexplorer" ;;
    no_billing_gcp) echo "$m/billing/gcp google.golang.org/api/bigquery/v2" ;;
    no_payments_stripe) echo "$m/payment/stripe github.com/stripe/stripe-go/v87" ;;
    no_payments_payjp) echo "$m/payment/payjp" ;;
    no_voice_livekit) echo "$m/voice/livekit" ;;
    no_voice_sora) echo "$m/voice/sora" ;;
    no_otel) echo "$m/telemetry/otel $m/telemetry/otel/otlptest go.opentelemetry.io/otel/sdk/metric go.opentelemetry.io/otel/sdk/trace go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp" ;;
    e2e_fakes) ;; # adds the fakes and leaves nothing out
    *)
      echo "unknown build tag: $1 (see docs/reference/build-tags.md)" >&2
      return 1
      ;;
  esac
}

# fake_packages prints the packages of the fakes that only a build with e2e_fakes may link into cmd/my.
fake_packages() {
  echo "github.com/mycroblog/my/internal/adapter/payment/fake"
  echo "github.com/mycroblog/my/internal/adapter/pushservice/fake"
}

# check_deps SET reads the packages that cmd/my built with SET depends on from stdin and fails, printing why, when one
# of them is left out by a tag of SET or is a fake while SET has no e2e_fakes, or when a fake that e2e_fakes adds is
# missing.
check_deps() {
  local set="$1" deps status=0 tag pkg pkgs
  local -a tags=()
  deps="$(cat)"
  if [[ -n "$set" ]]; then
    IFS=, read -ra tags <<<"$set"
  fi
  for tag in "${tags[@]}"; do
    pkgs="$(excluded_packages "$tag" "$set")" || return 1
    for pkg in $pkgs; do
      if grep -qFx "$pkg" <<<"$deps"; then
        echo "cmd/my built with -tags '$set' still depends on $pkg, which $tag leaves out" >&2
        status=1
      fi
    done
  done
  for pkg in $(fake_packages); do
    if [[ ",$set," == *,e2e_fakes,* ]]; then
      if ! grep -qFx "$pkg" <<<"$deps"; then
        echo "cmd/my built with -tags '$set' does not depend on $pkg, which e2e_fakes adds" >&2
        status=1
      fi
    elif grep -qFx "$pkg" <<<"$deps"; then
      echo "cmd/my built with -tags '$set' depends on the test-only fake $pkg: only a build with e2e_fakes may link it" >&2
      status=1
    fi
  done
  return "$status"
}

if [[ "${1:-}" == check-deps ]]; then
  check_deps "${2-}"
  exit
fi
if (($# > 0)); then
  sets=("$@")
fi

status=0
# The default build, which go:build builds: only its dependencies are checked here.
echo "--- no tags"
go list -deps ./cmd/my | check_deps "" || status=1
for set in "${sets[@]}"; do
  echo "--- -tags $set"
  CGO_ENABLED=0 go build -tags "$set" ./...
  go vet -tags "$set" ./...
  go list -deps -tags "$set" ./cmd/my | check_deps "$set" || status=1
  if [[ ",$set," == *,e2e_fakes,* ]]; then
    mapfile -t pkgs < <(grep -rlE '^//go:build .*e2e_fakes' --include='*_test.go' internal cmd | xargs -n1 dirname | sort -u | sed 's|^|./|')
    go test -tags "$set" -run E2EFakes "${pkgs[@]}"
  fi
done
exit "$status"
