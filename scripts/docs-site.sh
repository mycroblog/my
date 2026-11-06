#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Publishes the documentation site (ADR-0150 §6, §7) on the Cloudflare Worker of scripts/docs-site/wrangler.jsonc.
# docs-site-deploy.yml runs it from main's checkout. One deploy replaces every asset of the Worker, so each deploy
# builds every published version and combines them:
#
#   /next/      main (the checkout this script runs in)
#   /vX.Y/      each minor version of app/docs/versions.json that has a published stable release, from the tag of its
#               newest one
#   /latest/    the newest of those, from the same tag
#   /           the landing page and the 404 page of /latest/ (of /next/ before the first release, when /latest/
#               redirects to /next/)
#
# A pull request gets /next/ alone on the Workers Preview pr-<number> of the same Worker: docs-site-preview.yml builds
# it from the pull request's checkout without secrets (build-preview), and docs-site-preview-deploy.yml publishes it
# from main's checkout (deploy-pr). That build comes from code anyone can write (forks included), so it is only ever
# data: `stage-preview` checks that it is plain static files and copies it next to main's Worker config and redirects,
# and wrangler uploads it as assets. Nothing in it is executed or sourced, and nothing of it reaches a shell, a config,
# or a command line.
#
#   scripts/docs-site.sh resolve <versions.json> [<releases>]   the resolved list of versions (JSON) on stdout
#   scripts/docs-site.sh build <out dir>                        build every version and combine them in <out dir>
#   scripts/docs-site.sh deploy <site dir>                      publish the combined site as the Worker
#   scripts/docs-site.sh build-preview <out dir>                build /next/ of this checkout alone into <out dir>/next
#   scripts/docs-site.sh stage-preview <build dir> <work dir>   check a preview build, copy it to <work dir>/site
#   scripts/docs-site.sh pages <site dir> <url> <changed files> the Markdown list of the pages the changed files make
#   scripts/docs-site.sh deploy-pr <build dir> <pr> [<changed files>]
#                                                               publish the Preview pr-<pr>; writes <work>/pr/comment.md
#   scripts/docs-site.sh cleanup                                delete the Previews pr-<number> of closed pull requests
#   scripts/docs-site.sh comment <pr> <body file>               create or update the preview comment
#   scripts/docs-site.sh install                                fetch the pinned wrangler into the npm cache
#
# <releases> is a file with the tag of each published release, one per line; without it `resolve` reads them from the
# GitHub API (GH_TOKEN, GITHUB_REPOSITORY). Drafts and prereleases never count. A minor version listed in versions.json
# with no release yet is left out; a stable release of a minor version versions.json does not list fails, since
# releasing.md adds the entry before the release. <changed files> lists the repository paths a pull request changes,
# one per line; the comment links the pages of the published documents among them.
#
# Environment: CLOUDFLARE_API_TOKEN and CLOUDFLARE_ACCOUNT_ID (deploy, deploy-pr, and the Cloudflare API of cleanup);
# GH_TOKEN and GITHUB_REPOSITORY (build, resolve without <releases>, comment, cleanup); DOCS_SITE_WORK, the work dir
# (default .tmp/docs-site); DOCS_SITE_RELEASES, a <releases> file for `build` instead of the API; HEAD_SHA, the commit
# the preview comment names. DRY_RUN=1 runs `deploy` and `deploy-pr` as `wrangler deploy --dry-run`, which checks the
# config and the assets without an account, with https://pr-<pr>-my-docs.example.workers.dev as the Preview URL, and
# prints the wrangler and gh commands of deploy-pr, comment, and cleanup instead of running them (cleanup still lists
# the Previews and reads the pull requests, both read-only):
#
#   git tag --list 'v*' > .tmp/releases   # or an empty file before the first release
#   DOCS_SITE_RELEASES=.tmp/releases scripts/docs-site.sh build .tmp/docs-site/site
#   DRY_RUN=1 scripts/docs-site.sh deploy .tmp/docs-site/site
#   scripts/docs-site.sh build-preview .tmp/docs-site/preview
#   DRY_RUN=1 scripts/docs-site.sh deploy-pr .tmp/docs-site/preview 123 && cat .tmp/docs-site/pr/comment.md
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
config="$root/scripts/docs-site/wrangler.jsonc"
# The same wrangler as scripts/ui-preview.sh; fetched by its exact version, since only this CI job runs it.
wrangler_version="4.151.0"
dry_run="${DRY_RUN:-}"
work="${DOCS_SITE_WORK:-$root/.tmp/docs-site}"
marker="<!-- docs-site-preview -->"

