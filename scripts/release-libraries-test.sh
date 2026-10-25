#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/release-libraries.sh on copies of the libraries' package.json in a temporary directory; it changes
# no file of the repository.
#
#   scripts/release-libraries-test.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/scripts/release-libraries.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

failed=0
fail() {
  echo "FAIL: $*" >&2
  failed=1
}

# expect_eq NAME WANT GOT
expect_eq() {
  [ "$2" = "$3" ] || fail "$1: want '$2', got '$3'"
}

# expect_error NAME COMMAND... passes when COMMAND fails.
expect_error() {
  local name="$1"
  shift
  if "$@" > /dev/null 2>&1; then fail "$name: want an error"; fi
}

# The versions: the form release.yml takes, and the dist-tag of each.
expect_eq "dist-tag of a release" latest "$("$script" npm-dist-tag v0.1.0)"
expect_eq "dist-tag of a pre-release" next "$("$script" npm-dist-tag v0.1.0-rc.1)"
for bad in 0.1.0 v0.1 v01.0.0 v0.1.0- v0.1.0-rc..1 "v0.1.0 "; do
  expect_error "version '$bad'" "$script" npm-dist-tag "$bad"
done
expect_error "unknown command" "$script" publish v0.1.0

# The Go tags.
expect_eq "go-tags" "pkg/mykit/v0.1.0-rc.1" "$("$script" go-tags v0.1.0-rc.1)"

# npm-version on a copy of @mycroblog/mykit's package.json.
mkdir -p "$work/mykit"
cp "$root/pkg/mykit/ts/package.json" "$work/mykit/"
"$script" npm-version v0.1.0-rc.1 "$work/mykit"
expect_eq "mykit version" 0.1.0-rc.1 "$(jq -r .version "$work/mykit/package.json")"
expect_eq "mykit dependencies" "$(jq -S .dependencies "$root/pkg/mykit/ts/package.json")" \
  "$(jq -S .dependencies "$work/mykit/package.json")"
expect_eq "mykit other fields" "$(jq -S 'del(.version)' "$root/pkg/mykit/ts/package.json")" \
  "$(jq -S 'del(.version)' "$work/mykit/package.json")"

# A workspace dependency on a package of the release is pinned; one on another package fails and leaves the file.
mkdir -p "$work/pin" "$work/other"
cat > "$work/pin/package.json" << 'EOF'
{"name": "@mycroblog/x", "version": "0.0.0", "dependencies": {"@mycroblog/mykit": "workspace:*", "croner": "10.0.1"},
 "devDependencies": {"@mycroblog/y": "workspace:^"}}
EOF
"$script" npm-version v1.2.3 "$work/pin"
expect_eq "pinned dependency" 1.2.3 "$(jq -r '.dependencies["@mycroblog/mykit"]' "$work/pin/package.json")"
expect_eq "pinned dev dependency" 1.2.3 "$(jq -r '.devDependencies["@mycroblog/y"]' "$work/pin/package.json")"
expect_eq "other dependency" 10.0.1 "$(jq -r '.dependencies.croner' "$work/pin/package.json")"
cat > "$work/other/package.json" << 'EOF'
{"name": "@mycroblog/x", "version": "0.0.0", "dependencies": {"@my/ui": "workspace:*"}}
EOF
before="$(cat "$work/other/package.json")"
expect_error "workspace dependency outside the release" "$script" npm-version v1.2.3 "$work/other"
expect_eq "file left as it was" "$before" "$(cat "$work/other/package.json")"
expect_error "no package.json" "$script" npm-version v1.2.3 "$work/none"

# The release notes name the tag and the npm version.
notes="$("$script" notes v0.1.0-rc.1)"
for want in "## mykit" "go get github.com/mycroblog/my/pkg/mykit@v0.1.0-rc.1" "npm install @mycroblog/mykit@0.1.0-rc.1" \
  "dist-tag \`next\`"; do
  [[ "$notes" == *"$want"* ]] || fail "notes: no '$want'"
done

if [ "$failed" -ne 0 ]; then exit 1; fi
echo "release-libraries: ok"
