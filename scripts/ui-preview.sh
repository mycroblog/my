#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Publishes the UI preview (ADR-0112 §2-§4): the static build of the app/web Storybook (with the @my/ui Storybook
# under ui/, `scripts/task.sh web:storybook-build`) on the Cloudflare Worker of scripts/ui-preview/wrangler.jsonc.
# ui-preview-deploy.yml runs it from main's checkout on the build that ui-preview.yml uploaded. A pull request's build
# comes from code anyone can write (forks included), so it is only ever data: `stage` checks that it is plain static
# files and copies it next to main's Worker config, wrangler uploads it as assets, and `stories` reads its index.json
# with jq. Nothing in it is executed or sourced, and nothing of it reaches a shell, a config, or a command line.
#
#   scripts/ui-preview.sh stage <build dir> <work dir>          check the build, copy it to <work dir>/site with the config
#   scripts/ui-preview.sh stories <site dir> <main dir> <url> [<changed files>]
#                                                               the Markdown list of the added and changed stories
#   scripts/ui-preview.sh deploy-pr <build dir> <pr> [<changed files>]
#                                                               publish the Preview pr-<pr>; writes <work>/comment.md
#   scripts/ui-preview.sh deploy-main <build dir>               publish the build as the Worker itself (main)
#   scripts/ui-preview.sh delete-pr <pr>                        delete the Preview pr-<pr>; writes <work>/comment.md
#   scripts/ui-preview.sh comment <pr> <body file> [update]     create (unless `update`) or update the preview comment
#   scripts/ui-preview.sh install                               fetch the pinned wrangler into the npm cache
#
# Environment: CLOUDFLARE_API_TOKEN and CLOUDFLARE_ACCOUNT_ID (wrangler); GH_TOKEN and GITHUB_REPOSITORY (comment);
# UI_PREVIEW_WORK, the work dir of deploy-* and delete-pr (default .tmp/ui-preview); UI_PREVIEW_METHOD, `previews`
# (default: Workers Previews) or `versions` (a version with the preview alias pr-<pr>, for when Previews do not take
# an assets-only Worker); HEAD_SHA, the commit the comment names.
#
# DRY_RUN=1 runs everything that needs no account: stage, stories, `wrangler deploy --dry-run` on the staged site
# (which checks the config and the assets), and the comment body, with https://pr-<pr>-my-ui-preview.example.workers.dev
# as the Preview URL; main's index.json comes from MAIN_INDEX_DIR (a build dir) if set, else main counts as empty.
# It prints the wrangler and gh commands it would run instead of running them:
#
#   scripts/task.sh web:storybook-build
#   DRY_RUN=1 scripts/ui-preview.sh deploy-pr app/web/storybook-static 123 && cat .tmp/ui-preview/comment.md
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
config="$root/scripts/ui-preview/wrangler.jsonc"
# Previews need wrangler 4.135.0 or later. wrangler is the only tool of this script that is not on the runner; it is
# fetched by its exact version rather than added to a lockfile, since only this CI job runs it.
wrangler_version="4.151.0"
marker="<!-- ui-preview -->"
dry_run="${DRY_RUN:-}"
method="${UI_PREVIEW_METHOD:-previews}"
work="${UI_PREVIEW_WORK:-$root/.tmp/ui-preview}"

# The limits of Workers static assets on the free plan, and a bound on the whole upload.
max_files=20000
max_file_bytes=$((25 * 1024 * 1024))
max_total_bytes=$((300 * 1024 * 1024))
# The files the build must have: the screens' Storybook and the composed @my/ui one.
required=(index.html iframe.html index.json ui/index.html ui/iframe.html ui/index.json)
# The most stories the comment links; the rest are counted.
max_listed=40

die() {
  echo "ui-preview: $*" >&2
  exit 1
}

usage() {
  sed -n '11,19p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 2
}

check_pr() {
  [[ "$1" =~ ^[1-9][0-9]{0,8}$ ]] || die "not a pull request number: $1"
}

