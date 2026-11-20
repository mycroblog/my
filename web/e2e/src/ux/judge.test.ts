// SPDX-License-Identifier: AGPL-3.0-only

// How the lab judges a run (judge.ts): a measure over its threshold, or much worse than the last run's by both the
// share and the floor of its metric, is a finding; a measure the last run has not is judged by its threshold alone.

import { describe, expect, test } from "bun:test";
import { judge, muchWorse, type Report } from "./judge";
import type { Measure } from "./measures";

const measure = (over: Partial<Measure> = {}): Measure => ({
  scenario: "home",
  entry: "index.html",
  runs: 3,
  lcp: 1000,
  inp: 100,
  longFrames: { count: 2, total: 200, longest: 120 },
  ...over,
});

const report = (measures: Measure[], gzip = 200 * 1024): Report => ({
  version: 1,
  commit: "abc",
  date: "2026-11-20T18:00:00.000Z",
  cpuThrottling: 4,
  measures,
  bundles: [{ entry: "index.html", files: 10, bytes: 3 * gzip, gzip }],
  failed: [],
});

describe("muchWorse", () => {
  test("needs both the share and the floor", () => {
    // LCP: 20 % and 250 ms.
    expect(muchWorse("lcp", 1300, 1000)).toBe(true);
    expect(muchWorse("lcp", 1250, 1000)).toBe(false); // 25 % but only 250 ms
    expect(muchWorse("lcp", 6000, 5500)).toBe(false); // 500 ms but only 9 %
    // INP: 20 % and 50 ms.
    expect(muchWorse("inp", 60, 40)).toBe(false); // 50 % but 20 ms
    expect(muchWorse("inp", 160, 100)).toBe(true);
    // The bundle: 5 % and 10 KiB.
    expect(muchWorse("bundle", 220 * 1024, 200 * 1024)).toBe(true);
    expect(muchWorse("bundle", 209 * 1024, 200 * 1024)).toBe(false);
  });

  test("a better value is never worse", () => {
    expect(muchWorse("long_frames", 100, 900)).toBe(false);
  });
});

describe("judge", () => {
  test("a run within the thresholds and close to the last run has no finding", () => {
    expect(judge(report([measure({ lcp: 1100, inp: 110 })]), report([measure()]))).toEqual([]);
  });

  test("a measure over its threshold is a finding, with no last run", () => {
    expect(judge(report([measure({ lcp: 2600, inp: 201 })]))).toEqual([
      { scenario: "home", entry: "index.html", metric: "lcp", value: 2600, reason: "threshold", against: 2500 },
      { scenario: "home", entry: "index.html", metric: "inp", value: 201, reason: "threshold", against: 200 },
    ]);
  });

  test("the thresholds are the good of Core Web Vitals, inclusive", () => {
    expect(judge(report([measure({ lcp: 2500, inp: 200 })]))).toEqual([]);
  });

  test("a measure much worse than the last run's is a regression", () => {
    const now = report([measure({ lcp: 1600, longFrames: { count: 5, total: 600, longest: 200 } })], 240 * 1024);
    expect(judge(now, report([measure()]))).toEqual([
      { scenario: "home", entry: "index.html", metric: "lcp", value: 1600, reason: "regression", against: 1000 },
      { scenario: "home", entry: "index.html", metric: "long_frames", value: 600, reason: "regression", against: 200 },
      {
        scenario: null,
        entry: "index.html",
        metric: "bundle",
        value: 240 * 1024,
        reason: "regression",
        against: 200 * 1024,
      },
    ]);
  });

  test("a measure over its threshold is reported once, as over the threshold", () => {
    const findings = judge(report([measure({ lcp: 4000 })]), report([measure()]));
    expect(findings).toEqual([
      { scenario: "home", entry: "index.html", metric: "lcp", value: 4000, reason: "threshold", against: 2500 },
    ]);
  });

  test("a scenario the last run did not measure is judged by its thresholds alone", () => {
    const now = report([measure({ scenario: "new", lcp: 2000, inp: 190 })]);
    expect(judge(now, report([measure({ lcp: 500, inp: 40 })]))).toEqual([]);
  });

  test("a run that heard no interaction counts as the shortest the probe hears", () => {
    // None last time, 95 ms now: 55 ms worse than 40 ms, and more than 20 % worse.
    expect(judge(report([measure({ inp: 95 })]), report([measure({ inp: null })]))).toEqual([
      { scenario: "home", entry: "index.html", metric: "inp", value: 95, reason: "regression", against: 40 },
    ]);
    expect(judge(report([measure({ inp: null })]), report([measure()]))).toEqual([]);
  });

  test("a run with no LCP judges none", () => {
    expect(judge(report([measure({ lcp: null })]), report([measure()]))).toEqual([]);
  });
});
