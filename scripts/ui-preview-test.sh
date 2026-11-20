#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/ui-preview.sh without an account or the network: the checks of `stage` on builds a pull request could
# upload, what it adds next to the build, the stories the preview comment links, and the dry run of `comment`.
#
#   scripts/ui-preview-test.sh
set -euo pipefail

script="$(cd "$(dirname "$0")" && pwd)/ui-preview.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

failed=0
fail() {
  echo "FAIL: $*" >&2
  failed=1
}

# catalog DIR writes a minimal Storybook build: its pages, an asset, and the index of four components in the order of
# the catalog, one of them with two stories.
catalog() {
  local dir="$1"
  mkdir -p "$dir/assets" "$dir/sb-manager"
  echo '<!-- index -->' >"$dir/index.html"
  echo '<!-- iframe -->' >"$dir/iframe.html"
  echo '// js' >"$dir/assets/button.stories-Bx_1~a.js"
  echo '// js' >"$dir/sb-manager/runtime.js"
  cat >"$dir/index.json" <<'EOF'
{"v": 5, "entries": {
  "tokens-type--type": {"type": "story", "id": "tokens-type--type", "title": "Tokens/Type", "name": "Type", "importPath": "../design-system/src/tokens.stories.tsx"},
  "design-system-button--docs": {"type": "docs", "id": "design-system-button--docs", "title": "Design system/Button", "name": "Docs", "importPath": "../design-system/src/button.stories.tsx"},
  "design-system-button--default": {"type": "story", "id": "design-system-button--default", "title": "Design system/Button", "name": "Default", "importPath": "../design-system/src/button.stories.tsx"},
  "design-system-button--pressed": {"type": "story", "id": "design-system-button--pressed", "title": "Design system/Button", "name": "Pressed", "importPath": "../design-system/src/button.stories.tsx"},
  "design-system-button-group--default": {"type": "story", "id": "design-system-button-group--default", "title": "Design system/Button group", "name": "Default", "importPath": "../design-system/src/button-group.stories.tsx"},
  "screens-home--empty": {"type": "story", "id": "screens-home--empty", "title": "Screens/Home [see @everyone](https://example.com)", "name": "Empty", "importPath": "../app/src/home.stories.tsx"}
}}
EOF
}

# expect_stage NAME WANT_ERROR SETUP: WANT_ERROR is a part of the error, or "" when the build must pass.
expect_stage() {
  local name="$1" want="$2" dir="$tmp/stage-$1" out
  catalog "$dir"
  (cd "$dir" && eval "$3")
  if out="$("$script" stage "$dir" "$tmp/work-$name" 2>&1)"; then
    [[ -z "$want" ]] || fail "stage $name: passed, want an error with '$want'"
    [[ -n "$want" || (-f "$tmp/work-$name/site/iframe.html" && -f "$tmp/work-$name/wrangler.jsonc") ]] ||
      fail "stage $name: did not copy the build and the config"
    [[ -n "$want" ]] || [[ "$(cat "$tmp/work-$name/site/_headers")" == $'/*\n  X-Robots-Tag: noindex, nofollow' ]] ||
      fail "stage $name: no _headers that keeps every page out of search engines"
  else
    [[ -n "$want" && "$out" == *"$want"* ]] || fail "stage $name: '$out', want '${want:-success}'"
  fi
}

expect_stage plain "" ":"
expect_stage symlink "not a regular file" "ln -s /etc/passwd assets/x.js"
expect_stage dotfile "starting with a dot" "touch .assetsignore"
expect_stage wrangler-dir "starting with a dot" "mkdir .wrangler && touch .wrangler/config.json"
expect_stage headers "configuration" "touch _headers"
expect_stage redirects "configuration" "touch assets/_redirects"
expect_stage worker "configuration" "touch _worker.js"
expect_stage odd-name "outside" "touch 'assets/a b.js'"
expect_stage no-iframe "iframe.html" "rm iframe.html"
expect_stage no-index "index.json" "rm index.json"
expect_stage too-many "more than 20000" "mkdir many && (cd many && seq 1 20001 | xargs touch)"

url=https://pr-1-my-ui.example.workers.dev
catalog "$tmp/site"

# stories: the components whose story file, or a file of the same name next to it, changed, in the order of the
# catalog; not button-group for button.tsx, nothing outside web/, and no title that is not plain characters.
printf '%s\n' web/design-system/src/button.tsx web/design-system/src/tokens.css web/app/src/home.stories.tsx \
  web/design-system/src/other.tsx docs/adr/0165-ui-preview.md 'web/../design-system/src/button-group.tsx' >"$tmp/changed"