# The limits of Workers static assets on the free plan, and a bound on the upload of a preview.
max_files=20000
max_file_bytes=$((25 * 1024 * 1024))
max_preview_bytes=$((300 * 1024 * 1024))
# The most pages the preview comment links; the rest are counted.
max_listed=40

die() {
  echo "docs-site: $*" >&2
  exit 1
}

usage() {
  sed -n '22,32p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 2
}

# releases prints the tag of each published release of the repository, one per line.
releases() {
  local repo="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is not set}"
  gh api --paginate "repos/$repo/releases" --jq '.[] | select(.draft | not) | select(.prerelease | not) | .tag_name'
}

# resolve VERSIONS [RELEASES] prints the versions of VERSIONS that have a published stable release, each with the tag
# of its newest one, and the minor version /latest/ serves: {"latest": "vX.Y" | null, "versions": [...]}.
resolve() {
  local file="$1" list tags minors minor tag entries="[]" status
  [[ -f "$file" ]] || die "no versions file: $file"
  jq -e '.versions | type == "array"' "$file" >/dev/null || die "$file: .versions is not a list"
  if [[ $# -ge 2 ]]; then
    list="$(cat "$2")"
  else
    list="$(releases)"
  fi
  # The stable releases, vX.Y.Z only, newest first.
  tags="$(grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' <<<"$list" | sort -rV || true)"
  minors="$(jq -r '.versions[].minor' "$file")"
  while IFS= read -r minor; do
    [[ -n "$minor" ]] || continue
    [[ "$minor" =~ ^v[0-9]+\.[0-9]+$ ]] || die "$file: $minor is not a minor version (vX.Y)"
    status="$(jq -r --arg m "$minor" '.versions[] | select(.minor == $m) | .status' "$file")"
    [[ "$status" == supported || "$status" == ended ]] || die "$file: the status of $minor is neither supported nor ended"
    tag="$(grep -E "^${minor//./\\.}\.[0-9]+$" <<<"$tags" | head -n 1 || true)"
    if [[ -z "$tag" ]]; then
      echo "docs-site: $minor has no published release yet; leaving it out" >&2
      continue
    fi
    entries="$(jq -c --arg m "$minor" --arg t "$tag" --arg s "$status" '. + [{minor: $m, tag: $t, status: $s}]' <<<"$entries")"
  done <<<"$minors"
  while IFS= read -r tag; do
    [[ -n "$tag" ]] || continue
    minor="${tag%.*}"
    grep -qxF "$minor" <<<"$minors" || die "the release $tag is of $minor, which $file does not list (docs/agents/releasing.md)"
  done <<<"$tags"
  jq -n --argjson v "$entries" '{latest: ($v[0].minor // null), versions: $v}'
}

# build_tree TREE VERSION BASE REF RESOLVED DEST builds the site of the checkout TREE as VERSION under BASE and copies
# it to DEST.
build_tree() {
  local tree="$1" version="$2" base="$3" ref="$4" resolved="$5" dest="$6"
  echo "docs-site: building $version ($ref) under $base" >&2
  (cd "$tree" && DOCS_SITE_VERSION="$version" DOCS_SITE_BASE="$base" DOCS_SITE_REF="$ref" DOCS_SITE_VERSIONS="$resolved" \
    scripts/task.sh docs:site-build)
  [[ -f "$tree/app/docs/dist/index.html" ]] || die "the build of $version has no index.html"
  mkdir -p "$(dirname "$dest")"
  rm -rf "$dest"
  cp -R "$tree/app/docs/dist" "$dest"
}

# build OUT builds every version and combines them in OUT.
build() {
  local out="$1" resolved="$work/versions.json" latest count minor tag tree home
  mkdir -p "$work"
  if [[ -n "${DOCS_SITE_RELEASES:-}" ]]; then
    resolve "$root/app/docs/versions.json" "$DOCS_SITE_RELEASES" >"$resolved"
  else
    resolve "$root/app/docs/versions.json" >"$resolved"
  fi
  rm -rf "$out"
  mkdir -p "$out"

  build_tree "$root" next /next/ main "$resolved" "$out/next"

  latest="$(jq -r '.latest // empty' "$resolved")"
  count="$(jq '.versions | length' "$resolved")"
  for ((i = 0; i < count; i++)); do
    minor="$(jq -r ".versions[$i].minor" "$resolved")"
    tag="$(jq -r ".versions[$i].tag" "$resolved")"
    tree="$work/trees/$minor"
    if [[ -e "$tree" ]]; then
      git -C "$root" worktree remove --force "$tree"
    fi
    git -C "$root" worktree add --detach "$tree" "refs/tags/$tag" >&2
    build_tree "$tree" "$minor" "/$minor/" "$tag" "$resolved" "$out/$minor"
    if [[ "$minor" == "$latest" ]]; then
      build_tree "$tree" latest /latest/ "$tag" "$resolved" "$out/latest"
    fi
    git -C "$root" worktree remove --force "$tree"
  done

  # The root: the landing page, which introduces the project, and the 404 page, of the newest release.
  if [[ -n "$latest" ]]; then
    home=latest
  else
    home=next
    # Before the first release, /latest/ is main's documentation.
    printf '%s\n' '/latest /next/ 302' '/latest/* /next/:splat 302' >"$out/_redirects"
  fi
  cp "$out/$home/index.html" "$out/index.html"
  [[ ! -f "$out/$home/404.html" ]] || cp "$out/$home/404.html" "$out/404.html"
  check "$out"
  echo "docs-site: built next and $count released version(s) in $out" >&2
}

# check SITE fails when SITE exceeds the limits of Workers static assets.
check() {
  local site="$1" count bad
  count="$(find "$site" -type f | wc -l)"
  ((count <= max_files)) || die "the site has $count files, more than $max_files"
  bad="$(find "$site" -type f -size +"$max_file_bytes"c -print -quit)"
  [[ -z "$bad" ]] || die "${bad#"$site"/} is larger than $max_file_bytes bytes"
}

# deploy SITE publishes SITE as the Worker, from a work dir that holds only the Worker config and site/.
deploy() {
  local site="$1" stage="$work/deploy"
  [[ -f "$site/index.html" && -d "$site/next" ]] || die "$site is not a combined site of scripts/docs-site.sh build"
  check "$site"
  rm -rf "$stage"
  mkdir -p "$stage/site"
  cp -R "$site/." "$stage/site/"
  cp "$config" "$stage/wrangler.jsonc"
  local args=(deploy)
  [[ -z "$dry_run" ]] || args+=(--dry-run)
  wrangler_in "$stage" "${args[@]}"
}

# wrangler_in DIR ARGS... runs the pinned wrangler on the config DIR/wrangler.jsonc, from DIR (which holds only the
# config and site/), with telemetry off.
wrangler_in() {
  local dir="$1"
  shift
  (cd "$dir" && WRANGLER_SEND_METRICS=false npx --yes "wrangler@$wrangler_version" "$@" --config "$dir/wrangler.jsonc")
}

# wrangler_or_print DIR ARGS... is wrangler_in, which a dry run only prints.
wrangler_or_print() {
  if [[ -n "$dry_run" ]]; then
    echo "dry run: wrangler ${*:2} --config $1/wrangler.jsonc" >&2
    return 0
  fi
  wrangler_in "$@"
}

check_pr() {
  [[ "$1" =~ ^[1-9][0-9]{0,8}$ ]] || die "not a pull request number: $1"
}

# build_preview OUT builds /next/ of this checkout alone in OUT/next: the site of a pull request's Preview.
build_preview() {
  local out="$1" resolved="$work/preview-versions.json"
  mkdir -p "$work"
  # The Preview serves /next/ alone, so its version switcher lists no released version.
  resolve "$root/app/docs/versions.json" /dev/null >"$resolved"
  rm -rf "$out"
  mkdir -p "$out"
  build_tree "$root" next /next/ main "$resolved" "$out/next"
  check "$out"
}

# stage_preview BUILD DEST checks that BUILD is a preview build of plain files (build-preview) and copies it to
# DEST/site, with the Worker config and the redirects of this checkout.
stage_preview() {
  local build="$1" dest="$2" bad total
  [[ -d "$build" ]] || die "no build directory: $build"
  bad="$(find "$build" -mindepth 1 ! -type f ! -type d -print -quit)"
  [[ -z "$bad" ]] || die "the build has a file that is not a regular file or a directory (a link?): ${bad#"$build"/}"
  # Every name is plain: no dot files (.assetsignore, .wrangler), nothing a shell or a URL would read differently.
  bad="$(find "$build" -mindepth 1 -printf '%P\n' |
    grep -vE '^[A-Za-z0-9_@+~-][A-Za-z0-9._@+~-]*(/[A-Za-z0-9_@+~-][A-Za-z0-9._@+~-]*)*$' | head -n 1)" || true
  [[ -z "$bad" ]] || die "the build has a name outside [A-Za-z0-9._@+~-] or starting with a dot: $bad"
  # Workers static assets read these at the root as configuration; the preview takes none of them from a build.
  bad="$(find "$build" -mindepth 1 \( -name _headers -o -name _redirects -o -name _worker.js -o -name _routes.json \) \
    -printf '%P\n' -quit)"
  [[ -z "$bad" ]] || die "the build has $bad, which the Worker would read as configuration"
  bad="$(find "$build" -mindepth 1 -maxdepth 1 ! -name next -printf '%P\n' -quit)"
  [[ -z "$bad" ]] || die "the build has $bad next to next/: a preview holds /next/ alone"
  [[ -f "$build/next/index.html" ]] || die "the build has no next/index.html: is it the output of scripts/docs-site.sh build-preview?"
  check "$build"
  total="$(find "$build" -type f -printf '%s\n' | awk '{ s += $1 } END { print s + 0 }')"
  ((total <= max_preview_bytes)) || die "the build has $total bytes, more than $max_preview_bytes"

  rm -rf "$dest"
  mkdir -p "$dest/site"
  cp -R "$build/." "$dest/site/"
  cp "$config" "$dest/wrangler.jsonc"
  # The Preview has no released version: its root and /latest/ open /next/.
  printf '%s\n' '/ /next/ 302' '/latest /next/ 302' '/latest/* /next/:splat 302' >"$dest/site/_redirects"
  echo "docs-site: staged the preview ($total bytes) in $dest/site" >&2
}

# pages SITE URL CHANGED prints the Markdown list of the pages of SITE whose published document is among the
# repository paths listed in CHANGED, each linked under URL. The paths come from the pull request: they only select
# pages of SITE by a name of plain characters.
pages() {
  local site="$1" url="$2" changed="$3" path slug n=0
  local -a listed=()
  [[ -n "$changed" && -f "$changed" ]] || return 0
  while IFS= read -r path; do
    [[ "$path" =~ ^docs/(operations|reference|guide|developers)/[A-Za-z0-9_-][A-Za-z0-9._/-]*\.md$ ]] || continue
    [[ "$path" != *..* && "$path" != */./* ]] || continue
    slug="${path#docs/}"
    slug="${slug%.md}"
    slug="${slug,,}"
    slug="${slug%/readme}"
    [[ -f "$site/next/$slug/index.html" ]] || continue
    listed+=("- [$slug]($url/next/$slug/)")
  done < <(sort -u "$changed")
  n="${#listed[@]}"
  ((n > 0)) || return 0
  echo "The pages this pull request changes:"
  echo
  printf '%s\n' "${listed[@]:0:max_listed}"
  ((n <= max_listed)) || printf -- '- and %d more\n' "$((n - max_listed))"
}

# deploy_pr BUILD PR [CHANGED] publishes the preview build BUILD on the Preview pr-<PR> and writes the body of the pull
# request's comment to WORK/pr/comment.md.
deploy_pr() {
  local build="$1" pr="$2" changed="${3:-}" stage="$work/pr" out url sha body
  check_pr "$pr"
  stage_preview "$build" "$stage"
  out="$work/pr-wrangler-output.jsonl"
  : >"$out"
  if [[ -n "$dry_run" ]]; then
    # Checks the config and the assets as the upload would, without an account.
    wrangler_in "$stage" deploy --dry-run
  fi
  WRANGLER_OUTPUT_FILE_PATH="$out" wrangler_or_print "$stage" preview --name "pr-$pr" --message "Pull request #$pr"
  if [[ -n "$dry_run" ]]; then
    url="https://pr-$pr-my-docs.example.workers.dev"
  else
    url="$(jq -r 'select(.type == "preview") | .preview_urls[0] // empty' "$out" | tail -n 1)"
  fi
  url="${url%/}"
  [[ "$url" =~ ^https://pr-$pr-[a-z0-9.-]+$ ]] || die "wrangler gave no Preview URL of pr-$pr (got '$url')"
  sha="${HEAD_SHA:-}"
  [[ -z "$sha" || "$sha" =~ ^[0-9a-f]{40}$ ]] || die "HEAD_SHA is not a commit: $sha"
  body="$stage/comment.md"
  {
    echo "$marker"
    echo "### Documentation preview"
    echo
    echo "The documentation site of this pull request${sha:+ at $sha}, as main would publish it under /next/: $url/next/"
    echo
    pages "$stage/site" "$url" "$changed"
  } >"$body"
  echo "$url"
  echo "docs-site: wrote $body" >&2
}

# cleanup deletes the Preview pr-<number> of each closed or merged pull request. The Previews come from the Cloudflare
# API (wrangler has no list command) and the state of each pull request from the GitHub API, so a Preview whose pull
# request is open, or is not one, stays.
cleanup() {
  local repo="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is not set}" worker stage="$work/cleanup" page=1 body page_names
  local names="" name pr state deleted=0
  worker="$(grep -v '^[[:space:]]*//' "$config" | jq -r .name)"
  # 100 Previews a page; the cap bounds a loop on an API that ignores the page.
  while ((page <= 50)); do
    body="$(curl -fsS -H "Authorization: Bearer ${CLOUDFLARE_API_TOKEN:?CLOUDFLARE_API_TOKEN is not set}" \
      "https://api.cloudflare.com/client/v4/accounts/${CLOUDFLARE_ACCOUNT_ID:?CLOUDFLARE_ACCOUNT_ID is not set}/workers/workers/$worker/previews?per_page=100&page=$page")"
    page_names="$(jq -r '.result[]?.name // empty' <<<"$body")"
    [[ -n "$page_names" ]] || break
    names+="$page_names"$'\n'
    [[ "$(wc -l <<<"$page_names")" -ge 100 ]] || break
    page=$((page + 1))
  done
  rm -rf "$stage"
  mkdir -p "$stage/site"
  cp "$config" "$stage/wrangler.jsonc"
  while IFS= read -r name; do
    [[ "$name" =~ ^pr-([1-9][0-9]{0,8})$ ]] || continue
    pr="${BASH_REMATCH[1]}"
    if ! state="$(gh api "repos/$repo/pulls/$pr" --jq .state)"; then
      echo "docs-site: cannot read pull request $pr; keeping $name" >&2
      continue
    fi
    if [[ "$state" == closed ]]; then
      echo "docs-site: deleting $name: pull request $pr is closed" >&2
      wrangler_or_print "$stage" preview delete --name "$name" --skip-confirmation || echo "docs-site: could not delete $name" >&2
      deleted=$((deleted + 1))
    else
      echo "docs-site: keeping $name: pull request $pr is $state" >&2
    fi
  done < <(sort -u <<<"$names")
  echo "docs-site: closed pull requests with a Preview: $deleted" >&2
}

# comment PR BODY creates the preview comment of the pull request, or updates the one this workflow wrote before.
comment() {
  local pr="$1" body="$2" repo="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is not set}" id
  check_pr "$pr"
  [[ -f "$body" ]] || die "no comment body: $body"
  if [[ -n "$dry_run" ]]; then
    echo "dry run: create or update the comment of $repo#$pr with $body" >&2
    return 0
  fi
  id="$(gh api --paginate "repos/$repo/issues/$pr/comments" \
    --jq ".[] | select(.user.login == \"github-actions[bot]\" and (.body | startswith(\"$marker\"))) | .id" | head -n 1)"
  if [[ -n "$id" ]]; then
    gh api --method PATCH "repos/$repo/issues/comments/$id" -F "body=@$body" --silent
  else
    gh api --method POST "repos/$repo/issues/$pr/comments" -F "body=@$body" --silent
  fi
}

cmd="${1:-}"
[[ $# -gt 0 ]] && shift
case "$cmd" in
  resolve) [[ $# -ge 1 && $# -le 2 ]] || usage; resolve "$@" ;;
  build) [[ $# -eq 1 ]] || usage; build "$@" ;;
  deploy) [[ $# -eq 1 ]] || usage; deploy "$@" ;;
  build-preview) [[ $# -eq 1 ]] || usage; build_preview "$@" ;;
  stage-preview) [[ $# -eq 2 ]] || usage; stage_preview "$@" ;;
  pages) [[ $# -eq 3 ]] || usage; pages "$@" ;;
  deploy-pr) [[ $# -ge 2 && $# -le 3 ]] || usage; deploy_pr "$@" ;;
  cleanup) [[ $# -eq 0 ]] || usage; cleanup ;;
  comment) [[ $# -eq 2 ]] || usage; comment "$@" ;;
  # Run before the steps that hold the token, so that the package's install scripts never see it.
  install) [[ $# -eq 0 ]] || usage; npx --yes "wrangler@$wrangler_version" --version ;;
  *) usage ;;
esac
