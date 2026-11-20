// SPDX-License-Identifier: AGPL-3.0-only

// The reporter of the lab: it adds up the runs each test attached (test.ts), takes the bundle of each entry from the
// build's manifest, judges the run against the last one's report (UX_LAST names its report.json; none, and only the
// thresholds are judged), and writes report.json and report.md to UX_REPORT_DIR (.tmp/ux by default), printing the
// Markdown. A finding fails nothing: the Nightly opens an issue for it (.github/workflows/nightly.yml, web-ux).

import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { Reporter, TestCase, TestResult } from "@playwright/test/reporter";
import { bundles, type Manifest } from "../src/ux/bundle";
import { judge, type Report } from "../src/ux/judge";
import { summarize, type Run } from "../src/ux/measures";
import { markdown } from "../src/ux/report";
import { root } from "../src/server/paths";
import { buildDir, cpuThrottling } from "./lab";

function commit(): string | null {
  if (process.env["GITHUB_SHA"] !== undefined) return process.env["GITHUB_SHA"];
  try {
    return execFileSync("git", ["rev-parse", "HEAD"], { cwd: root, encoding: "utf8" }).trim();
  } catch {
    return null;
  }
}

export default class UXReporter implements Reporter {
  private readonly runs: Run[] = [];
  private readonly failed = new Set<string>();

  onTestEnd(test: TestCase, result: TestResult): void {
    if (result.status !== "passed") {
      this.failed.add(test.title);
      return;
    }
    const attachment = result.attachments.find((a) => a.name === "ux-runs");
    if (attachment?.body !== undefined) this.runs.push(...(JSON.parse(attachment.body.toString("utf8")) as Run[]));
  }

  onEnd(): void {
    const manifest = JSON.parse(readFileSync(join(buildDir, ".vite/manifest.json"), "utf8")) as Manifest;
    const report: Report = {
      version: 1,
      commit: commit(),
      date: new Date().toISOString(),
      cpuThrottling,
      measures: summarize(this.runs),
      bundles: bundles(manifest, (file) => readFileSync(join(buildDir, file))),
      failed: [...this.failed].sort(),
    };
    const lastFile = process.env["UX_LAST"];
    const last =
      lastFile !== undefined && lastFile !== "" && existsSync(lastFile)
        ? (JSON.parse(readFileSync(lastFile, "utf8")) as Report)
        : undefined;
    const findings = judge(report, last);
    const dir = process.env["UX_REPORT_DIR"] ?? join(root, ".tmp/ux");
    mkdirSync(dir, { recursive: true });
    const text = markdown(report, findings, last);
    writeFileSync(join(dir, "report.json"), `${JSON.stringify({ ...report, findings }, null, 2)}\n`);
    writeFileSync(join(dir, "report.md"), text);
    console.log(`\n${text}\nThe report is in ${dir} (report.md, report.json).`);
  }

  printsToStdio(): boolean {
    return false;
  }
}
