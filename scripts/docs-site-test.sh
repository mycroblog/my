#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/docs-site.sh without an account or the network: how `resolve` turns app/docs/versions.json and the
# published releases into the versions the deploy builds, the checks of `stage-preview` on builds a pull request
# could upload, the pages the preview comment links, what `index-site` writes for search engines and what `check-site`
# and `check-preview` refuse, and the dry run of `comment`.
#
#   scripts/docs-site-test.sh
set -euo pipefail

script="$(cd "$(dirname "$0")" && pwd)/docs-site.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

failed=0
fail() {
  echo "FAIL: $*" >&2
  failed=1
}

# expect NAME VERSIONS RELEASES WANT runs resolve on the versions list (the JSON of .versions) and the releases (one
# tag per line) and compares the compact output with WANT.
expect() {
  local name="$1" got
  printf '{"versions":%s}\n' "$2" >"$tmp/versions.json"
  printf '%b' "$3" >"$tmp/releases"
  if ! got="$("$script" resolve "$tmp/versions.json" "$tmp/releases" 2>/dev/null | jq -c .)"; then
    fail "$name: resolve failed"
    return
  fi
  [[ "$got" == "$4" ]] || fail "$name: got $got, want $4"
}

# refuse NAME VERSIONS RELEASES expects resolve to fail.
refuse() {
  printf '{"versions":%s}\n' "$2" >"$tmp/versions.json"
  printf '%b' "$3" >"$tmp/releases"
  if "$script" resolve "$tmp/versions.json" "$tmp/releases" >/dev/null 2>&1; then
    fail "$1: resolve passed"
  fi
}

expect "before the first release" '[]' '' '{"latest":null,"versions":[]}'
expect "prereleases do not count" '[{"minor":"v0.1","status":"supported"}]' 'v0.1.0-rc.1\nv0.1.0-rc.2\n' \
  '{"latest":null,"versions":[]}'
expect "the newest patch" '[{"minor":"v0.1","status":"supported"}]' 'v0.1.2\nv0.1.10\nv0.1.9\nv0.1.11-rc.1\n' \
  '{"latest":"v0.1","versions":[{"minor":"v0.1","tag":"v0.1.10","status":"supported"}]}'
expect "an ended version stays" \
  '[{"minor":"v0.2","status":"supported"},{"minor":"v0.1","status":"ended"}]' 'v0.1.3\nv0.2.0\nv0.1.4\n' \
  '{"latest":"v0.2","versions":[{"minor":"v0.2","tag":"v0.2.0","status":"supported"},{"minor":"v0.1","tag":"v0.1.4","status":"ended"}]}'
expect "a listed version before its release" \
  '[{"minor":"v0.2","status":"supported"},{"minor":"v0.1","status":"supported"}]' 'v0.1.4\n' \
  '{"latest":"v0.1","versions":[{"minor":"v0.1","tag":"v0.1.4","status":"supported"}]}'
expect "v0.1 is not v0.10" '[{"minor":"v0.1","status":"supported"},{"minor":"v0.10","status":"supported"}]' \
  'v0.10.0\nv0.1.5\n' \
  '{"latest":"v0.1","versions":[{"minor":"v0.1","tag":"v0.1.5","status":"supported"},{"minor":"v0.10","tag":"v0.10.0","status":"supported"}]}'
refuse "a release of an unlisted minor version" '[{"minor":"v0.1","status":"supported"}]' 'v0.1.0\nv0.2.0\n'
refuse "an unknown status" '[{"minor":"v0.1","status":"old"}]' 'v0.1.0\n'
refuse "not a minor version" '[{"minor":"v0.1.0","status":"supported"}]' 'v0.1.0\n'

# preview DIR writes a minimal preview build: /next/ with two pages and the assets of Astro and Pagefind.
preview() {
  local dir="$1" f
  mkdir -p "$dir/next/operations/running" "$dir/next/guide" "$dir/next/_astro" "$dir/next/pagefind"
  for f in index.html 404.html operations/running/index.html guide/index.html _astro/index.Bx_1~a.css \
    pagefind/pagefind-entry.json; do
    echo "<!-- $f -->" >"$dir/next/$f"
  done
}

# expect_stage NAME WANT_ERROR SETUP: WANT_ERROR is a part of the error, or "" when the build must pass.
expect_stage() {
  local name="$1" want="$2" dir="$tmp/stage-$1" out
  preview "$dir"
  (cd "$dir" && eval "$3")
  if out="$("$script" stage-preview "$dir" "$tmp/work-$name" 2>&1)"; then
    [[ -z "$want" ]] || fail "stage $name: passed, want an error with '$want'"
    [[ -n "$want" || (-f "$tmp/work-$name/site/next/index.html" && -f "$tmp/work-$name/wrangler.jsonc") ]] ||
      fail "stage $name: did not copy the build and the config"
    [[ -n "$want" ]] || grep -qx '/ /next/ 302' "$tmp/work-$name/site/_redirects" ||
      fail "stage $name: no redirect of / to /next/"
  else
    [[ -n "$want" && "$out" == *"$want"* ]] || fail "stage $name: '$out', want '${want:-success}'"
  fi
}