# wrangler ARGS... runs the pinned wrangler on the staged config, from the work dir (which holds only the config and
# site/), with telemetry off; in a dry run it only prints the command.
wrangler() {
  if [[ -n "$dry_run" ]]; then
    echo "dry run: wrangler $* --config $work/wrangler.jsonc" >&2
    return 0
  fi
  (cd "$work" && WRANGLER_SEND_METRICS=false npx --yes "wrangler@$wrangler_version" "$@" --config "$work/wrangler.jsonc")
}

# stage BUILD WORK checks that BUILD is a Storybook build of plain files and copies it to WORK/site, with the Worker
# config of this checkout as WORK/wrangler.jsonc.
stage() {
  local build="$1" dest="$2" bad count total f
  [[ -d "$build" ]] || die "no build directory: $build"
  bad="$(find "$build" -mindepth 1 ! -type f ! -type d -print -quit)"
  [[ -z "$bad" ]] || die "the build has a file that is not a regular file or a directory (a link?): ${bad#"$build"/}"
  # Every name is plain: no dot files (.assetsignore, .wrangler), nothing a shell or a URL would read differently.
  bad="$(find "$build" -mindepth 1 -printf '%P\n' |
    grep -vE '^[A-Za-z0-9_@+~-][A-Za-z0-9._@+~-]*(/[A-Za-z0-9_@+~-][A-Za-z0-9._@+~-]*)*$' | head -n 1)" || true
  [[ -z "$bad" ]] || die "the build has a name outside [A-Za-z0-9._@+~-] or starting with a dot: $bad"
  # Workers static assets read _headers and _redirects at the root as rules; the preview takes neither from a build.
  for f in _headers _redirects _worker.js _routes.json; do
    [[ ! -e "$build/$f" ]] || die "the build has $f, which the Worker would read as configuration"
  done
  for f in "${required[@]}"; do
    [[ -f "$build/$f" ]] || die "the build has no $f: is it the output of scripts/task.sh web:storybook-build?"
  done
  for f in index.json ui/index.json; do
    jq -e '.entries | type == "object"' "$build/$f" >/dev/null 2>&1 || die "$f is not a Storybook index"
  done
  count="$(find "$build" -type f | wc -l)"
  ((count <= max_files)) || die "the build has $count files, more than $max_files"
  bad="$(find "$build" -type f -size +"$max_file_bytes"c -print -quit)"
  [[ -z "$bad" ]] || die "${bad#"$build"/} is larger than $max_file_bytes bytes"
  total="$(find "$build" -type f -printf '%s\n' | awk '{ s += $1 } END { print s + 0 }')"
  ((total <= max_total_bytes)) || die "the build has $total bytes, more than $max_total_bytes"

  rm -rf "$dest"
  mkdir -p "$dest/site"
  cp -R "$build/." "$dest/site/"
  cp "$config" "$dest/wrangler.jsonc"
  echo "staged $count files ($total bytes) in $dest/site" >&2
}

