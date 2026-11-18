#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Keeps the fakes of the test build out of the server (ADR-0015 §6, ADR-0015 §9). The test-only build tag e2e_fakes
# adds fakes that record payments as paid, accept any push message, keep a search index in memory, and translate
# nothing: none of them may reach a released binary, and the test build must have them all.
#
#   scripts/go-fakes.sh                  fail when cmd/my without e2e_fakes depends on a fake or on a package that only
#                                        tests may link, or when cmd/my with e2e_fakes lacks a fake; builds nothing
#                                        (`scripts/task.sh go:fakes-check`)
#   scripts/go-fakes.sh test             go vet on the build with e2e_fakes, and the tests that only it has, named
#                                        *E2EFakes* (`scripts/task.sh go:fakes-test`)
#   scripts/go-fakes.sh check-deps TAGS  check the packages on stdin (`go list -deps ./cmd/my` built with TAGS, a
#                                        comma-separated list) without reading the module; scripts/go-fakes-test.sh
#                                        tests it
set -euo pipefail

# fake_packages prints the packages of the fakes that only a build with e2e_fakes may link into cmd/my.
fake_packages() {
  echo "github.com/mycroblog/my/internal/adapter/payment/fake"
  echo "github.com/mycroblog/my/internal/adapter/pushservice/fake"
  echo "github.com/mycroblog/my/internal/adapter/search/fake"
  echo "github.com/mycroblog/my/internal/adapter/translator/fake"
}

# never_linked_packages prints the packages that only tests link, which no build of cmd/my may depend on.
never_linked_packages() {
  echo "github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
}

# check_deps TAGS reads the packages that cmd/my built with TAGS depends on from stdin and fails, printing why, when
# one of them is a fake while TAGS has no e2e_fakes, when a fake that e2e_fakes adds is missing, or when one of them is
# a package only tests may link.
check_deps() {
  local tags="$1" deps status=0 pkg
  deps="$(cat)"
  for pkg in $(fake_packages); do
    if [[ ",$tags," == *,e2e_fakes,* ]]; then
      if ! grep -qFx "$pkg" <<<"$deps"; then
        echo "cmd/my built with -tags '$tags' does not depend on $pkg, which e2e_fakes adds: wire it in a file of internal/platform with the constraint //go:build e2e_fakes" >&2
        status=1
      fi
    elif grep -qFx "$pkg" <<<"$deps"; then
      echo "cmd/my built with -tags '$tags' depends on the test-only fake $pkg: only a build with e2e_fakes may link it, so import it from a file with the constraint //go:build e2e_fakes alone" >&2
      status=1
    fi
  done
  for pkg in $(never_linked_packages); do
    if grep -qFx "$pkg" <<<"$deps"; then
      echo "cmd/my built with -tags '$tags' depends on $pkg, which only tests may link: import it from _test.go files alone" >&2
      status=1
    fi
  done
  return "$status"
}

case "${1:-}" in
  check-deps)
    check_deps "${2-}"
    ;;
  test)
    go vet -tags e2e_fakes ./...
    mapfile -t pkgs < <(grep -rlE '^//go:build .*e2e_fakes' --include='*_test.go' internal cmd | xargs -n1 dirname | sort -u | sed 's|^|./|')
    go test -tags e2e_fakes -run E2EFakes "${pkgs[@]}"
    ;;
  "")
    status=0
    go list -deps ./cmd/my | check_deps "" || status=1
    go list -deps -tags e2e_fakes ./cmd/my | check_deps "e2e_fakes" || status=1
    if [ "$status" -eq 0 ]; then echo "cmd/my links the fakes with e2e_fakes alone"; fi
    exit "$status"
    ;;
  *)
    sed -n '8,15p' "$0" | sed 's/^# \{0,1\}//' >&2
    exit 2
    ;;
esac