expect_stage plain "" ":"
expect_stage symlink "not a regular file" "ln -s /etc/passwd next/_astro/x.js"
expect_stage dotfile "starting with a dot" "touch next/.assetsignore"
expect_stage wrangler-dir "starting with a dot" "mkdir .wrangler && touch .wrangler/config.json"
expect_stage headers "configuration" "touch next/_headers"
expect_stage redirects "configuration" "touch next/guide/_redirects"
expect_stage worker "configuration" "touch _worker.js"
expect_stage odd-name "outside" "touch 'next/a b.html'"
expect_stage other-version "next to next/" "mkdir latest && touch latest/index.html"
expect_stage no-index "next/index.html" "rm next/index.html"

# pages: the published documents among the changed files that the build has, and nothing else.
preview "$tmp/pages"
printf '%s\n' docs/operations/running.md docs/guide/README.md docs/reference/commands.md docs/adr/0150-x.md \
  docs/operations/gone.md 'docs/guide/../../etc/passwd.md' app/docs/astro.config.ts >"$tmp/changed"
got="$("$script" pages "$tmp/pages" https://pr-1-my-docs.example.workers.dev "$tmp/changed")"
want="$(printf '%s\n' 'The pages this pull request changes:' '' \
  '- [guide](https://pr-1-my-docs.example.workers.dev/next/guide/)' \
  '- [operations/running](https://pr-1-my-docs.example.workers.dev/next/operations/running/)')"
[[ "$got" == "$want" ]] || fail "pages: got '$got', want '$want'"
printf '%s\n' app/docs/astro.config.ts >"$tmp/changed"
got="$("$script" pages "$tmp/pages" https://pr-1-my-docs.example.workers.dev "$tmp/changed")"
[[ -z "$got" ]] || fail "pages with no published document: got '$got'"

# page FILE CANONICAL [noindex] writes a page with the head app/docs/src/indexing.ts gives it.
origin=https://mycroblog.dev
page() {
  local robots=""
  [[ -z "${3:-}" ]] || robots='<meta name="robots" content="noindex, follow"/>'
  mkdir -p "$(dirname "$1")"
  printf '%s' "<!DOCTYPE html><html><head><link rel=\"canonical\" href=\"$2\"/><meta property=\"og:title\" content=\"T\"/>" \
    "<meta property=\"og:url\" content=\"$2\"/><meta property=\"og:description\" content=\"D\"/>" \
    "<meta name=\"description\" content=\"D\"/>$robots</head><body></body></html>" >"$1"
  echo >>"$1"
}

# released DIR writes a combined site with a release: /latest/ (v0.1) is indexed, /v0.1/ and /next/ are not.
released() {
  local d="$1" v
  page "$d/latest/index.html" "$origin/"
  page "$d/latest/404.html" "$origin/latest/404/"
  page "$d/latest/operations/running/index.html" "$origin/latest/operations/running/"
  page "$d/latest/ja/index.html" "$origin/latest/ja/"
  # A fallback page, the English page under /ja/: its canonical is the English page.
  page "$d/latest/ja/operations/running/index.html" "$origin/latest/operations/running/"
  echo '<sitemapindex/>' >"$d/latest/sitemap-index.xml"
  for v in v0.1 next; do
    page "$d/$v/index.html" "$origin/$v/" noindex
    page "$d/$v/404.html" "$origin/$v/404/" noindex
    page "$d/$v/operations/running/index.html" "$origin/$v/operations/running/" noindex
    page "$d/$v/ja/operations/running/index.html" "$origin/$v/operations/running/" noindex
    echo '<sitemapindex/>' >"$d/$v/sitemap-index.xml"
  done
  page "$d/v0.1/operations/old/index.html" "$origin/v0.1/operations/old/" noindex
  page "$d/next/guide/new/index.html" "$origin/next/guide/new/" noindex
  cp "$d/latest/index.html" "$d/index.html"
}

canonical_of() {
  grep -o '<link rel="canonical" href="[^"]*"' "$1" | sed -E 's#.*href="([^"]*)"#\1#'
}

# expect_site NAME WANT_ERROR SETUP indexes a released site, runs SETUP in it, and checks it: WANT_ERROR is a part of
# the error, or "" when the check must pass.
expect_site() {
  local name="$1" want="$2" dir="$tmp/site-$1" out
  released "$dir"
  "$script" index-site "$dir" 2>/dev/null || {
    fail "site $name: index-site failed"
    return
  }
  (cd "$dir" && eval "$3")
  if out="$("$script" check-site "$dir" 2>&1)"; then
    [[ -z "$want" ]] || fail "site $name: passed, want an error with '$want'"
  else
    [[ -n "$want" && "$out" == *"$want"* ]] || fail "site $name: '$out', want '${want:-success}'"
  fi
}

expect_site plain "" ":"
expect_site robots "robots.txt" "echo 'User-agent: *' >robots.txt"
expect_site sitemap-extra "does not list the indexed pages" \
  "sed -i 's#</urlset>#<url><loc>$origin/next/guide/new/</loc></url></urlset>#' sitemap-0.xml"
