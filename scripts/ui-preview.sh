#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Publishes the UI preview (ADR-0165 §2, §3), the static build of the Storybook host web/storybook, on the Cloudflare
# Worker of scripts/ui-preview/wrangler.jsonc: main's catalog on the Worker, and a pull request's on its Workers
# Preview pr-<number>. ui-preview.yml builds the catalog without secrets (`build`), from main or from the pull request's
# checkout (forks included); ui-preview-deploy.yml publishes it from main's checkout (`deploy`, `deploy-pr`). A pull
# request's build comes from code anyone can write, so it is only ever data: `stage` checks that it is plain static
# files and copies it next to main's Worker config and the _headers file that keeps every page out of search engines,
# and wrangler uploads it as assets. Nothing in it is executed or sourced, and nothing of it reaches a shell, a config,
# or a command line; the comment takes from its index.json only the ids and the titles of plain characters.
#
#   scripts/ui-preview.sh build <out dir>                     build the catalog of this checkout into <out dir>
#   scripts/ui-preview.sh stage <build dir> <work dir>        check a build, copy it to <work dir>/site
#   scripts/ui-preview.sh stories <site dir> <url> <changed files>
#                                                             the Markdown list of the stories the changed files change
#   scripts/ui-preview.sh deploy <build dir>                  publish the build as the Worker (main's catalog)
#   scripts/ui-preview.sh deploy-pr <build dir> <pr> [<changed files>]
#                                                             publish the Preview pr-<pr>; writes <work>/pr/comment.md
#   scripts/ui-preview.sh cleanup                             delete the Previews pr-<number> of closed pull requests
#   scripts/ui-preview.sh comment <pr> <body file>            create or update the preview comment
#   scripts/ui-preview.sh install                             fetch the pinned wrangler into Bun's cache
#
# <changed files> lists the repository paths a pull request changes, one per line. A story file is changed when a
# changed file of web/ has its name up to the first dot in the same directory: web/design-system/src/button.tsx and
# button.stories.tsx change the stories of button.stories.tsx. The comment lists them in the order of the catalog.
#
# Environment: CLOUDFLARE_API_TOKEN and CLOUDFLARE_ACCOUNT_ID (deploy, deploy-pr, and the Cloudflare API of cleanup);
# GH_TOKEN and GITHUB_REPOSITORY (comment, cleanup); UI_PREVIEW_WORK, the work dir (default .tmp/ui-preview); HEAD_SHA,
# the commit the preview comment names. DRY_RUN=1 runs `deploy` and `deploy-pr` as `wrangler deploy --dry-run`, which
# checks the config and the assets without an account, with https://pr-<pr>-my-ui.example.workers.dev as the Preview
# URL, and prints the wrangler and gh commands of deploy-pr, comment, and cleanup instead of running them:
#
#   scripts/ui-preview.sh build .tmp/ui-preview/build
#   DRY_RUN=1 scripts/ui-preview.sh deploy-pr .tmp/ui-preview/build 123 && cat .tmp/ui-preview/pr/comment.md
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
config="$root/scripts/ui-preview/wrangler.jsonc"
# The pinned wrangler, the limits of Workers static assets, and the Previews and the comment of a pull request.
# shellcheck source=scripts/workers-preview.sh
. "$root/scripts/workers-preview.sh"
dry_run="${DRY_RUN:-}"
work="${UI_PREVIEW_WORK:-$root/.tmp/ui-preview}"
marker="<!-- ui-preview -->"
# The most components the preview comment links; the rest are counted.
max_listed=40

die() {
  echo "ui-preview: $*" >&2
  exit 1
}

usage() {
  sed -n '13,22p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 2
}

# build OUT builds the Storybook host of this checkout (web:stories) and copies it to OUT.
build() {
  local out="$1" dist="$root/web/storybook/dist"
  rm -rf "$dist"
  (cd "$root" && scripts/task.sh web:stories)
  [[ -f "$dist/index.html" ]] || die "the build has no index.html"
  rm -rf "$out"
  mkdir -p "$(dirname "$out")"
  cp -R "$dist" "$out"
  check_plain "$out" >/dev/null
  echo "ui-preview: built the catalog in $out ($(find "$out" -type f | wc -l) files)" >&2
}

# stage_catalog BUILD DEST checks that BUILD is a build of plain files with the pages of a Storybook (build) and copies
# it to DEST/site, with the Worker config of this checkout and the _headers file that keeps every page out of search
# engines.
stage_catalog() {
  local build="$1" dest="$2" total file
  total="$(check_plain "$build")"
  for file in index.html iframe.html index.json; do
    [[ -f "$build/$file" ]] || die "the build has no $file: is it the output of scripts/ui-preview.sh build?"
  done
  rm -rf "$dest"
  mkdir -p "$dest/site"
  cp -R "$build/." "$dest/site/"
  cp "$config" "$dest/wrangler.jsonc"
  printf '%s\n' '/*' '  X-Robots-Tag: noindex, nofollow' >"$dest/site/_headers"
  echo "ui-preview: staged the catalog ($total bytes) in $dest/site" >&2
}

# stem PATH prints PATH up to the first dot of its name: web/a/button.stories.tsx is web/a/button.
stem() {
  local dir="${1%/*}" name="${1##*/}"
  echo "$dir/${name%%.*}"
}