got="$("$script" stories "$tmp/site" "$url" "$tmp/changed")"
want="$(printf '%s\n' 'The components and the screens it changes, in the order of the catalog:' '' \
  "- [Tokens/Type]($url/?path=/story/tokens-type--type)" \
  "- [Design system/Button]($url/?path=/story/design-system-button--default) (2 stories)" \
  "- [screens-home--empty]($url/?path=/story/screens-home--empty)")"
[[ "$got" == "$want" ]] || fail "stories: got '$got', want '$want'"

printf '%s\n' web/design-system/src/button-group.stories.tsx >"$tmp/changed"
got="$("$script" stories "$tmp/site" "$url" "$tmp/changed")"
[[ "$got" == *"[Design system/Button group]($url/?path=/story/design-system-button-group--default)" && "$got" != *"Button]"* ]] ||
  fail "stories of button-group: got '$got'"

printf '%s\n' web/docs/astro.config.ts >"$tmp/changed"
got="$("$script" stories "$tmp/site" "$url" "$tmp/changed")"
[[ "$got" == "It changes no story file and no file next to one." ]] || fail "stories with no story: got '$got'"

# An id that is not one of Storybook's, or an import path out of web/, is never linked.
sed -i 's#"id": "tokens-type--type"#"id": "x](https://example.com) --type"#; s#\.\./app/src/home#../../../etc/home#' "$tmp/site/index.json"
printf '%s\n' web/design-system/src/tokens.css web/app/src/home.stories.tsx >"$tmp/changed"
got="$("$script" stories "$tmp/site" "$url" "$tmp/changed")"
[[ "$got" == "It changes no story file and no file next to one." ]] || fail "stories of untrusted entries: got '$got'"

echo '{"entries": []}' >"$tmp/site/index.json"
if "$script" stories "$tmp/site" "$url" "$tmp/changed" >/dev/null 2>&1; then
  fail "stories: took an index.json with no entries"
fi

# comment: a dry run checks the arguments and prints what it would do.
echo body >"$tmp/body.md"
GITHUB_REPOSITORY=mycroblog/my DRY_RUN=1 "$script" comment 12 "$tmp/body.md" 2>/dev/null || fail "comment: dry run failed"
if GITHUB_REPOSITORY=mycroblog/my DRY_RUN=1 "$script" comment 12x "$tmp/body.md" >/dev/null 2>&1; then
  fail "comment: took a pull request number that is not one"
fi

# cleanup: the Cloudflare API is a stand-in curl on PATH that answers with the status in $tmp/status, and gh a
# stand-in that says every pull request is closed. A Worker that does not exist yet (404) has nothing to delete; another
# status fails; a page of Previews lists pr-7 for deletion, which the dry run prints instead of running.
mkdir -p "$tmp/bin"
cat >"$tmp/bin/curl" <<'EOF'
#!/usr/bin/env bash
status="$(cat "$UI_PREVIEW_TEST_STATUS")"
if [[ "$status" == 200 ]]; then printf '{"result": [{"name": "pr-7"}, {"name": "main"}]}'; else printf '{}'; fi
printf '\n%s' "$status"
EOF
printf '#!/usr/bin/env bash\necho closed\n' >"$tmp/bin/gh"
chmod +x "$tmp/bin/curl" "$tmp/bin/gh"
cleanup() {
  echo "$1" >"$tmp/status"
  PATH="$tmp/bin:$PATH" UI_PREVIEW_TEST_STATUS="$tmp/status" CLOUDFLARE_API_TOKEN=t CLOUDFLARE_ACCOUNT_ID=a \
    GITHUB_REPOSITORY=mycroblog/my DRY_RUN=1 UI_PREVIEW_WORK="$tmp/work" "$script" cleanup 2>&1
}
cleanup 404 >"$tmp/out" || fail "cleanup: a Worker not created yet failed the cleanup: $(cat "$tmp/out")"
grep -q 'no such Worker yet' "$tmp/out" || fail "cleanup: a 404 did not say the Worker does not exist yet"
if cleanup 500 >"$tmp/out"; then
  fail "cleanup: took an error of the Cloudflare API as no Preview"
fi
cleanup 200 >"$tmp/out" || fail "cleanup: a page of Previews failed: $(cat "$tmp/out")"
grep -q 'deleting pr-7' "$tmp/out" || fail "cleanup: did not delete the Preview of a closed pull request: $(cat "$tmp/out")"

if [[ "$failed" -ne 0 ]]; then
  exit 1
fi
echo "ui-preview: all tests passed"
