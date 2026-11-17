#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Prints the packages under web/ whose tests the changes since BASE reach, one directory name per line (`app` for
# web/app): each changed package and every package that depends on it, directly or through others (ADR-0079 §18).
# `scripts/task.sh web:test PACKAGES=...` runs them; `scripts/task.sh check:changed` and the `changes` job of
# .github/workflows/ci.yml ask it which.
#
#   scripts/changed-web-packages.sh BASE          changes from the merge base of BASE and HEAD to the working tree,
#                                                 plus untracked files (local use)
#   scripts/changed-web-packages.sh --all         every package web:test runs
#   scripts/changed-web-packages.sh --files FILE  the paths listed in FILE, one per line (CI and tests)
#
# The packages and the dependencies between them are read from the workspace, so a new package or dependency needs no
# edit here:
#   - The packages are the members of the Bun workspace (the root package.json) under web/ whose manifest has a `test`
#     script that runs `bun test`, which web:test runs.
#   - A package depends on the workspace members its manifest names (dependencies, devDependencies, peerDependencies):
#     @my/e2e for a test on the fake API, @my/devtools for a test on a DOM, which its `dom-tests` runs on a preload.
# What no manifest says is said here:
#   - web/docs and web/i18n are no packages of web:test: the site and i18n contexts run their tests (docs:site-test,
#     i18n:test).
#   - A package with catalogs (locales/) depends on @my/i18n, which compiles them into its messages.gen.ts before the
#     tests (i18n:compile).
#   - api/openapi.yaml changes pkg/mykit/ts, whose types are generated from it (scripts/generate.sh).
#
# A changed path selects:
#   - inside a workspace member (pkg/mykit/ts and tools/i18n are members too): that member and what depends on it;
#   - under web/ but in no member (web/brand): every package, with a line on stderr;
#   - anywhere else: every package when the path selects the web context of scripts/changed-contexts.sh (the lockfile,
#     the workspace's configuration, the tasks, this script), and none otherwise.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

usage() {
  echo "usage: scripts/changed-web-packages.sh BASE | --all | --files FILE" >&2
  exit 2
}

all=0
[ "$#" -eq 1 ] || [ "$#" -eq 2 ] || usage
case "$1" in
  --all)
    [ "$#" -eq 1 ] || usage
    all=1
    files=""
    ;;
  --files)
    [ "$#" -eq 2 ] || usage
    files="$(cat "$2")"
    ;;
  -*) usage ;;
  *)
    [ "$#" -eq 1 ] || usage
    base="$(git -C "$root" merge-base "$1" HEAD)"
    files="$(
      git -C "$root" diff --name-only "$base" --
      git -C "$root" ls-files --others --exclude-standard
    )"
    ;;
esac

# The workspace: the directory, whether web:test runs its tests, and the dependents of each member, by name.
declare -A dir_of=() tested=() dependents=()
manifests=()
mapfile -t patterns < <(jq -r '.workspaces[]' "$root/package.json")
for pattern in "${patterns[@]}"; do
  for f in "$root"/$pattern/package.json; do
    [ -f "$f" ] && manifests+=("$f")
  done
done
while IFS=$'\t' read -r f name bun_test deps; do
  f="${f#"$root"/}"
  dir="${f%/package.json}"
  dir_of[$name]="$dir"
  case "$dir" in
    web/docs | web/i18n) ;;
    web/*) [ "$bun_test" = true ] && tested[$name]=1 ;;
  esac
  [ -d "$root/$dir/locales" ] && deps+=" @my/i18n"
  for dep in $deps; do
    dependents[$dep]+=" $name"
  done
done < <(jq -r '[input_filename, .name, (.scripts.test // "" | startswith("bun test")),
  ([.dependencies, .devDependencies, .peerDependencies] | map(select(. != null) | to_entries[]
    | select(.value | startswith("workspace:")) | .key) | join(" "))] | @tsv' "${manifests[@]}")

# member_of PATH sets `member` to the name of the workspace member that holds the path, or to nothing.
member_of() {
  for member in "${!dir_of[@]}"; do
    case "$1" in "${dir_of[$member]}"/*) return ;; esac
  done
  member=""
}

# The changed members, and the paths outside every member, which scripts/changed-contexts.sh knows.
declare -A changed=()
outside=""
while IFS= read -r path; do
  case "$path" in
    "") continue ;;
    # A map file, which `agentkit map check` alone reads.
    context-map.yaml | */context-map.yaml) continue ;;
    # The spec changes the types generated from it.
    api/openapi.yaml) path="pkg/mykit/ts/src/schema.gen.ts" ;;
  esac
  member_of "$path"
  if [ -n "$member" ]; then
    changed[$member]=1
    continue
  fi
  case "$path" in
    web/*)
      echo "changed-web-packages.sh: $path belongs to no package of the workspace: every package" >&2
      all=1
      ;;
  esac
  outside+="$path"$'\n'
done <<< "$files"
if [ "$all" -eq 0 ] && [ -n "$outside" ]; then
  contexts="$("$root/scripts/changed-contexts.sh" --files <(printf '%s' "$outside"))"
  case $'\n'"$contexts"$'\n' in *$'\nweb\n'*) all=1 ;; esac
fi

# Every member that depends on a changed one, directly or through others.
declare -A reached=()
queue=("${!changed[@]}")
while [ "${#queue[@]}" -gt 0 ]; do
  name="${queue[0]}"
  queue=("${queue[@]:1}")
  [ -n "${reached[$name]:-}" ] && continue
  reached[$name]=1
  # shellcheck disable=SC2206 # one word per dependent
  queue+=(${dependents[$name]:-})
done

for name in "${!tested[@]}"; do
  if [ "$all" -eq 1 ] || [ -n "${reached[$name]:-}" ]; then
    echo "${dir_of[$name]#web/}"
  fi
done | sort
