#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/nightly-ux-report.sh with a fake gh that records its calls; it reaches no GitHub API.
#
#   scripts/nightly-ux-report-test.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/scripts/nightly-ux-report.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# The fake gh appends each call to $work/calls; `issue list` prints $OPEN_ISSUE and `issue create` a URL.
mkdir -p "$work/bin"
cat > "$work/bin/gh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$CALLS"
case "$1 $2" in
  "issue list") printf '%s\n' "$OPEN_ISSUE" ;;
  "issue create") echo "https://github.com/o/r/issues/99" ;;
esac
EOF
chmod +x "$work/bin/gh"

failed=0
fail() {
  echo "FAIL: $*" >&2
  failed=1
}

# report FINDINGS FAILED: writes a report whose findings and failed scenarios are the JSON arrays given.
report() {
  rm -rf "$work/report"
  mkdir -p "$work/report"
  printf '{"findings":%s,"failed":%s}\n' "$1" "$2" > "$work/report/report.json"
  # shellcheck disable=SC2016 # the backquotes are Markdown
  printf '# UX in the lab\n\n## Findings\n\n- home, `index.html`: INP 208 ms, over the threshold of 200 ms\n' > "$work/report/report.md"
}

# run NAME OUTCOME OPEN_ISSUE [REF]: runs the script and leaves its calls in $calls and its summary in $out.
run() {
  local name="$1"
  : > "$work/calls"
  : > "$work/summary"
  if ! PATH="$work/bin:$PATH" CALLS="$work/calls" OUTCOME="$2" OPEN_ISSUE="$3" REF="${4:-refs/heads/main}" \
    REPORT="$work/report" RUN_ID=7 SHA=abc GH_REPO=o/r GITHUB_STEP_SUMMARY="$work/summary" \
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

finding='[{"metric":"inp"}]'

report "$finding" '[]'
run "a first finding" success ""
expect_has "a first finding" "$calls" "issue create" "--label ux:lab-regression" "INP 208 ms" "actions/runs/7"
expect_not "a first finding" "$calls" "ci:nightly-failure" "issue comment" "issue close"
expect_has "a first finding" "$out" "## Findings" "Opened https://github.com/o/r/issues/99."

run "a finding with an issue open" success "12"
expect_has "a finding with an issue open" "$calls" "issue comment 12" "INP 208 ms"
expect_not "a finding with an issue open" "$calls" "issue create"

report '[]' '[]'
run "no finding" success "12"
expect_not "no finding" "$calls" "issue create" "issue comment" "issue close"
expect_has "no finding" "$out" "No finding."

report '[]' '["notifications"]'
run "a scenario that failed" failure ""
expect_has "a scenario that failed" "$calls" "issue create" "--label ux:lab-regression"

rm -rf "$work/report"
mkdir -p "$work/report"
run "no report" failure ""
expect_has "no report" "$calls" "issue create" "The lab wrote no report"

report "$finding" '[]'
run "a finding on a branch" success "" refs/heads/feat/x
expect_not "a finding on a branch" "$calls" "issue create" "issue comment"
expect_has "a finding on a branch" "$out" "not main: no issue"

run "a cancelled run" cancelled ""
expect_not "a cancelled run" "$calls" "issue create" "issue comment"

if [ "$failed" -ne 0 ]; then
  exit 1
fi
echo "nightly-ux-report: ok"
