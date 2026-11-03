#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Writes the draft of a release's notes (docs/agents/releasing.md, "Release notes") for
# .github/workflows/release.yml. The facts (Upgrading, Breaking changes, API changes, mykit, and mybot) come from the
# repository's state at HEAD and at the previous release, read by tools/agentkit/cmd/releasenotes, and never from a
# model. The prose comes from Claude, in tools/agentkit/cmd/releaseprose, and joins the facts only when a program
# accepts it. The decider reads the draft and publishes it. The commands need Go; `prs` also needs gh, and `prose`
# Claude Code. They change nothing but DIR. scripts/release-notes-test.sh tests them.
#
#   scripts/release-notes.sh draft VERSION [PREVIOUS]     Print the facts-only draft. PREVIOUS is the tag to compare
#                                                         with, as release.yml's input of that name: pre-releases only.
#   scripts/release-notes.sh facts VERSION PREVIOUS DIR   Write that draft to DIR/body.md, with what the steps below
#                                                         read. PREVIOUS may be empty.
#   scripts/release-notes.sh skeleton VERSION [PREVIOUS]  Print the sections with a request to write them by hand,
#                                                         for when the facts fail after the release's tags exist.
#   scripts/release-notes.sh prs DIR                      Write the PRs merged since DIR's previous release and the
#                                                         closed issues to DIR, from git and from GitHub.
#   scripts/release-notes.sh prompt VERSION DIR           Print what the model would get, and call nothing.
#   scripts/release-notes.sh prose VERSION DIR            Replace DIR/body.md with the prose and the facts, or the facts
#                                                         alone when no attempt passes the check. A failed call is not
#                                                         a failure of this command.
#
# The previous release is the one the upgrade test starts from: scripts/previous-minor.sh, which ignores pre-releases
# and the libraries' tags. A patch of the first minor version has none, so it is compared with the latest release.
# Without any release, the draft is that of a first release. RELEASE_NOTES_TODAY (YYYY-MM-DD) replaces today's date,
# which tells the Sunset dates that have passed. RELEASE_NOTES_BODY_LIMIT replaces GitHub's limit on a Release's body.
set -euo pipefail

scripts="$(cd "$(dirname "$0")" && pwd)"
root="$(dirname "$scripts")"

# The files that tools/agentkit/cmd/releasenotes reads, at each release.
FILES=(internal/platform/setup_dbupgrade.go internal/platform/config/retired.go docs/reference/configuration.md api/openapi.yaml)

work=""
printed=""
trap '[ -z "$work" ] || rm -rf "$work"; [ -z "$printed" ] || rm -rf "$printed"' EXIT

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

# A Release's body takes at most 125,000 characters. The count is in bytes here, which are never fewer than characters.
BODY_LIMIT="${RELEASE_NOTES_BODY_LIMIT:-125000}"

# The first line of a body with the facts alone: tools/agentkit/cmd/releaseprose writes the same words, and release.yml
# replaces a draft's body only while it has them.
FACTS_ONLY='<!-- release-notes: facts only.'

# pick_previous VERSION prints the release to compare with, or nothing for the first release.
pick_previous() {
  local previous
  previous="$(git tag -l 'v[0-9]*' | "$scripts/previous-minor.sh" "$1")"
  if [ -z "$previous" ]; then
    previous="$(git tag -l 'v[0-9]*' | "$scripts/previous-minor.sh")"
  fi
  printf '%s' "$previous"
}

# changelog VERSION PREVIOUS prints the link that stands for the list of changes.
changelog() {
  if [ -n "$2" ]; then
    printf '**Full Changelog**: https://github.com/mycroblog/my/compare/%s...%s\n' "$2" "$1"
  else
    printf '**Full Changelog**: https://github.com/mycroblog/my/commits/%s\n' "$1"
  fi
}

