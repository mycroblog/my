#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/go-notices.sh with a fake go-licenses (GO_LICENSES) that prints a given report: it writes each module
# once with its licenses and text after the standard library's, leaves the server's own module out, and fails without
# writing on a module whose license or text is unknown. Then it checks that the Dockerfile runs it and puts its output
# and LICENSE into the image. Needs go.
#
#   scripts/go-notices-test.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/scripts/go-notices.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

failed=0
fail() {
  echo "FAIL: $*" >&2
  failed=1
}

printf 'MIT License\n\nCopyright (c) a\n' > "$work/LICENSE-a"
printf 'BSD and MIT text of b\n' > "$work/LICENSE-b"

# fake REPORT writes a go-licenses that prints REPORT, with the fixture's paths for @a and @b.
fake() {
  local report="${1//@a/$work/LICENSE-a}"
  report="${report//@b/$work/LICENSE-b}"
  printf '%s\n' "$report" > "$work/report.tsv"
  printf '#!/usr/bin/env bash\ncat %q\n' "$work/report.tsv" > "$work/go-licenses"
  chmod +x "$work/go-licenses"
}

# run OUT runs the script from the repository root with the fake.
run() {
  (cd "$root" && GO_LICENSES="$work/go-licenses" "$script" "$1") 2> "$work/stderr"
}

tab=$'\t'
fake "github.com/mycroblog/my${tab}Unknown${tab}AGPL-3.0${tab}$root/LICENSE
example.test/a${tab}v1.0.0${tab}MIT${tab}@a
example.test/b${tab}v2.1.0${tab}BSD-3-Clause${tab}@b
example.test/b${tab}v2.1.0${tab}MIT${tab}@b"
if run "$work/ok/go-modules.md"; then
  out="$work/ok/go-modules.md"
  if ! grep -q '^## Go standard library - go[0-9.]* (BSD-3-Clause)$' "$out"; then fail "no heading of the standard library"; fi
  if ! grep -q '^## example.test/a - v1.0.0 (MIT)$' "$out"; then fail "no heading of a"; fi
  if ! grep -q 'Copyright (c) a' "$out"; then fail "no text of a"; fi
  if [[ "$(grep -c '^## example.test/b ' "$out")" != 1 ]]; then fail "b, with two licenses, is not written once"; fi
  if ! grep -q '^## example.test/b - v2.1.0 (BSD-3-Clause, MIT)$' "$out"; then fail "no heading of b with both licenses"; fi
  if grep -q 'mycroblog/my' "$out"; then fail "the server's own module is listed"; fi
else
  fail "a report of known licenses: want it accepted, got: $(cat "$work/stderr")"
fi

fake "example.test/a${tab}v1.0.0${tab}MIT${tab}@a
example.test/c${tab}v0.1.0${tab}Unknown${tab}@a"
if run "$work/unknown/go-modules.md"; then fail "an unknown license: want an error"; fi
if ! grep -q 'example.test/c cannot be identified' "$work/stderr"; then fail "an unknown license: the error does not name the module"; fi
if [[ -e "$work/unknown/go-modules.md" ]]; then fail "an unknown license: the notices were written"; fi

fake "example.test/d${tab}v0.1.0${tab}MIT${tab}Unknown"
if run "$work/notext/go-modules.md"; then fail "a license with no file: want an error"; fi

fake ""
if run "$work/empty/go-modules.md"; then fail "an empty report: want an error"; fi

if (cd "$root" && "$script") > /dev/null 2>&1; then fail "no arguments: want an error"; fi

# The Dockerfile writes the notices in the build stage and puts them, the npm notices and LICENSE into the image.
dockerfile="$root/Dockerfile"
if ! grep -q 'scripts/go-notices.sh' "$dockerfile"; then fail "the Dockerfile does not run scripts/go-notices.sh"; fi
if ! grep -q '^COPY LICENSE /usr/share/doc/my/LICENSE$' "$dockerfile"; then fail "the image lacks LICENSE"; fi
if ! grep -q '^COPY --from=build /out/licenses /usr/share/doc/my/licenses$' "$dockerfile"; then
  fail "the image lacks the notices"
fi

if ((failed)); then exit 1; fi
echo "go-notices: ok"
