#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/docs-site.sh without an account or the network: how `resolve` turns app/docs/versions.json and the
# published releases into the versions the deploy builds, the checks of `stage-preview` on builds a pull request
# could upload, the pages the preview comment links, and the dry run of `comment`.
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
