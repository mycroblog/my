// SPDX-License-Identifier: AGPL-3.0-only

// The report of a run of the lab in Markdown: its findings, the measures of each scenario and entry and the bundle of
// each entry beside the last run's, and how a finding is judged (judge.ts), so that the report says it itself.

import type { Finding, Metric, Report } from "./judge";
import { regression, thresholds } from "./judge";

const names: Readonly<Record<Metric, string>> = {
  lcp: "LCP",
  inp: "INP",
  long_frames: "Long animation frames (total)",
  bundle: "Bundle (gzipped)",
};

const kib = (bytes: number) => `${(bytes / 1024).toFixed(1)} KiB`;

const amount = (metric: Metric, value: number) => (metric === "bundle" ? kib(value) : `${value} ms`);

/** `value`, and how it differs from `last` when there is a last value. */
function compared(value: number | null, last: number | null | undefined, unit: (v: number) => string): string {
  if (value === null) return "-";
  if (last === undefined || last === null || last === 0) return unit(value);
  const change = Math.round(((value - last) / last) * 100);
  return `${unit(value)} (${change >= 0 ? "+" : ""}${change}%)`;
}

const ms = (v: number) => `${v} ms`;

/** The line of a finding. */
function findingLine(f: Finding): string {
  const where = f.scenario === null ? `\`${f.entry}\`` : `${f.scenario}, \`${f.entry}\``;
  const why =
    f.reason === "threshold"
      ? `over the threshold of ${amount(f.metric, f.against)}`
      : `much worse than the last run's ${amount(f.metric, f.against)}`;
  return `- ${where}: ${names[f.metric]} ${amount(f.metric, f.value)}, ${why}`;
}

/** How a finding is judged, as the report states it. */
function rules(): string {
  const r = (m: Metric) =>
    `${names[m]}: more than ${Math.round(regression[m].share * 100)}% and more than ${amount(m, regression[m].floor)} worse`;
  return [
    "A measure is a finding, and the Nightly opens an issue for it, when it is",
    "",
    `- over its threshold, the "good" of Core Web Vitals: LCP over ${thresholds.lcp} ms, INP over ${thresholds.inp} ms; or`,
    "- much worse than the same measure of the last Nightly, both by a share of the last value and by a floor:",
    ...(["lcp", "inp", "long_frames", "bundle"] as const).map((m) => `  - ${r(m)}`),
    "",
    "Each measure is the median of the runs of its scenario. INP is the worst interaction of a run; an interaction under",
    "40 ms is not heard, and a run that heard none counts as 40 ms.",
  ].join("\n");
}

/** The report of `report` in Markdown, with its `findings` and the last run's report `last`. */
export function markdown(report: Report, findings: readonly Finding[], last?: Report): string {
  const lines: string[] = ["# UX in the lab", ""];
  const runs = Math.max(0, ...report.measures.map((m) => m.runs));
  lines.push(
    `Commit ${report.commit ?? "unknown"}, ${report.date}. The CPU throttled ${report.cpuThrottling}×; each measure is the median of ${runs} run${runs === 1 ? "" : "s"}.`,
    last === undefined
      ? "No last run to compare with: only the thresholds are judged."
      : `Compared with the last run: commit ${last.commit ?? "unknown"}, ${last.date}.`,
    "",
    "## Findings",
    "",
  );
  lines.push(...(findings.length === 0 ? ["None."] : findings.map(findingLine)), "");
  if (report.failed.length > 0) {
    lines.push("## Scenarios that failed", "", "Their measures are missing:", "");
    lines.push(...report.failed.map((s) => `- ${s}`), "");
  }
  lines.push(
    "## Measures",
    "",
    "| Scenario | Entry | LCP | INP | Long animation frames (count, total, longest) |",
    "| --- | --- | --- | --- | --- |",
  );
  for (const m of report.measures) {
    const before = last?.measures.find((x) => x.scenario === m.scenario && x.entry === m.entry);
    const frames = `${m.longFrames.count}, ${compared(m.longFrames.total, before?.longFrames.total, ms)}, ${m.longFrames.longest} ms`;
    lines.push(
      `| ${m.scenario} | \`${m.entry}\` | ${compared(m.lcp, before?.lcp, ms)} | ${compared(m.inp, before?.inp, ms)} | ${frames} |`,
    );
  }
  lines.push("", "## Bundles", "", "| Entry | Files | Size | Gzipped |", "| --- | --- | --- | --- |");
  for (const b of report.bundles) {
    const before = last?.bundles.find((x) => x.entry === b.entry);
    lines.push(`| \`${b.entry}\` | ${b.files} | ${kib(b.bytes)} | ${compared(b.gzip, before?.gzip, kib)} |`);
  }
  lines.push("", "## How a finding is judged", "", rules(), "");
  return lines.join("\n");
}
