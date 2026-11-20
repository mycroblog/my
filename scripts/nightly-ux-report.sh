#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Reports the UX in the lab of a Nightly (ADR-0167 §4; nightly.yml's web-ux job, `scripts/task.sh web:e2e-ux`): adds
# the report to the run's summary, and, on main, when the run has a finding (a measure over its threshold or much
# worse than the last Nightly's) or could not measure, opens the issue labelled ux:lab-regression, or comments on the
# one already open, with the report. That label is not ci:nightly-failure: a measure's noise never stops the line. It
# never closes the issue: the next run compares with this one, so a regression it reported is no longer one to it.
#
#   scripts/nightly-ux-report.sh
#
# Environment:
#   REPORT       the directory of report.json and report.md
#   OUTCOME      the outcome of the step that measured: success, failure, cancelled or skipped
#   RUN_ID       the run of the Nightly
#   SHA          the commit it measured
#   REF          the ref it ran on; only refs/heads/main opens an issue
#   GH_REPO      owner/name
#   GITHUB_STEP_SUMMARY
# gh must be on PATH with a token that may write issues.
set -euo pipefail

label="ux:lab-regression"
summary="${GITHUB_STEP_SUMMARY:-/dev/stdout}"
run_url="https://github.com/$GH_REPO/actions/runs/$RUN_ID"
json="$REPORT/report.json"
md="$REPORT/report.md"

if [ "$OUTCOME" = cancelled ] || [ "$OUTCOME" = skipped ]; then
  echo "The lab did not run to its end ($OUTCOME): nothing is reported." | tee -a "$summary"
  exit 0
fi

if [ -f "$md" ]; then cat "$md" >> "$summary"; fi

problem=""
if [ ! -f "$json" ]; then
  problem="The lab wrote no report: it failed before its scenarios ended. See the run."
elif [ "$(jq '(.findings | length) + (.failed | length)' "$json")" -gt 0 ]; then
  problem="$(tail -n +2 "$md")"
elif [ "$OUTCOME" != success ]; then
  problem="The lab failed after it wrote a report with no finding. See the run."$'\n'"$(tail -n +2 "$md")"
fi

if [ -z "$problem" ]; then
  echo "No finding." >> "$summary"
  exit 0
fi
if [ "$REF" != refs/heads/main ]; then
  echo "A finding on $REF, not main: no issue." >> "$summary"
  exit 0
fi

body="The UX in the lab on $SHA: $run_url (its artifact ux-report holds report.md and report.json).
$problem

Close this issue once the finding is fixed or accepted: no run closes it."

open_issue="$(gh issue list --repo "$GH_REPO" --label "$label" --state open --json number --jq '.[0].number // ""')"
if [ -n "$open_issue" ]; then
  gh issue comment "$open_issue" --repo "$GH_REPO" --body "$body"
  echo "Commented on #$open_issue." >> "$summary"
else
  gh label create "$label" --repo "$GH_REPO" --color FBCA04 \
    --description "The UX in the lab of a Nightly is over its threshold or much worse than the last run" > /dev/null 2>&1 || true
  url="$(gh issue create --repo "$GH_REPO" --title "The UX in the lab got worse" --label "$label" --body "$body")"
  echo "Opened $url." >> "$summary"
fi
