#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/ui-preview.sh without an account or the network: the checks of `stage` on builds a pull request
# could upload, the list of added and changed stories of `stories`, and the dry run of `delete-pr` and `comment`.
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

# index FILE ENTRY... writes a Storybook index with the entries, each "id|title|name|importPath".
index() {
  local file="$1" entry id title name path
  shift
  mkdir -p "$(dirname "$file")"
  {
    echo '{"v":5,"entries":{'
    local sep=""
    for entry in "$@"; do
      IFS='|' read -r id title name path <<<"$entry"
      printf '%s"%s":{"type":"story","id":"%s","title":"%s","name":"%s","importPath":"%s"}' \
        "$sep" "$id" "$id" "$title" "$name" "$path"
      sep=","
    done
    echo '}}'
  } >"$file"
}

# build DIR writes a minimal composed Storybook build.
build() {
  local dir="$1" f
  mkdir -p "$dir/ui" "$dir/assets"
  for f in index.html iframe.html ui/index.html ui/iframe.html assets/main-Bx_1~a.js; do
    echo "<!-- $f -->" >"$dir/$f"
  done
  index "$dir/index.json" "screens-a--normal|Screens/A|Normal|./src/features/a/a.stories.tsx"
  index "$dir/ui/index.json" "display-b--plain|Display/B|Plain|./src/B.stories.tsx"
}

# expect_stage NAME WANT_ERROR SETUP: WANT_ERROR is a part of the error, or "" when the build must pass.
expect_stage() {
  local name="$1" want="$2" dir="$tmp/stage-$1" out
  build "$dir"
  (cd "$dir" && eval "$3")
  if out="$("$script" stage "$dir" "$tmp/work-$name" 2>&1)"; then
    [[ -z "$want" ]] || fail "stage $name: passed, want an error with '$want'"
    [[ -z "$want" && -f "$tmp/work-$name/site/index.json" && -f "$tmp/work-$name/wrangler.jsonc" ]] ||
      [[ -n "$want" ]] || fail "stage $name: did not copy the build and the config"
  else
    [[ -n "$want" && "$out" == *"$want"* ]] || fail "stage $name: '$out', want '${want:-success}'"
  fi
}

expect_stage plain "" ":"
expect_stage symlink "not a regular file" "ln -s /etc/passwd assets/x.js"
expect_stage dotfile "starting with a dot" "touch .assetsignore"
expect_stage wrangler-dir "starting with a dot" "mkdir .wrangler && touch .wrangler/config.json"
expect_stage headers "_headers" "touch _headers"
expect_stage redirects "_redirects" "touch _redirects"
expect_stage odd-name "outside" "touch 'assets/a b.js'"
expect_stage no-ui "ui/index.json" "rm ui/index.json"
expect_stage not-index "is not a Storybook index" "echo '[]' > index.json"

# stories: main has A (Normal) and B; the pull request renames A's title, adds A--empty and C (in ui), keeps B but
# changes its file, and keeps D untouched.
pr="$tmp/pr"
main="$tmp/main"
build "$pr"
build "$main"
index "$main/index.json" \
  "screens-a--normal|Screens/A|Normal|./src/features/a/a.stories.tsx" \
  "screens-d--normal|Screens/D|Normal|./src/features/d/d.stories.tsx"
index "$main/ui/index.json" "display-b--plain|Display/B|Plain|./src/B.stories.tsx"
index "$pr/index.json" \
  "screens-a--normal|Screens/A2|Normal|./src/features/a/a.stories.tsx" \
  "screens-a--empty|Screens/A2|Empty @everyone [x](http://evil)|./src/features/a/a.stories.tsx" \
  "screens-d--normal|Screens/D|Normal|./src/features/d/d.stories.tsx"
index "$pr/ui/index.json" \
  "display-b--plain|Display/B|Plain|./src/B.stories.tsx" \
  "display-c--plain|Display/C|Plain|./src/C.stories.tsx"
printf 'app/ui/src/B.stories.tsx\napp/web/src/other.ts\n' >"$tmp/changed"
got="$("$script" stories "$pr" "$main" https://pr-1-x.example.workers.dev/ "$tmp/changed")"
want='#### Added (2)

- [Screens/A2 › Empty \@everyone \[x\]\(http\://evil\)](https://pr-1-x.example.workers.dev/?path=/story/screens-a--empty)
- [Display/C › Plain](https://pr-1-x.example.workers.dev/?path=/story/ui_display-c--plain)

#### Changed (2)

- [Screens/A2 › Normal](https://pr-1-x.example.workers.dev/?path=/story/screens-a--normal)
- [Display/B › Plain](https://pr-1-x.example.workers.dev/?path=/story/ui_display-b--plain)'
[[ "$got" == "$want" ]] || fail "stories: got
$got
want
$want"

got="$("$script" stories "$main" "$main" https://x.example)"
[[ "$got" == "No story was added or changed compared with main." ]] || fail "stories without changes: $got"

# The dry runs touch no account.
if out="$(DRY_RUN=1 UI_PREVIEW_WORK="$tmp/delete" "$script" delete-pr 7 2>&1)"; then
  [[ "$out" == *"wrangler preview delete --name pr-7 --skip-confirmation"* ]] || fail "delete-pr: $out"
  grep -q '^<!-- ui-preview -->$' "$tmp/delete/comment.md" || fail "delete-pr wrote no comment"
else
  fail "delete-pr: $out"
fi
out="$(DRY_RUN=1 GITHUB_REPOSITORY=o/r "$script" comment 7 "$tmp/delete/comment.md" 2>&1)" || fail "comment: $out"
"$script" delete-pr 'x; true' >/dev/null 2>&1 && fail "delete-pr took a PR number that is not one"

if [[ "$failed" != 0 ]]; then
  exit 1
fi
echo "ui-preview: ok"
