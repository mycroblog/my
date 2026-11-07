#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Reports a full Nightly run on main (ADR-0079 §8: a red Nightly stops the line). When a checked job failed, it opens
# the issue of the failing Nightly, or adds a comment to the one already open, with the failed jobs and the commits
# since the last green run. When every checked job passed, it closes that issue. It also writes the E2E tests that
# passed only on a retry (flaky candidates) to the step summary.
#
#   scripts/nightly-report.sh
#
# Environment:
#   RESULTS      the `needs` of the calling job as JSON ({"slow-suite": {"result": "failure"}, ...})
#   RUN_ID       the run of the Nightly; its failed jobs are read from the API
#   SHA          the commit the run checked
#   SINCE        the commit of the last successful full run, empty when there is none
#   FLAKY        a file of the flaky tests, one per line (optional; written by web-e2e.yml)
#   GH_REPO      owner/name
#   GITHUB_STEP_SUMMARY
# gh must be on PATH with a token that may write issues.
set -euo pipefail

label="nightly-failure"
summary="${GITHUB_STEP_SUMMARY:-/dev/stdout}"
run_url="https://github.com/$GH_REPO/actions/runs/$RUN_ID"

# A cancelled job is a newer Nightly replacing this one, not a failure of main.
failed_needs="$(jq -r 'to_entries[] | select(.value.result == "failure") | .key' <<< "$RESULTS")"
passed=true
if [ -n "$failed_needs" ]; then passed=false; fi
if jq -e 'any(.[]; .result == "cancelled")' <<< "$RESULTS" > /dev/null; then
  echo "A job was cancelled: nothing is reported." | tee -a "$summary"
  exit 0
fi

if [ -n "${FLAKY:-}" ] && [ -s "$FLAKY" ]; then
  {
    echo "### Flaky candidates"
    echo
    echo "These E2E tests failed and then passed on a retry:"
    echo
    sed 's/^/- /' "$FLAKY"
  } >> "$summary"
fi

open_issue="$(gh issue list --repo "$GH_REPO" --label "$label" --state open --json number --jq '.[0].number // ""')"

if [ "$passed" = true ]; then
  if [ -n "$open_issue" ]; then
    gh issue comment "$open_issue" --repo "$GH_REPO" --body "The Nightly passed on $SHA ($run_url)."
    gh issue close "$open_issue" --repo "$GH_REPO"
    echo "Closed #$open_issue: the Nightly passed." >> "$summary"
  fi
  exit 0
fi

jobs="$(gh api "repos/$GH_REPO/actions/runs/$RUN_ID/jobs?per_page=100" --paginate \
  --jq '.jobs[] | select(.conclusion == "failure") | "- [\(.name)](\(.html_url))"')"
if [ -n "${SINCE:-}" ]; then
  changes="Commits since the last green Nightly: https://github.com/$GH_REPO/compare/$SINCE...$SHA"
else
  changes="This run does not know the last green Nightly (a run by hand): compare with the latest green run of nightly.yml."
fi
body="The Nightly failed on $SHA: $run_url

Failed jobs:

${jobs:-$(while read -r job; do echo "- $job"; done <<< "$failed_needs")}

$changes

Fix it before starting more issues (docs/agents/workflow.md, \"Failures on main\"). This issue closes itself when a Nightly passes."

if [ -n "$open_issue" ]; then
  gh issue comment "$open_issue" --repo "$GH_REPO" --body "$body"
  echo "Commented on #$open_issue." >> "$summary"
else
  gh label create "$label" --repo "$GH_REPO" --color B60205 \
    --description "The Nightly on main fails; closed by the next green Nightly" > /dev/null 2>&1 || true
  url="$(gh issue create --repo "$GH_REPO" --title "[Bug] The Nightly fails on main" --label "$label" --label type:bug \
    --label priority:p1 --body "$body")"
  echo "Opened $url." >> "$summary"
fi