cmd_facts() {
  [ $# -eq 3 ] || die "usage: facts VERSION PREVIOUS DIR"
  local version="$1" previous="$2" dir="$3"
  "$scripts/release-libraries.sh" npm-dist-tag "$version" > /dev/null
  if [ -n "$previous" ]; then
    [[ "$version" == *-* ]] || die "PREVIOUS is for pre-releases only: $version"
    if ! [[ "$previous" =~ ^v[0-9] ]] || ! git rev-parse -q --verify "refs/tags/$previous" > /dev/null; then
      die "PREVIOUS must be a release tag (v...): $previous"
    fi
  else
    previous="$(pick_previous "$version")"
  fi

  work="$(mktemp -d)"
  mkdir -p "$work/old" "$work/new" "$work/out" "$dir"
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

  {
    cat "$work/out/sections.md"
    "$scripts/release-libraries.sh" notes "$version" "$work/out/mykit.md" "$work/out/mybot.md"
  } > "$dir/facts.md"
  cp "$work/out/required.json" "$dir/required.json"
  printf '%s\n' "$previous" > "$dir/previous"
  changelog "$version" "$previous" > "$dir/changelog.md"
  {
    printf '%s The prose has not joined the facts yet, or could not: the log of the job release-notes-prose says why. -->\n\n' "$FACTS_ONLY"
    cat "$dir/facts.md"
    printf '\n'
    cat "$dir/changelog.md"
  } > "$dir/body.md"
  [ "$(wc -c < "$dir/body.md")" -le "$BODY_LIMIT" ] || die "the facts are over the limit of $BODY_LIMIT"
}

cmd_draft() {
  [ $# -eq 1 ] || [ $# -eq 2 ] || die "usage: draft VERSION [PREVIOUS]"
  printed="$(mktemp -d)"
  cmd_facts "$1" "${2:-}" "$printed"
  cat "$printed/body.md"
}

# merged_prs PREVIOUS prints the numbers of the PRs merged on the first-parent line since PREVIOUS (all of them without
# it): the merge queue's merge commits, and the squash merges of the early history.
merged_prs() {
  local range=HEAD
  if [ -n "$1" ]; then range="$1..HEAD"; fi
  git log --first-parent --format=%s "$range" |
    sed -nE 's/^Merge pull request #([0-9]+) .*/\1/p; s/.*\(#([0-9]+)\)$/\1/p'
}

cmd_prs() {
  [ $# -eq 1 ] || die "usage: prs DIR"
  local dir="$1" previous since=""
  previous="$(cat "$dir/previous")"
  if [ -n "$previous" ]; then since="$(git log -1 --format=%cI "$previous")"; fi
  printf '%s\n' "$since" > "$dir/since"
  merged_prs "$previous" > "$dir/pr-numbers"
  # The search API stops at 1,000 results, so these list everything and releaseprose keeps what belongs to the release.
  gh pr list --state merged --limit 1000000 --json number,title,body > "$dir/prs.json"
  gh issue list --state closed --limit 1000000 --json number,title,closedAt,stateReason > "$dir/issues.json"
}

releaseprose() {
  (cd "$root/tools/agentkit" && GOWORK=off CGO_ENABLED=0 go run ./cmd/releaseprose "$@")
}

cmd_prompt() {
  [ $# -eq 2 ] || die "usage: prompt VERSION DIR"
  releaseprose -version "$1" -dir "$(cd "$2" && pwd)" -dry-run
}

cmd_prose() {
  [ $# -eq 2 ] || die "usage: prose VERSION DIR"
  releaseprose -version "$1" -dir "$(cd "$2" && pwd)"
}

cmd_skeleton() {
  [ $# -eq 1 ] || [ $# -eq 2 ] || die "usage: skeleton VERSION [PREVIOUS]"
  local section previous="${2:-}"
  [ -n "$previous" ] || previous="$(pick_previous "$1" 2> /dev/null || true)"
  for section in Upgrading "Breaking changes" "API changes"; do
    printf '## %s\n\n<!-- scripts/release-notes.sh failed, so this draft has no generated text (the log of the release job says why). Write this section by hand: docs/agents/releasing.md, "Release notes". -->\n\n' "$section"
  done
  "$scripts/release-libraries.sh" notes "$1"
  printf '\n'
  changelog "$1" "$previous"
}

[ $# -ge 1 ] || die "usage: $0 draft|facts|skeleton|prs|prompt|prose ..."
command="$1"
shift
case "$command" in
  draft) cmd_draft "$@" ;;
  skeleton) cmd_skeleton "$@" ;;
  facts) cmd_facts "$@" ;;
  prs) cmd_prs "$@" ;;
  prompt) cmd_prompt "$@" ;;
  prose) cmd_prose "$@" ;;
  *) die "unknown command: $command" ;;
esac
