// SPDX-License-Identifier: AGPL-3.0-only

// How the lab judges a run (ADR-0167 §4): a measure over its threshold, the "good" of ADR-0167 §1 (INP 200 ms, LCP
// 2.5 s), or much worse than the same measure of the last run, is a finding, and a run with a finding opens an issue.
// "Much worse" is worse by more than a share of the last value and by more than a floor, both: the share alone would
// flag a few milliseconds of a fast screen, the floor alone the noise of a slow one. The run is the median of a few
// runs of each scenario (measures.ts), and the bundle's size has no noise, so its share is smaller.

import type { Bundle } from "./bundle";
import type { Measure } from "./measures";

/** The report of one run of the lab, the JSON that the next run compares with. */
export type Report = {
  readonly version: 1;
  /** The commit measured, when known. */
  readonly commit: string | null;
  /** When the run ended (ISO 8601). */
  readonly date: string;
  /** The CPU throttling rate the pages ran at (Emulation.setCPUThrottlingRate). */
  readonly cpuThrottling: number;
  readonly measures: readonly Measure[];
  readonly bundles: readonly Bundle[];
  /** The scenarios that failed, whose measures are missing. */
  readonly failed: readonly string[];
};

export type Metric = "lcp" | "inp" | "long_frames" | "bundle";

/** A measure over its threshold, or much worse than the last run. */
export type Finding = {
  /** The scenario; null for a bundle, which is the entry's alone. */
  readonly scenario: string | null;
  readonly entry: string;
  readonly metric: Metric;
  readonly value: number;
  readonly reason: "threshold" | "regression";
  /** The threshold passed, or the last run's value. */
  readonly against: number;
};

/** The "good" of ADR-0167 §1: a value above it is a finding. */
export const thresholds = { lcp: 2500, inp: 200 } as const;

/**
 * What "much worse than the last run" is, for each metric: more than `share` of the last value worse, and more than
 * `floor` worse (ms, or gzipped bytes for the bundle). The long frames are judged by their durations added up.
 */
export const regression: Readonly<Record<Metric, { readonly share: number; readonly floor: number }>> = {
  lcp: { share: 0.2, floor: 250 },
  inp: { share: 0.2, floor: 50 },
  long_frames: { share: 0.2, floor: 100 },
  bundle: { share: 0.05, floor: 10 * 1024 },
};

/** The probe hears no interaction shorter than this (ms): a run that heard none is at most this. */
const quietInteraction = 40;

/** Whether `value` is much worse than `last` by the rule of `metric`. */
export function muchWorse(metric: Metric, value: number, last: number): boolean {
  const { share, floor } = regression[metric];
  return value - last > floor && value > last * (1 + share);
}

/** The findings of `report`, compared with `last`, the report of the run before it, when there is one. */
export function judge(report: Report, last?: Report): Finding[] {
  const findings: Finding[] = [];
  const lastMeasure = (m: Measure) => last?.measures.find((x) => x.scenario === m.scenario && x.entry === m.entry);
  for (const m of report.measures) {
    const before = lastMeasure(m);
    const timed: [Metric, number | null, number | null | undefined][] = [
      ["lcp", m.lcp, before?.lcp],
      ["inp", m.inp ?? quietInteraction, before?.inp ?? quietInteraction],
      ["long_frames", m.longFrames.total, before?.longFrames.total],
    ];
    for (const [metric, value, previous] of timed) {
      if (value === null) continue;
      const threshold = metric === "lcp" || metric === "inp" ? thresholds[metric] : undefined;
      if (threshold !== undefined && value > threshold) {
        findings.push({ scenario: m.scenario, entry: m.entry, metric, value, reason: "threshold", against: threshold });
      } else if (
        previous !== null &&
        previous !== undefined &&
        before !== undefined &&
        muchWorse(metric, value, previous)
      ) {
        findings.push({ scenario: m.scenario, entry: m.entry, metric, value, reason: "regression", against: previous });
      }
    }
  }
  for (const b of report.bundles) {
    const before = last?.bundles.find((x) => x.entry === b.entry);
    if (before !== undefined && muchWorse("bundle", b.gzip, before.gzip)) {
      findings.push({
        scenario: null,
        entry: b.entry,
        metric: "bundle",
        value: b.gzip,
        reason: "regression",
        against: before.gzip,
      });
    }
  }
  return findings;
}
