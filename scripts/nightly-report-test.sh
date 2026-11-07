#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/nightly-report.sh with a fake gh that records its calls; it reaches no GitHub API.
#
#   scripts/nightly-report-test.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/scripts/nightly-report.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# The fake gh appends each call to $work/calls. `issue list` prints $OPEN_ISSUE, `api` prints $FAILED_JOBS, and
# `issue create` prints a URL.
mkdir -p "$work/bin"
cat > "$work/bin/gh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$CALLS"
case "$1 $2" in
  "issue list") printf '%s\n' "$OPEN_ISSUE" ;;
  "issue create") echo "https://github.com/o/r/issues/99" ;;
  "api "*) printf '%s' "$FAILED_JOBS" ;;
esac
EOF
chmod +x "$work/bin/gh"

failed=0
fail() {
  echo "FAIL: $*" >&2
  failed=1
}

# run NAME RESULTS OPEN_ISSUE: runs the script and leaves its calls in $calls and its summary in $out.
run() {
  local name="$1"
  : > "$work/calls"
  : > "$work/summary"
  if ! PATH="$work/bin:$PATH" CALLS="$work/calls" RESULTS="$2" OPEN_ISSUE="$3" \
    FAILED_JOBS=$'- [slow-suite / web-e2e / shard 1/8](https://example.test/j/1)\n' \
    RUN_ID=7 SHA=abc SINCE=def GH_REPO=o/r FLAKY="${FLAKY:-}" GITHUB_STEP_SUMMARY="$work/summary" \
    bash "$script" > "$work/stdout" 2>&1; then
    fail "$name: the script failed: $(cat "$work/stdout")"
  fi
  calls="$(cat "$work/calls")"
  out="$(cat "$work/summary" "$work/stdout")"
}

expect_has() {
  local name="$1" text="$2" want
  shift 2
  for want in "$@"; do
    [[ "$text" == *"$want"* ]] || fail "$name: no '$want' in:"$'\n'"$text"
  done
}

expect_not() {
  local name="$1" text="$2" unwanted
  shift 2
  for unwanted in "$@"; do
    [[ "$text" != *"$unwanted"* ]] || fail "$name: '$unwanted' in:"$'\n'"$text"
  done
}

red='{"slow-suite":{"result":"failure"},"demo-e2e":{"result":"success"}}'
green='{"slow-suite":{"result":"success"},"demo-e2e":{"result":"success"}}'

run "a first failure" "$red" ""
expect_has "a first failure" "$calls" "issue create" "--label nightly-failure" "shard 1/8" "compare/def...abc" "actions/runs/7"
expect_not "a first failure" "$calls" "issue comment" "issue close "

run "a failure that goes on" "$red" "12"
expect_has "a failure that goes on" "$calls" "issue comment 12" "shard 1/8"
expect_not "a failure that goes on" "$calls" "issue create" "issue close "

run "green again" "$green" "12"
expect_has "green again" "$calls" "issue comment 12" "issue close 12"
expect_not "green again" "$calls" "issue create"

run "green with nothing open" "$green" ""
expect_not "green with nothing open" "$calls" "issue create" "issue comment" "issue close "

run "a cancelled run" '{"slow-suite":{"result":"cancelled"},"demo-e2e":{"result":"failure"}}' "12"
expect_not "a cancelled run" "$calls" "issue create" "issue comment" "issue close "

printf 'app/web/e2e/compose.spec.ts › posts a status (en-320)\n' > "$work/flaky"
FLAKY="$work/flaky" run "flaky candidates" "$green" ""
expect_has "flaky candidates" "$out" "Flaky candidates" "- app/web/e2e/compose.spec.ts › posts a status (en-320)"

if [ "$failed" -ne 0 ]; then
  exit 1
fi
echo "nightly-report: ok"
