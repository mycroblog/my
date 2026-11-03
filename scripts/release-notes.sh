#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Prints the starting draft of a release's notes (docs/operations/releasing.md, "Release notes"), for
# .github/workflows/release.yml: Upgrading, Breaking changes, API changes, mykit, and mybot, from the repository's
# state at HEAD and at the previous release. The decider still reviews it and publishes; HTML comments in the draft
# say what a script cannot fill. It reads the repository of the current directory, changes nothing, and needs Go
# (tools/agentkit/cmd/releasenotes does the reading). scripts/release-notes-test.sh tests it.
#
#   scripts/release-notes.sh draft VERSION [PREVIOUS]   Print the draft. PREVIOUS is the tag to compare with, as
#                                                       release.yml's input of that name: pre-releases only.
#   scripts/release-notes.sh skeleton VERSION           Print the sections with a request to write them by hand,
#                                                       for when the draft fails after the release's tags exist.
#
# The previous release is the one the upgrade test starts from: scripts/previous-minor.sh, which ignores pre-releases
# and the libraries' tags. A patch of the first minor version has none, so it is compared with the latest release.
# Without any release, the draft is that of a first release. RELEASE_NOTES_TODAY (YYYY-MM-DD) replaces today's date,
# which tells the Sunset dates that have passed.
set -euo pipefail

scripts="$(cd "$(dirname "$0")" && pwd)"
root="$(dirname "$scripts")"

# The files that tools/agentkit/cmd/releasenotes reads, at each release.
FILES=(internal/platform/setup_dbupgrade.go internal/platform/config/retired.go docs/reference/configuration.md api/openapi.yaml)

work=""
trap '[ -z "$work" ] || rm -rf "$work"' EXIT

die() {
  echo "release-notes: $*" >&2
  exit 1
}

# extract REF DIR copies the FILES of REF that exist to DIR.
extract() {
  local file
  for file in "${FILES[@]}"; do
    if git cat-file -e "$1:$file" 2> /dev/null; then
      mkdir -p "$2/$(dirname "$file")"
      git show "$1:$file" > "$2/$file"
    fi
  done
}

# breaking_commits PREVIOUS prints each commit marked ! after PREVIOUS: <sha> TAB <subject> TAB <paths, by commas>.
breaking_commits() {
  local sha subject marked='^[a-z]+(\([^)]*\))?!: '
  while IFS=$'\t' read -r sha subject; do
    if [[ "$subject" =~ $marked ]]; then
      printf '%s\t%s\t%s\n' "$sha" "$subject" "$(git diff-tree --no-commit-id --name-only -r "$sha" | paste -sd ,)"
    fi
  done < <(git log --no-merges --format='%h%x09%s' "$1..HEAD")
}

cmd_draft() {
  [ $# -eq 1 ] || [ $# -eq 2 ] || die "usage: draft VERSION [PREVIOUS]"
  local version="$1" previous="${2:-}"
  "$scripts/release-libraries.sh" npm-dist-tag "$version" > /dev/null
  if [ -n "$previous" ]; then
    [[ "$version" == *-* ]] || die "PREVIOUS is for pre-releases only: $version"
    if ! [[ "$previous" =~ ^v[0-9] ]] || ! git rev-parse -q --verify "refs/tags/$previous" > /dev/null; then
      die "PREVIOUS must be a release tag (v...): $previous"
    fi
  else
    previous="$(git tag -l 'v[0-9]*' | "$scripts/previous-minor.sh" "$version")"
    if [ -z "$previous" ]; then
      previous="$(git tag -l 'v[0-9]*' | "$scripts/previous-minor.sh")"
    fi
  fi

  work="$(mktemp -d)"
  mkdir -p "$work/old" "$work/new" "$work/out"
  extract HEAD "$work/new"
  : > "$work/commits"
  : > "$work/schema"
  if [ -n "$previous" ]; then
    extract "$previous" "$work/old"
    breaking_commits "$previous" > "$work/commits"
    git diff --name-only "$previous" HEAD -- 'schema/*/*.sql' > "$work/schema"
  fi

  (cd "$root/tools/agentkit" && GOWORK=off CGO_ENABLED=0 go run ./cmd/releasenotes \
    -version "$version" -previous "$previous" -today "${RELEASE_NOTES_TODAY:-$(date -u +%F)}" \
    -old "$work/old" -new "$work/new" -commits "$work/commits" -schema "$work/schema" -out "$work/out")

  cat "$work/out/sections.md"
  "$scripts/release-libraries.sh" notes "$version" "$work/out/mykit.md" "$work/out/mybot.md"
}

cmd_skeleton() {
  [ $# -eq 1 ] || die "usage: skeleton VERSION"
  local section
  for section in Upgrading "Breaking changes" "API changes"; do
    printf '## %s\n\n<!-- scripts/release-notes.sh failed, so this draft has no generated text (the log of the release job says why). Write this section by hand: docs/operations/releasing.md, "Release notes". -->\n\n' "$section"
  done
  "$scripts/release-libraries.sh" notes "$1"
}

[ $# -ge 1 ] || die "usage: $0 draft|skeleton ..."
command="$1"
shift
case "$command" in
  draft) cmd_draft "$@" ;;
  skeleton) cmd_skeleton "$@" ;;
  *) die "unknown command: $command" ;;
esac
