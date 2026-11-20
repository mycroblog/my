// SPDX-License-Identifier: AGPL-3.0-only

// The Markdown of a run (report.ts): its findings, its measures beside the last run's, and how a finding is judged.

import { describe, expect, test } from "bun:test";
import { judge, type Report } from "./judge";
import type { Measure } from "./measures";
import { markdown } from "./report";

const home: Measure = {
  scenario: "home",
  entry: "index.html",
  runs: 3,
  lcp: 1000,
  inp: 100,
  longFrames: { count: 2, total: 200, longest: 120 },
};

const base: Report = {
  version: 1,
  commit: "abc123",
  date: "2026-11-20T18:00:00.000Z",
  cpuThrottling: 4,
  measures: [home],
  bundles: [{ entry: "index.html", files: 12, bytes: 600 * 1024, gzip: 200 * 1024 }],
  failed: [],
};

describe("markdown", () => {
  test("states the findings, the measures beside the last run's, and how a finding is judged", () => {
    const now: Report = {
      ...base,
      commit: "def456",
      measures: [{ ...home, lcp: 2600 }],
      failed: ["notifications"],
    };
    const text = markdown(now, judge(now, base), base);
    expect(text).toContain("Commit def456");
    expect(text).toContain("The CPU throttled 4×; each measure is the median of 3 runs.");
    expect(text).toContain("Compared with the last run: commit abc123");
    expect(text).toContain("- home, `index.html`: LCP 2600 ms, over the threshold of 2500 ms");
    expect(text).toContain("- notifications");
    expect(text).toContain("| home | `index.html` | 2600 ms (+160%) | 100 ms (+0%) | 2, 200 ms (+0%), 120 ms |");
    expect(text).toContain("| `index.html` | 12 | 600.0 KiB | 200.0 KiB (+0%) |");
    expect(text).toContain("LCP over 2500 ms, INP over 200 ms");
    expect(text).toContain("LCP: more than 20% and more than 250 ms worse");
    expect(text).toContain("Bundle (gzipped): more than 5% and more than 10.0 KiB worse");
  });

  test("says when there is no last run, and that a run with no finding has none", () => {
    const text = markdown(base, judge(base));
    expect(text).toContain("No last run to compare with: only the thresholds are judged.");
    expect(text).toContain("## Findings\n\nNone.");
    expect(text).toContain("| home | `index.html` | 1000 ms | 100 ms | 2, 200 ms, 120 ms |");
  });
});
