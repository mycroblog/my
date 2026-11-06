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
#   scripts/docs-site.sh resolve <versions.json> [<releases>]   the resolved list of versions (JSON) on stdout
#   scripts/docs-site.sh build <out dir>                        build every version and combine them in <out dir>
#   scripts/docs-site.sh deploy <site dir>                      publish the combined site as the Worker
#   scripts/docs-site.sh install                                fetch the pinned wrangler into the npm cache
#
# <releases> is a file with the tag of each published release, one per line; without it `resolve` reads them from the
# GitHub API (GH_TOKEN, GITHUB_REPOSITORY). Drafts and prereleases never count. A minor version listed in versions.json
# with no release yet is left out; a stable release of a minor version versions.json does not list fails, since
# releasing.md adds the entry before the release.
#
# Environment: CLOUDFLARE_API_TOKEN and CLOUDFLARE_ACCOUNT_ID (deploy); GH_TOKEN and GITHUB_REPOSITORY (build, and
# resolve without <releases>); DOCS_SITE_WORK, the work dir (default .tmp/docs-site); DOCS_SITE_RELEASES, a <releases>
# file for `build` instead of the API. DRY_RUN=1 runs `deploy` as `wrangler deploy --dry-run`, which checks the config
# and the assets without an account:
#
#   git tag --list 'v*' > .tmp/releases   # or an empty file before the first release
#   DOCS_SITE_RELEASES=.tmp/releases scripts/docs-site.sh build .tmp/docs-site/site
#   DRY_RUN=1 scripts/docs-site.sh deploy .tmp/docs-site/site
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
config="$root/scripts/docs-site/wrangler.jsonc"
# The same wrangler as scripts/ui-preview.sh; fetched by its exact version, since only this CI job runs it.
wrangler_version="4.151.0"
dry_run="${DRY_RUN:-}"
work="${DOCS_SITE_WORK:-$root/.tmp/docs-site}"

# The limits of Workers static assets on the free plan.
max_files=20000
max_file_bytes=$((25 * 1024 * 1024))

die() {
  echo "docs-site: $*" >&2
  exit 1
}

usage() {
  sed -n '15,19p' "$0" | sed 's/^# \{0,1\}//' >&2
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
  local args=(deploy --config "$stage/wrangler.jsonc")
  [[ -z "$dry_run" ]] || args+=(--dry-run)
  (cd "$stage" && WRANGLER_SEND_METRICS=false npx --yes "wrangler@$wrangler_version" "${args[@]}")
}

cmd="${1:-}"
[[ $# -gt 0 ]] && shift
case "$cmd" in
  resolve) [[ $# -ge 1 && $# -le 2 ]] || usage; resolve "$@" ;;
  build) [[ $# -eq 1 ]] || usage; build "$@" ;;
  deploy) [[ $# -eq 1 ]] || usage; deploy "$@" ;;
  # Run before the steps that hold the token, so that the package's install scripts never see it.
  install) [[ $# -eq 0 ]] || usage; npx --yes "wrangler@$wrangler_version" --version ;;
  *) usage ;;
esac
