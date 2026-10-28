// SPDX-License-Identifier: AGPL-3.0-only
// Files the issues of an L4 run (report.ts) with `gh`: one open issue per screen (tasks over the threshold or not completed; unavailable ones only in the summary), created with the label `area:web`
// the first time and commented on by later runs (found by its exact title). `scripts/task.sh web:l4-issues` runs it;
// --dry-run prints the issues instead.
//
//   node l4/issues.ts [--results <file>] [--dry-run]
//
// GH_REPO names the repository (default mycroblog/my); GITHUB_SERVER_URL, GITHUB_REPOSITORY and GITHUB_RUN_ID, set
// in GitHub Actions, link the run.
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { parseArgs } from "node:util";
import { issueBody, issueTitle, lostByScreen, skippedSummary, skippedTasks } from "./report.ts";
import type { RunResult } from "./results.ts";

const root = resolve(import.meta.dirname, "../../..");
const { values } = parseArgs({
  options: {
    results: { type: "string", default: join(root, ".tmp/l4/results.json") },
    "dry-run": { type: "boolean", default: false },
  },
});

const repo = process.env["GH_REPO"] ?? "mycroblog/my";
const runID = process.env["GITHUB_RUN_ID"];
const runURL =
  runID === undefined
    ? ""
    : `${process.env["GITHUB_SERVER_URL"] ?? "https://github.com"}/${process.env["GITHUB_REPOSITORY"] ?? repo}/actions/runs/${runID}`;

function gh(...args: string[]): string {
  return execFileSync("gh", ["--repo", repo, ...args], { encoding: "utf8" }).trim();
}

const result = JSON.parse(readFileSync(resolve(values.results), "utf8")) as RunResult;
const lost = lostByScreen(result);
if (result.dryRun) {
  console.log("l4-issues: the results are of a dry run; no task was measured.");
}
if (lost.size === 0) {
  console.log(`l4-issues: no task over lostness ${String(result.threshold)} or not completed.`);
}
const skipped = skippedTasks(result);
if (skipped.length > 0) {
  console.log(`l4-issues: ${String(skipped.length)} task(s) not measured, no issue filed:`);
  console.log(skippedSummary(skipped).join("\n"));
}
for (const tasks of lost.values()) {
  const [first] = tasks;
  if (first === undefined) {
    continue;
  }
  const title = issueTitle(first);
  const body = issueBody(result, tasks, runURL, repo);
  if (values["dry-run"]) {
    console.log(`l4-issues: would file or update "${title}":\n\n${body}`);
    continue;
  }
  const open = JSON.parse(
    gh(
      "issue",
      "list",
      "--state",
      "open",
      "--search",
      `"${title}" in:title`,
      "--json",
      "number,title",
      "--limit",
      "100",
    ),
  ) as { number: number; title: string }[];
  const existing = open.find((issue) => issue.title === title);
  if (existing === undefined) {
    console.log(`l4-issues: ${gh("issue", "create", "--title", title, "--label", "area:web", "--body", body)}`);
  } else {
    gh("issue", "comment", String(existing.number), "--body", body);
    console.log(`l4-issues: updated #${String(existing.number)} ${title}`);
  }
}
