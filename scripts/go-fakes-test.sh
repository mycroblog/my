#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests the dependency check of scripts/go-fakes.sh (check-deps) on made-up package lists: above all, that a build
# without e2e_fakes that links the fake payment provider, push service, search index, translation provider, mailer, or
# OpenID Connect provider fails.
# It builds nothing and changes no file.
#
#   scripts/go-fakes-test.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/scripts/go-fakes.sh"
fake=github.com/mycroblog/my/internal/adapter/payment/fake
push=github.com/mycroblog/my/internal/adapter/pushservice/fake
search=github.com/mycroblog/my/internal/adapter/search/fake
translator=github.com/mycroblog/my/internal/adapter/translator/fake
mail=github.com/mycroblog/my/internal/adapter/mail/fake
oidc=github.com/mycroblog/my/internal/adapter/oidc/fake
fakes="$fake"$'\n'"$push"$'\n'"$search"$'\n'"$translator"$'\n'"$mail"$'\n'"$oidc"
stripe=github.com/mycroblog/my/internal/adapter/payment/stripe
fakeplugin=github.com/mycroblog/my/internal/adapter/plugin/fakeplugin
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
expect_error "another tag does not admit the fake" "no_valkey" "$plain"$'\n'"$fake" "only a build with e2e_fakes may link it"
expect_error "the default build linking the fake push service" "" "$plain"$'\n'"$push" "depends on the test-only fake $push"
expect_error "the default build linking the fake search index" "" "$plain"$'\n'"$search" "depends on the test-only fake $search"
expect_error "the default build linking the fake translation provider" "" "$plain"$'\n'"$translator" "depends on the test-only fake $translator"
expect_error "the default build linking the fake mailer" "" "$plain"$'\n'"$mail" "depends on the test-only fake $mail"
expect_error "the default build linking the fake OpenID Connect provider" "" "$plain"$'\n'"$oidc" "depends on the test-only fake $oidc"
expect_ok "e2e_fakes with the fakes" "e2e_fakes" "$plain"$'\n'"$fakes"
expect_ok "e2e_fakes among other tags, with the fakes" "no_valkey,e2e_fakes" "$plain"$'\n'"$fakes"
for missing in "$fake" "$push" "$search" "$translator" "$mail" "$oidc"; do
  expect_error "e2e_fakes without $missing" "e2e_fakes" "$plain"$'\n'"$(grep -vFx "$missing" <<<"$fakes")" "does not depend on $missing"
done
expect_error "the default build linking a package of the tests" "" "$plain"$'\n'"$fakeplugin" "which only tests may link"
expect_error "the test build linking a package of the tests" "e2e_fakes" "$plain"$'\n'"$fakes"$'\n'"$fakeplugin" "which only tests may link"

if [ "$failed" -ne 0 ]; then exit 1; fi
echo "go-fakes-test: ok"