# stories SITE MAIN URL [CHANGED] prints the Markdown list of the stories of SITE that MAIN (a build dir, possibly
# empty) does not have (added), or has with another title, name, or file, or whose story or component file is among
# the repository paths listed in CHANGED (changed), each linked on the Storybook at URL. The @my/ui stories are those
# of ui/index.json, under the ref `ui` of the composition.
stories() {
  local site="$1" main="$2" url="$3" changed="${4:-}"
  local -a files=("$site/index.json" "$site/ui/index.json")
  local f
  for f in "$main/index.json" "$main/ui/index.json"; do
    if [[ -f "$f" ]] && jq -e '.entries | type == "object"' "$f" >/dev/null 2>&1; then
      files+=("$f")
    else
      files+=(/dev/null)
    fi
  done
  if [[ -n "$changed" ]]; then files+=("$changed"); else files+=(/dev/null); fi
  jq -nr --arg url "${url%/}" --argjson max "$max_listed" \
    --rawfile pr_web "${files[0]}" --rawfile pr_ui "${files[1]}" \
    --rawfile main_web "${files[2]}" --rawfile main_ui "${files[3]}" --rawfile changed "${files[4]}" '
    # The entries of one index as {id, type, title, name, paths}; ids are those of the composition (ui_<id>).
    def entries($raw; $ref; $dir):
      ($raw | if . == "" then {} else (fromjson? // {}) end | .entries // {})
      | if type == "object" then [.[]] else [] end
      | map(select(type == "object" and (.id | type) == "string" and (.id | test("^[a-z0-9][a-z0-9_-]*$"))))
      | map({
          id: ((if $ref == "" then "" else $ref + "_" end) + .id),
          type: (if .type == "docs" then "docs" else "story" end),
          title: (.title // "" | tostring),
          name: (.name // "" | tostring),
          paths: [.importPath, .componentPath | select(type == "string") | $dir + ltrimstr("./")]
        });
    # Text for a Markdown link: printable, short, and with nothing GitHub would render or notify (mentions, links).
    def text: gsub("[\\p{Cc}]"; " ") | .[0:120] | gsub("(?<c>[\\\\`*_{}\\[\\]()<>#+!|~@:])"; "\\\(.c)");
    def line: "- [\(.title | text) › \(.name | text)](\($url)/?path=/\(.type)/\(.id))";
    (entries($pr_web; ""; "app/web/") + entries($pr_ui; "ui"; "app/ui/")) as $pr
    | (entries($main_web; ""; "app/web/") + entries($main_ui; "ui"; "app/ui/") | map({key: .id, value: .}) | from_entries) as $main
    | ($changed | split("\n") | map(select(. != "")) | map({key: ., value: true}) | from_entries) as $files
    | ($pr | map(select($main[.id] == null))) as $added
    | ($pr | map(select($main[.id] != null and (
        $main[.id] != . or any(.paths[]; $files[.] == true))))) as $updated
    | def section($heading; $list):
        if ($list | length) == 0 then empty
        else "#### \($heading) (\($list | length))", "", ($list[0:$max][] | line),
          (if ($list | length) > $max then "- and \(($list | length) - $max) more" else empty end), ""
        end;
      if ($added | length) + ($updated | length) == 0 then "No story was added or changed compared with main.\n"
      else section("Added"; $added), section("Changed"; $updated)
      end
  '
}

# preview_url reads the URL of the Preview from wrangler's output file.
preview_url() {
  local out="$1"
  case "$method" in
    previews) jq -r 'select(.type == "preview") | .preview_urls[0] // empty' "$out" | tail -n 1 ;;
    versions) jq -r 'select(.type == "version-upload") | .preview_alias_url // empty' "$out" | tail -n 1 ;;
  esac
}

deploy_pr() {
  local build="$1" pr="$2" changed="${3:-}" out url prod main_dir sha body
  check_pr "$pr"
  stage "$build" "$work"
  out="$work/wrangler-output.jsonl"
  : >"$out"
  if [[ -n "$dry_run" ]]; then
    # Checks the config and the assets as the upload would, without an account.
    (cd "$work" && WRANGLER_SEND_METRICS=false npx --yes "wrangler@$wrangler_version" deploy --dry-run --config "$work/wrangler.jsonc")
  fi
  case "$method" in
    previews) WRANGLER_OUTPUT_FILE_PATH="$out" wrangler preview --name "pr-$pr" --message "Pull request #$pr" ;;
    versions) WRANGLER_OUTPUT_FILE_PATH="$out" wrangler versions upload --preview-alias "pr-$pr" --message "Pull request #$pr" ;;
    *) die "UI_PREVIEW_METHOD must be previews or versions, not $method" ;;
  esac
  if [[ -n "$dry_run" ]]; then
    url="https://pr-$pr-my-ui-preview.example.workers.dev"
  else
    url="$(preview_url "$out")"
  fi
  [[ "$url" =~ ^https://pr-$pr-[a-z0-9.-]+$ ]] || die "wrangler gave no Preview URL of pr-$pr (got '$url')"
  # main's Worker: the same host without the pr-<pr>- prefix.
  prod="https://${url#https://pr-"$pr"-}"
  main_dir="$work/main"
  mkdir -p "$main_dir/ui"
  if [[ -n "$dry_run" ]]; then
    if [[ -n "${MAIN_INDEX_DIR:-}" ]]; then
      cp "$MAIN_INDEX_DIR/index.json" "$main_dir/index.json"
      cp "$MAIN_INDEX_DIR/ui/index.json" "$main_dir/ui/index.json"
    fi
  else
    curl -fsS --max-time 30 -o "$main_dir/index.json" "$prod/index.json" || echo "main's Storybook has no index.json yet" >&2
    curl -fsS --max-time 30 -o "$main_dir/ui/index.json" "$prod/ui/index.json" || true
  fi
  sha="${HEAD_SHA:-}"
  [[ -z "$sha" || "$sha" =~ ^[0-9a-f]{40}$ ]] || die "HEAD_SHA is not a commit: $sha"
  body="$work/comment.md"
  {
    echo "$marker"
    echo "### UI preview"
    echo
    echo "The Storybook of this pull request${sha:+ at $sha}: $url"
    echo
    echo "The screens are under **Screens**, the @my/ui components under **@my/ui**. main's Storybook: $prod"
    echo
    stories "$work/site" "$main_dir" "$url" "$changed"
  } >"$body"
  echo "$url"
  echo "wrote $body" >&2
}

deploy_main() {
  stage "$1" "$work"
  if [[ -n "$dry_run" ]]; then
    (cd "$work" && WRANGLER_SEND_METRICS=false npx --yes "wrangler@$wrangler_version" deploy --dry-run --config "$work/wrangler.jsonc")
    return 0
  fi
  wrangler deploy --message "main"
}

delete_pr() {
  local pr="$1"
  check_pr "$pr"
  rm -rf "$work"
  # The config names ./site; an empty one keeps wrangler from looking for the build.
  mkdir -p "$work/site"
  cp "$config" "$work/wrangler.jsonc"
  local note="The preview of this pull request was deleted when it closed."
  case "$method" in
    # A Preview that does not exist (the build never deployed) is not an error.
    previews) wrangler preview delete --name "pr-$pr" --skip-confirmation || echo "no Preview pr-$pr to delete" >&2 ;;
    # A preview alias cannot be deleted; the next upload with the same alias replaces it.
    versions)
      echo "the versions method keeps the preview alias pr-$pr" >&2
      note="This pull request closed: its preview is no longer updated."
      ;;
    *) die "UI_PREVIEW_METHOD must be previews or versions, not $method" ;;
  esac
  {
    echo "$marker"
    echo "### UI preview"
    echo
    echo "$note"
  } >"$work/comment.md"
  echo "wrote $work/comment.md" >&2
}

