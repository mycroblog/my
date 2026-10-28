#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests the dependency check of scripts/go-build-tags.sh (check-deps) on made-up package lists: above all, that a
# build without e2e_fakes that links the fake payment provider or the fake push service fails. It builds nothing and
# changes no file.
#
#   scripts/go-build-tags-test.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/scripts/go-build-tags.sh"
fake=github.com/mycroblog/my/internal/adapter/payment/fake
push=github.com/mycroblog/my/internal/adapter/pushservice/fake
fakes="$fake"$'\n'"$push"
stripe=github.com/mycroblog/my/internal/adapter/payment/stripe
plain="$(printf '%s\n' github.com/mycroblog/my/cmd/my github.com/mycroblog/my/internal/platform "$stripe")"

failed=0
fail() {
  echo "FAIL: $*" >&2
  failed=1
}

# expect_ok NAME SET DEPS passes when check-deps accepts DEPS of SET.
expect_ok() {
  if ! "$script" check-deps "$2" <<<"$3" > /dev/null 2>&1; then fail "$1: want it accepted"; fi
}

# expect_error NAME SET DEPS WANT passes when check-deps rejects DEPS of SET with a message containing WANT.
expect_error() {
  local out
  if out="$("$script" check-deps "$2" <<<"$3" 2>&1)"; then
    fail "$1: want an error"
  elif [[ "$out" != *"$4"* ]]; then
    fail "$1: want an error containing '$4', got '$out'"
  fi
}

expect_ok "the default build without the fake" "" "$plain"
expect_error "the default build linking the fake" "" "$plain"$'\n'"$fake" "depends on the test-only fake $fake"
expect_ok "a set of no_* tags without the fake" "no_postgres,no_valkey" "$plain"
expect_error "a set of no_* tags linking the fake" "no_postgres,no_valkey" "$plain"$'\n'"$fake" "only a build with e2e_fakes may link it"
expect_error "the default build linking the fake push service" "" "$plain"$'\n'"$push" "depends on the test-only fake $push"
expect_error "a set of no_* tags linking the fake push service" "no_postgres,no_valkey" "$plain"$'\n'"$push" "only a build with e2e_fakes may link it"
expect_ok "e2e_fakes with the fakes" "e2e_fakes" "$plain"$'\n'"$fakes"
expect_ok "e2e_fakes with no_* tags and the fakes" "no_valkey,e2e_fakes" "$plain"$'\n'"$fakes"
expect_error "e2e_fakes without the fake payment provider" "e2e_fakes" "$plain"$'\n'"$push" "does not depend on $fake"
expect_error "e2e_fakes without the fake push service" "e2e_fakes" "$plain"$'\n'"$fake" "does not depend on $push"
expect_error "a tag that leaves out an adapter still linked" "no_payments_stripe" "$plain" "still depends on $stripe"
expect_error "an unknown tag" "no_such_tag" "$plain" "unknown build tag"

if [ "$failed" -ne 0 ]; then exit 1; fi
echo "go-build-tags-test: ok"