# stories SITE URL CHANGED prints the Markdown list of the components of SITE (index.json) whose story file is changed
# by the repository paths of CHANGED, each linked under URL to its first story, in the order of the catalog. Only the
# ids, the import paths and the titles that are plain characters are read from the build.
stories() {
  local site="$1" url="$2" changed="$3" path title id count file label n=0
  local -A stems=()
  local -a listed=()
  [[ -n "$changed" && -f "$changed" ]] || return 0
  jq -e '.entries | type == "object"' "$site/index.json" >/dev/null 2>&1 || die "$site/index.json has no entries"
  while IFS= read -r path; do
    [[ "$path" =~ ^web/[A-Za-z0-9_-][A-Za-z0-9._/-]*$ && "$path" != *..* && "$path" != */./* ]] || continue
    stems[$(stem "$path")]=1
  done <"$changed"
  # One line for each title (a component or a screen), with its import path, its first story, and its number of stories.
  while IFS=$'\t' read -r path title id count; do
    # The import paths are relative to web/storybook.
    [[ "$path" =~ ^\.\./[A-Za-z0-9_-][A-Za-z0-9._/-]*\.stories\.tsx$ ]] || continue
    [[ "$path" != ../*..* && "$path" != */./* ]] || continue
    [[ "$id" =~ ^[a-z0-9][a-z0-9-]*--[a-z0-9][a-z0-9-]*$ && "$count" =~ ^[1-9][0-9]*$ ]] || continue
    file="web/${path#../}"
    [[ -n "${stems[$(stem "$file")]:-}" ]] || continue
    label="$title"
    [[ "$label" =~ ^[A-Za-z0-9][A-Za-z0-9\ ./_-]{0,119}$ ]] || label="$id"
    if ((count == 1)); then
      listed+=("- [$label]($url/?path=/story/$id)")
    else
      listed+=("- [$label]($url/?path=/story/$id) ($count stories)")
    fi
  done < <(jq -r 'reduce (.entries[] | select(.type == "story" and (.title | type) == "string"
      and (.id | type) == "string" and (.importPath | type) == "string")) as $e ({order: [], by: {}};
      if .by[$e.title] then .by[$e.title].n += 1
      else .order += [$e.title] | .by[$e.title] = {path: $e.importPath, id: $e.id, n: 1} end)
    | .order[] as $t | .by[$t] | [.path, $t, .id, (.n | tostring)] | @tsv' "$site/index.json")
  n="${#listed[@]}"
  if ((n == 0)); then
    echo "It changes no story file and no file next to one."
    return 0
  fi
  echo "The components and the screens it changes, in the order of the catalog:"
  echo
  printf '%s\n' "${listed[@]:0:max_listed}"
  ((n <= max_listed)) || printf -- '- and %d more\n' "$((n - max_listed))"
}

# deploy BUILD publishes the build BUILD of main as the Worker.
deploy() {
  local build="$1" stage="$work/deploy" args=(deploy)
  stage_catalog "$build" "$stage"
  [[ -z "$dry_run" ]] || args+=(--dry-run)
  wrangler_in "$stage" "${args[@]}"
}

# deploy_pr BUILD PR [CHANGED] publishes the build BUILD on the Preview pr-<PR> and writes the body of the pull
# request's comment to WORK/pr/comment.md.
deploy_pr() {
  local build="$1" pr="$2" changed="${3:-}" stage="$work/pr" url main sha body
  check_pr "$pr"
  stage_catalog "$build" "$stage"
  url="$(deploy_preview "$stage" "$pr")"
  # main's catalog is the Worker itself, on the same subdomain.
  main="https://${url#https://pr-"$pr"-}"
  sha="${HEAD_SHA:-}"
  [[ -z "$sha" || "$sha" =~ ^[0-9a-f]{40}$ ]] || die "HEAD_SHA is not a commit: $sha"
  body="$stage/comment.md"
  {
    echo "$marker"
    echo "### UI preview"
    echo
    echo "The Storybook of this pull request${sha:+ at $sha}: $url/ (main's: $main/)"
    echo
    stories "$stage/site" "$url" "$changed"
  } >"$body"
  echo "$url"
  echo "ui-preview: wrote $body" >&2
}

# cleanup deletes the Previews pr-<number> of the closed or merged pull requests (delete_closed_previews).
cleanup() {
  local stage="$work/cleanup"
  rm -rf "$stage"
  mkdir -p "$stage/site"
  cp "$config" "$stage/wrangler.jsonc"
  delete_closed_previews "$stage"
}

cmd="${1:-}"
[[ $# -gt 0 ]] && shift
case "$cmd" in
  build) [[ $# -eq 1 ]] || usage; build "$@" ;;
  stage) [[ $# -eq 2 ]] || usage; stage_catalog "$@" ;;
  stories) [[ $# -eq 3 ]] || usage; stories "$@" ;;
  deploy) [[ $# -eq 1 ]] || usage; deploy "$@" ;;
  deploy-pr) [[ $# -ge 2 && $# -le 3 ]] || usage; deploy_pr "$@" ;;
  cleanup) [[ $# -eq 0 ]] || usage; cleanup ;;
  comment) [[ $# -eq 2 ]] || usage; upsert_comment "$@" "$marker" ;;
  # Run before the steps that hold the token, so that the package's install scripts never see it.
  install) [[ $# -eq 0 ]] || usage; wrangler_install ;;
  *) usage ;;
esac
