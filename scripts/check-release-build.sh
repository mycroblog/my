#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Keeps the test builds out of the released binaries and images (ADR-0117 §2; docs/reference/build-tags.md). The build
# tag e2e_fakes adds fakes, among them a payment provider that records payments as paid without any money moving, so
# a binary or image that is published must never be built with it.
#
#   scripts/check-release-build.sh BINARY...   fails when a Go binary's build info (`go version -m`, its
#                                              `build -tags=` line) has e2e_fakes, or cannot be read; needs go
#   scripts/check-release-build.sh tags TAGS   fails when the build tags TAGS (comma- or space-separated, as
#                                              `go build -tags` takes them) have e2e_fakes; the Dockerfile runs it
#
# scripts/check-release-build-test.sh tests it.
set -euo pipefail

test_tag=e2e_fakes

# has_test_tag TAGS reports whether TAGS, comma- or space-separated, have the test tag.
has_test_tag() {
  local tag
  for tag in ${1//,/ }; do
    if [[ "$tag" == "$test_tag" ]]; then return 0; fi
  done
  return 1
}

if (($# == 0)); then
  echo "usage: $0 BINARY... | $0 tags TAGS" >&2
  exit 2
fi

if [[ "$1" == tags ]]; then
  if (($# != 2)); then
    echo "usage: $0 tags TAGS" >&2
    exit 2
  fi
  if has_test_tag "$2"; then
    echo "check-release-build: the build tags '$2' have $test_tag, which only test builds may have; released binaries and images never do" >&2
    exit 1
  fi
  exit 0
fi

status=0
for bin in "$@"; do
  if ! info="$(go version -m "$bin" 2>&1)"; then
    echo "check-release-build: cannot read the Go build info of $bin: $info" >&2
    status=1
    continue
  fi
  tags="$(awk '$1 == "build" && index($2, "-tags=") == 1 { print substr($2, 7) }' <<<"$info")"
  if has_test_tag "$tags"; then
    echo "check-release-build: $bin was built with -tags=$tags, which has $test_tag: a test build must never be released" >&2
    status=1
  fi
done
exit "$status"
