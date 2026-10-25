#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# The steps of .github/workflows/release.yml that release the client libraries with the server (ADR-0093 §8): the
# libraries take the server's version, Go as the tag pkg/<library>/vX.Y.Z and npm as X.Y.Z. It changes local files
# and prints; it never tags, pushes, or publishes (release.yml does that). scripts/release-libraries-test.sh tests it.
#
#   scripts/release-libraries.sh npm-version VERSION DIR...  Set the version of the package.json in each DIR to the
#                                                           npm version of VERSION, and pin every `workspace:`
#                                                           dependency on a package of the release (@mycroblog/*)
#                                                           to that version: npm publishes the file as it is.
#   scripts/release-libraries.sh npm-dist-tag VERSION        Print the npm dist-tag of VERSION: `next` for a
#                                                           pre-release, `latest` for a release.
#   scripts/release-libraries.sh go-tags VERSION             Print the Go tags of the libraries, one per line.
#   scripts/release-libraries.sh notes VERSION               Print the libraries' section of the release notes.
#
# VERSION is the release's version, vX.Y.Z or vX.Y.Z-<pre-release> (release.yml's input).
set -euo pipefail

# The Go modules of the libraries, as pkg/<name>.
GO_LIBRARIES=(pkg/mykit)

die() {
  echo "release-libraries: $*" >&2
  exit 1
}

# npm_version VERSION prints VERSION without its `v`, after checking its form (release.yml's validate job checks the
# same).
npm_version() {
  [[ "$1" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?$ ]] ||
    die "the version must be vX.Y.Z or vX.Y.Z-<pre-release>: $1"
  printf '%s\n' "${1#v}"
}

cmd_npm_version() {
  [ $# -ge 2 ] || die "usage: npm-version VERSION DIR..."
  local version dir file tmp left
  version="$(npm_version "$1")"
  shift
  for dir in "$@"; do
    file="$dir/package.json"
    [ -f "$file" ] || die "no package.json in $dir"
    tmp="$(mktemp)"
    jq --arg v "$version" '
      .version = $v
      | reduce ("dependencies", "peerDependencies", "optionalDependencies", "devDependencies") as $f (.;
          if has($f) then
            .[$f] |= with_entries(
              if (.key | startswith("@mycroblog/")) and (.value | startswith("workspace:")) then .value = $v else . end)
          else . end)
    ' "$file" > "$tmp"
    # A workspace: dependency on any other package would reach the registry as it is, which npm cannot install.
    left="$(jq -r '[(.dependencies, .peerDependencies, .optionalDependencies) // {} | to_entries[]
      | select(.value | startswith("workspace:")) | .key] | join(", ")' "$tmp")"
    if [ -n "$left" ]; then
      rm -f "$tmp"
      die "$file depends on a workspace package outside the release: $left"
    fi
    mv "$tmp" "$file"
  done
}

cmd_npm_dist_tag() {
  [ $# -eq 1 ] || die "usage: npm-dist-tag VERSION"
  local version
  version="$(npm_version "$1")"
  if [[ "$version" == *-* ]]; then echo next; else echo latest; fi
}

cmd_go_tags() {
  [ $# -eq 1 ] || die "usage: go-tags VERSION"
  npm_version "$1" > /dev/null
  local library
  for library in "${GO_LIBRARIES[@]}"; do
    printf '%s/%s\n' "$library" "$1"
  done
}

cmd_notes() {
  [ $# -eq 1 ] || die "usage: notes VERSION"
  local version tag
  version="$(npm_version "$1")"
  tag="$(cmd_npm_dist_tag "$1")"
  cat << EOF
## mykit

The client library of the client API, released with this version of the server
([pkg/mykit](https://github.com/mycroblog/my/tree/pkg/mykit/$1/pkg/mykit), MIT).

- Go: \`go get github.com/mycroblog/my/pkg/mykit@$1\`
- npm: \`npm install @mycroblog/mykit@$version\` (dist-tag \`$tag\`, with provenance)

<!-- The breaking changes of the hand-written layer (the commits marked with \`!\`) and the deprecations of the generated client (ADR-0093 §8), or "None". -->
EOF
}

[ $# -ge 1 ] || die "usage: $0 npm-version|npm-dist-tag|go-tags|notes ..."
command="$1"
shift
case "$command" in
  npm-version) cmd_npm_version "$@" ;;
  npm-dist-tag) cmd_npm_dist_tag "$@" ;;
  go-tags) cmd_go_tags "$@" ;;
  notes) cmd_notes "$@" ;;
  *) die "unknown command: $command" ;;
esac