expect_site sitemap-broken "is not a sitemap" "sed -i 's#</urlset>##' sitemap-0.xml"
expect_site version-sitemap "sitemap" "echo '<sitemapindex/>' >next/sitemap-index.xml"
expect_site next-indexed "carries no noindex" "sed -i 's#<meta name=\"robots\"[^>]*>##' next/guide/new/index.html"
expect_site latest-noindex "indexed /latest/ carries noindex" \
  "sed -i 's#</head>#<meta name=\"robots\" content=\"noindex, follow\"/></head>#' latest/operations/running/index.html"
expect_site canonical "not $origin/latest/operations/running/" \
  "sed -i 's#href=\"$origin/latest/operations/running/\"#href=\"$origin/v0.1/operations/running/\"#' v0.1/operations/running/index.html"
expect_site description "no <meta name=\"description\"" "sed -i 's#<meta name=\"description\"[^>]*>##' v0.1/404.html"
expect_site og-url "no <meta property=\"og:url\"" "sed -i 's#<meta property=\"og:url\"[^>]*>##' latest/ja/index.html"

# index-site: the root sitemap lists the root and the pages of /latest/ that are their own canonical, and the pages of
# the other versions point their canonical at /latest/ where it has the page.
dir="$tmp/site-plain"
got="$(grep -o '<loc>[^<]*</loc>' "$dir/sitemap-0.xml" | tr '\n' ' ')"
want="<loc>$origin/</loc> <loc>$origin/latest/ja/</loc> <loc>$origin/latest/operations/running/</loc> "
[[ "$got" == "$want" ]] || fail "index-site sitemap: got '$got', want '$want'"
grep -qx "Sitemap: $origin/sitemap-index.xml" "$dir/robots.txt" || fail "index-site: robots.txt names no sitemap"
[[ ! -e "$dir/latest/sitemap-index.xml" ]] || fail "index-site: left the sitemap of /latest/"
for want in "v0.1/index.html $origin/" "v0.1/operations/running/index.html $origin/latest/operations/running/" \
  "next/ja/operations/running/index.html $origin/latest/operations/running/" \
  "v0.1/operations/old/index.html $origin/v0.1/operations/old/" "next/guide/new/index.html $origin/next/guide/new/"; do
  got="$(canonical_of "$dir/${want%% *}")"
  [[ "$got" == "${want#* }" ]] || fail "index-site canonical of ${want%% *}: got $got, want ${want#* }"
done
grep -qF "<meta property=\"og:url\" content=\"$origin/latest/operations/running/\"" \
  "$dir/v0.1/operations/running/index.html" || fail "index-site: og:url of v0.1/operations/running is not /latest/'s"

# Before the first release, /next/ is indexed.
dir="$tmp/site-unreleased"
page "$dir/next/index.html" "$origin/"
page "$dir/next/guide/index.html" "$origin/next/guide/"
cp "$dir/next/index.html" "$dir/index.html"
"$script" index-site "$dir" 2>/dev/null || fail "unreleased: index-site failed"
"$script" check-site "$dir" >/dev/null 2>&1 || fail "unreleased: check-site failed"
got="$(grep -o '<loc>[^<]*</loc>' "$dir/sitemap-0.xml" | tr '\n' ' ')"
want="<loc>$origin/</loc> <loc>$origin/next/guide/</loc> "
[[ "$got" == "$want" ]] || fail "unreleased sitemap: got '$got', want '$want'"

# check-preview: every page of a Preview carries noindex, and it has no robots.txt and no sitemap.
expect_preview() {
  local name="$1" want="$2" dir="$tmp/preview-$1" out
  page "$dir/next/index.html" "$origin/next/" noindex
  page "$dir/next/404.html" "$origin/next/404/" noindex
  page "$dir/next/guide/index.html" "$origin/next/guide/" noindex
  (cd "$dir" && eval "$3")
  if out="$("$script" check-preview "$dir" 2>&1)"; then
    [[ -z "$want" ]] || fail "preview $name: passed, want an error with '$want'"
  else
    [[ -n "$want" && "$out" == *"$want"* ]] || fail "preview $name: '$out', want '${want:-success}'"
  fi
}
expect_preview plain "" ":"
expect_preview indexed "carries no noindex" "sed -i 's#<meta name=\"robots\"[^>]*>##' next/404.html"
expect_preview sitemap "sitemap-index.xml" "touch next/sitemap-index.xml"
expect_preview og-title "og:title" "sed -i 's#<meta property=\"og:title\"[^>]*>##' next/guide/index.html"

# comment: a dry run checks the arguments and prints what it would do.
echo body >"$tmp/body.md"
GITHUB_REPOSITORY=mycroblog/my DRY_RUN=1 "$script" comment 12 "$tmp/body.md" 2>/dev/null || fail "comment: dry run failed"
if GITHUB_REPOSITORY=mycroblog/my DRY_RUN=1 "$script" comment 12x "$tmp/body.md" >/dev/null 2>&1; then
  fail "comment: took a pull request number that is not one"
fi

if [[ "$failed" -ne 0 ]]; then
  exit 1
fi
echo "docs-site: all tests passed"
