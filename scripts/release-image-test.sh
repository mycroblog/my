#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests the commands of scripts/release-image.sh that need no registry: the tags of a version and the metadata.
#
#   scripts/release-image-test.sh
set -euo pipefail

script="$(cd "$(dirname "$0")" && pwd)/release-image.sh"

failed=0
fail() {
  echo "FAIL: $*" >&2
  failed=1
}
# expect_tags NAME WANT EXISTING ARGS...: WANT and EXISTING are space-separated lists.
expect_tags() {
  local name="$1" want="$2" existing="$3" got
  shift 3
  got="$(tr ' ' '\n' <<<"$existing" | "$script" tags "$@" | tr '\n' ' ')"
  [[ "${got% }" == "$want" ]] || fail "$name: want '$want', got '${got% }'"
}

existing="0.0.0-test.1 0.1 0.1.0 0.1.1 0.1.10 0.2 0.2.0 0.3.0-rc.1 sha256-abc"
expect_tags "the first release" "0.1.0 0.1" "" v0.1.0
expect_tags "the next patch moves X.Y" "0.1.11 0.1" "$existing" v0.1.11
expect_tags "the next minor version" "0.3.0 0.3" "$existing" v0.3.0
expect_tags "a patch of an older minor version moves only its X.Y" "0.1.11 0.1" "$existing 0.3.0 0.3" v0.1.11
expect_tags "a patch that is not the latest keeps X.Y" "0.1.2" "$existing" v0.1.2
expect_tags "tagging the same version again" "0.2.0 0.2" "$existing" v0.2.0
expect_tags "a pre-release has its own tag alone" "0.3.0-rc.2" "$existing" v0.3.0-rc.2
expect_tags "patches compare as numbers" "0.1.9" "$existing" v0.1.9
expect_tags "another major version does not count" "0.1.11 0.1" "1.1.20" v0.1.11
expect_tags "a pre-release of a later patch does not count" "0.1.11 0.1" "0.1.12-rc.1" v0.1.11
expect_tags "rolling back moves X.Y to an earlier patch" "0.1.2 0.1" "$existing" v0.1.2 --roll-back

for args in "0.1.0" "v0.1" "v0.1.0-rc.1 --roll-back" "v0.1.0 --other"; do
  # shellcheck disable=SC2086 # The arguments are split on purpose.
  if "$script" tags $args </dev/null >/dev/null 2>&1; then
    fail "tags $args: want an error"
  fi
done

got="$(printf '%s\n' 0.1.0 0.1 0.9 0.10 0.10.1 0.2-rc latest | "$script" series | tr '\n' ' ')"
[[ "$got" == "0.10 0.9 " ]] || fail "series: want the two latest minor versions, got '$got'"
got="$("$script" series </dev/null)"
[[ -z "$got" ]] || fail "series without images: got '$got'"

meta="$("$script" metadata v0.1.0-rc.1 0123abc 2026-11-03T00:00:00+00:00)"
for want in \
  org.opencontainers.image.source=https://github.com/mycroblog/my \
  org.opencontainers.image.documentation=https://github.com/mycroblog/my/blob/v0.1.0-rc.1/README.md \
  org.opencontainers.image.version=0.1.0-rc.1 \
  org.opencontainers.image.revision=0123abc \
  org.opencontainers.image.created=2026-11-03T00:00:00+00:00 \
  org.opencontainers.image.licenses=AGPL-3.0-only \
  org.opencontainers.image.base.name=gcr.io/distroless/static-debian13:nonroot; do
  grep -qxF "$want" <<<"$meta" || fail "metadata: no line $want in: $meta"
done
grep -qE '^org\.opencontainers\.image\.base\.digest=sha256:[0-9a-f]{64}$' <<<"$meta" || fail "metadata: no base digest in: $meta"
description="$(sed -n 's/^org\.opencontainers\.image\.description=//p' <<<"$meta")"
[[ -n "$description" && ${#description} -le 512 ]] || fail "metadata: GHCR reads a description of up to 512 characters: $description"

if [[ "$failed" -ne 0 ]]; then
  exit 1
fi
echo "release-image-test: ok"