# comment PR BODY [update] creates the preview comment of the pull request, or updates the one this workflow wrote
# before; with `update`, only updates it.
comment() {
  local pr="$1" body="$2" only_update="${3:-}" repo="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is not set}" id
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
  elif [[ "$only_update" != update ]]; then
    gh api --method POST "repos/$repo/issues/$pr/comments" -F "body=@$body" --silent
  fi
}

cmd="${1:-}"
[[ $# -gt 0 ]] && shift
case "$cmd" in
  stage) [[ $# -eq 2 ]] || usage; stage "$@" ;;
  stories) [[ $# -ge 3 && $# -le 4 ]] || usage; stories "$@" ;;
  deploy-pr) [[ $# -ge 2 && $# -le 3 ]] || usage; deploy_pr "$@" ;;
  deploy-main) [[ $# -eq 1 ]] || usage; deploy_main "$@" ;;
  delete-pr) [[ $# -eq 1 ]] || usage; delete_pr "$@" ;;
  comment) [[ $# -ge 2 && $# -le 3 ]] || usage; comment "$@" ;;
  # Run before the steps that hold the token, so that the package's install scripts never see it.
  install) [[ $# -eq 0 ]] || usage; npx --yes "wrangler@$wrangler_version" --version ;;
  *) usage ;;
esac
