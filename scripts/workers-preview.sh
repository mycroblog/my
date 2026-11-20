# shellcheck shell=bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Sourced by scripts/docs-site.sh and scripts/ui-preview.sh, which define die(): what both sites share of a Cloudflare
# Worker of static assets and its Workers Previews pr-<number>, one for each pull request. The pinned wrangler on Bun,
# the limits of Workers static assets, the checks that a pull request's build is plain files, the Preview URL that
# wrangler reports, the deletion of the Previews of closed pull requests, and the one comment of a pull request.
# DRY_RUN=1 prints the commands that change a Worker or a comment instead of running them.

# Fetched by its exact version, since only the CI jobs run it, and run on Bun (ADR-0025 §10).
wrangler_version="4.151.0"

# The limits of Workers static assets on the free plan, and a bound on the upload of a preview.
max_files=20000
max_file_bytes=$((25 * 1024 * 1024))
max_preview_bytes=$((300 * 1024 * 1024))

# wrangler_install fetches the pinned wrangler into Bun's cache. Run before the steps that hold the token, so that the
# install scripts of its packages never see it.
wrangler_install() {
  bunx --bun "wrangler@$wrangler_version" --version
}

# wrangler_in DIR ARGS... runs the pinned wrangler on the config DIR/wrangler.jsonc, from DIR (which holds only the
# config, the site, and the Worker's code if it has any), with telemetry off.
wrangler_in() {
  local dir="$1"
  shift
  (cd "$dir" && WRANGLER_SEND_METRICS=false bunx --bun "wrangler@$wrangler_version" "$@" --config "$dir/wrangler.jsonc")
}

# wrangler_or_print DIR ARGS... is wrangler_in, which a dry run only prints.
wrangler_or_print() {
  if [[ -n "${DRY_RUN:-}" ]]; then
    echo "dry run: wrangler ${*:2} --config $1/wrangler.jsonc" >&2
    return 0
  fi
  wrangler_in "$@"
}

# worker_name CONFIG prints the name of the Worker of the wrangler config CONFIG.
worker_name() {
  grep -v '^[[:space:]]*//' "$1" | jq -r .name
}

check_pr() {
  [[ "$1" =~ ^[1-9][0-9]{0,8}$ ]] || die "not a pull request number: $1"
}

# check_assets SITE fails when SITE exceeds the limits of Workers static assets.
check_assets() {
  local site="$1" count bad
  count="$(find "$site" -type f | wc -l)"
  ((count <= max_files)) || die "the site has $count files, more than $max_files"
  bad="$(find "$site" -type f -size +"$max_file_bytes"c -print -quit)"
  [[ -z "$bad" ]] || die "${bad#"$site"/} is larger than $max_file_bytes bytes"
}

# check_plain BUILD fails unless BUILD, a pull request's build, is plain static files within the limits: no link, no
# name a shell or a URL would read differently, nothing Workers static assets read as configuration. It prints the
# total size in bytes.
check_plain() {
  local build="$1" bad total
  [[ -d "$build" ]] || die "no build directory: $build"
  bad="$(find "$build" -mindepth 1 ! -type f ! -type d -print -quit)"
  [[ -z "$bad" ]] || die "the build has a file that is not a regular file or a directory (a link?): ${bad#"$build"/}"
  # Every name is plain: no dot files (.assetsignore, .wrangler), nothing a shell or a URL would read differently.
  bad="$(find "$build" -mindepth 1 -printf '%P\n' |
    grep -vE '^[A-Za-z0-9_@+~-][A-Za-z0-9._@+~-]*(/[A-Za-z0-9_@+~-][A-Za-z0-9._@+~-]*)*$' | head -n 1)" || true
  [[ -z "$bad" ]] || die "the build has a name outside [A-Za-z0-9._@+~-] or starting with a dot: $bad"
  # Workers static assets read these at the root as configuration; a preview takes none of them from a build.
  bad="$(find "$build" -mindepth 1 \( -name _headers -o -name _redirects -o -name _worker.js -o -name _routes.json \) \
    -printf '%P\n' -quit)"
  [[ -z "$bad" ]] || die "the build has $bad, which the Worker would read as configuration"
  check_assets "$build"
  total="$(find "$build" -type f -printf '%s\n' | awk '{ s += $1 } END { print s + 0 }')"
  ((total <= max_preview_bytes)) || die "the build has $total bytes, more than $max_preview_bytes"
  echo "$total"
}

# deploy_preview STAGE PR publishes the staged Worker STAGE (wrangler.jsonc and site/) on its Preview pr-<PR> and
# prints the Preview's URL. A dry run checks the config and the assets as the upload would, without an account, and
# prints https://pr-<PR>-<worker>.example.workers.dev.
deploy_preview() {
  local stage="$1" pr="$2" out url
  check_pr "$pr"
  out="$stage.wrangler-output.jsonl"
  : >"$out"
  if [[ -n "${DRY_RUN:-}" ]]; then
    wrangler_in "$stage" deploy --dry-run >&2 || die "wrangler deploy --dry-run failed"
    echo "dry run: wrangler preview --name pr-$pr --message Pull request #$pr --config $stage/wrangler.jsonc" >&2
    url="https://pr-$pr-$(worker_name "$stage/wrangler.jsonc").example.workers.dev"
  else
    WRANGLER_OUTPUT_FILE_PATH="$out" wrangler_in "$stage" preview --name "pr-$pr" --message "Pull request #$pr" >&2 ||
      die "wrangler preview failed"
    url="$(jq -r 'select(.type == "preview") | .preview_urls[0] // empty' "$out" | tail -n 1)"
  fi
  url="${url%/}"
  [[ "$url" =~ ^https://pr-$pr-[a-z0-9.-]+$ ]] || die "wrangler gave no Preview URL of pr-$pr (got '$url')"
  echo "$url"
}

# delete_closed_previews STAGE deletes the Preview pr-<number> of each closed or merged pull request of the Worker of
# STAGE/wrangler.jsonc. The Previews come from the Cloudflare API (wrangler has no list command) and the state of each
# pull request from the GitHub API, so a Preview whose pull request is open, or is not one, stays.
delete_closed_previews() {
  local stage="$1" repo="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is not set}" worker page=1 body page_names
  local names="" name pr state deleted=0
  worker="$(worker_name "$stage/wrangler.jsonc")"
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
  while IFS= read -r name; do
    [[ "$name" =~ ^pr-([1-9][0-9]{0,8})$ ]] || continue
    pr="${BASH_REMATCH[1]}"
    if ! state="$(gh api "repos/$repo/pulls/$pr" --jq .state)"; then
      echo "$worker: cannot read pull request $pr; keeping $name" >&2
      continue
    fi
    if [[ "$state" == closed ]]; then
      echo "$worker: deleting $name: pull request $pr is closed" >&2
      wrangler_or_print "$stage" preview delete --name "$name" --skip-confirmation || echo "$worker: could not delete $name" >&2
      deleted=$((deleted + 1))
    else
      echo "$worker: keeping $name: pull request $pr is $state" >&2
    fi
  done < <(sort -u <<<"$names")
  echo "$worker: closed pull requests with a Preview: $deleted" >&2
}

# upsert_comment PR BODY MARKER creates the comment of the pull request that starts with MARKER, or updates the one
# the workflow wrote before.
upsert_comment() {
  local pr="$1" body="$2" marker="$3" repo="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is not set}" id
  check_pr "$pr"
  [[ -f "$body" ]] || die "no comment body: $body"
  if [[ -n "${DRY_RUN:-}" ]]; then
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
