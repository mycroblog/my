#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/check-release-build.sh: it rejects a Go binary built with the test-only tag e2e_fakes and accepts one
# built without it (two small programs built in a temporary directory), checks the tags that the Dockerfile passes,
# and that the Dockerfile runs it on GO_TAGS before it builds the server and on the binary it built. Needs go.
#
#   scripts/check-release-build-test.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/scripts/check-release-build.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

failed=0
fail() {
  echo "FAIL: $*" >&2
  failed=1
}

# expect_ok NAME COMMAND... passes when COMMAND succeeds.
expect_ok() {
  local name="$1"
  shift
  if ! "$@" > /dev/null 2>&1; then fail "$name: want it accepted"; fi
}

# expect_error NAME COMMAND... passes when COMMAND fails.
expect_error() {
  local name="$1"
  shift
  if "$@" > /dev/null 2>&1; then fail "$name: want an error"; fi
}

# The tags of the Dockerfile's GO_TAGS.
expect_ok "no tags" "$script" tags ""
expect_ok "tags that leave adapters out" "$script" tags "no_postgres,no_valkey"
expect_ok "a tag that only resembles it" "$script" tags "no_e2e_fakes_x"
expect_error "e2e_fakes" "$script" tags "e2e_fakes"
expect_error "e2e_fakes among others" "$script" tags "no_postgres,e2e_fakes,no_valkey"
expect_error "e2e_fakes among space-separated tags" "$script" tags "no_postgres e2e_fakes"
expect_error "no arguments" "$script"

# The build info of real binaries. The go of this repository's toolchain builds them, whatever the temporary
# directory's version manager would pick.
gobin="$(cd "$root" && go env GOROOT)/bin/go"
cat > "$work/go.mod" <<'EOF'
module example.test/releasecheck

go 1.24
EOF
cat > "$work/main.go" <<'EOF'
package main

func main() {}
EOF
(cd "$work" && CGO_ENABLED=0 GOFLAGS=-mod=mod GOTOOLCHAIN=local "$gobin" build -trimpath -o released .)
(cd "$work" && CGO_ENABLED=0 GOFLAGS=-mod=mod GOTOOLCHAIN=local "$gobin" build -trimpath -tags no_valkey -o tagged .)
(cd "$work" && CGO_ENABLED=0 GOFLAGS=-mod=mod GOTOOLCHAIN=local "$gobin" build -trimpath -tags no_valkey,e2e_fakes -o test-build .)
expect_ok "a binary without tags" "$script" "$work/released"
expect_ok "a binary with tags that leave adapters out" "$script" "$work/tagged"
expect_error "a binary built with e2e_fakes" "$script" "$work/test-build"
expect_error "a test build among released binaries" "$script" "$work/released" "$work/test-build"
expect_error "a file that is not a Go binary" "$script" "$work/main.go"
expect_error "a missing file" "$script" "$work/missing"

# The Dockerfile checks GO_TAGS before the server's build and the binary after it.
# shellcheck disable=SC2016 # the patterns match the Dockerfile's own ${GO_TAGS}
check_line="$(grep -nF 'check-release-build.sh tags "${GO_TAGS}"' "$root/Dockerfile" | head -1 | cut -d: -f1)"
# shellcheck disable=SC2016
build_line="$(grep -n 'go build .*-tags "\${GO_TAGS}"' "$root/Dockerfile" | head -1 | cut -d: -f1)"
if [[ -z "$check_line" || -z "$build_line" ]] || ((check_line > build_line)); then
  fail "the Dockerfile must run check-release-build.sh on GO_TAGS before it builds the server"
fi
if ! grep -q 'check-release-build.sh /out/my' "$root/Dockerfile"; then
  fail "the Dockerfile must run check-release-build.sh on the binary it built"
fi

if [ "$failed" -ne 0 ]; then exit 1; fi
echo "check-release-build-test: ok"
