// SPDX-License-Identifier: AGPL-3.0-only

// The runs of a scenario from what the probe heard (measures.ts): each document by the entry its module script names,
// the worst LCP and interaction of each entry, its long frames added up, and the median of the runs.

import { describe, expect, test } from "bun:test";
import { entryOfScripts, median, runsOf, summarize, type Run, type Sample } from "./measures";

const manifest = {
  "index.html": { file: "assets/index-a1.js", isEntry: true },
  "auth.html": { file: "assets/auth-b2.js", isEntry: true },
  "src/chunk.ts": { file: "assets/chunk-c3.js" },
};

describe("entryOfScripts", () => {
  const entryOf = entryOfScripts(manifest);

  test("names the entry whose file a module script is", () => {
    expect(entryOf(["http://localhost:8787/assets/auth-b2.js"])).toBe("auth.html");
    expect(entryOf(["/assets/chunk-c3.js", "/assets/index-a1.js"])).toBe("index.html");
  });

  test("names none for a document of no entry", () => {
    expect(entryOf(["/assets/chunk-c3.js"])).toBeUndefined();
    expect(entryOf([])).toBeUndefined();
  });
});

describe("runsOf", () => {
  const entryOf = entryOfScripts(manifest);

  test("gives a run per entry, with its worst LCP and interaction and its long frames", () => {
    const samples: Sample[] = [
      { doc: "a", kind: "document", scripts: ["/assets/auth-b2.js"] },
      { doc: "a", kind: "lcp", value: 800 },
      { doc: "a", kind: "interaction", value: 64 },
      { doc: "a", kind: "interaction", value: 120 },
      { doc: "b", kind: "document", scripts: ["/assets/index-a1.js"] },
      { doc: "b", kind: "lcp", value: 1500 },
      { doc: "b", kind: "long_frame", value: 80 },
      { doc: "b", kind: "long_frame", value: 150 },
      { doc: "c", kind: "document", scripts: ["/assets/index-a1.js"] },
      { doc: "c", kind: "lcp", value: 1200 },
    ];
    expect(runsOf("sign-in", samples, entryOf)).toEqual([
      { scenario: "sign-in", entry: "auth.html", lcp: 800, inp: 120, longFrames: { count: 0, total: 0, longest: 0 } },
      {
        scenario: "sign-in",
        entry: "index.html",
        lcp: 1500,
        inp: null,
        longFrames: { count: 2, total: 230, longest: 150 },
      },
    ]);
  });

  test("leaves out what a document of no entry heard, and what came from an unknown document", () => {
    const samples: Sample[] = [
      { doc: "x", kind: "document", scripts: [] },
      { doc: "x", kind: "lcp", value: 900 },
      { doc: "y", kind: "lcp", value: 900 },
    ];
    expect(runsOf("s", samples, entryOf)).toEqual([]);
  });
});

describe("summarize", () => {
  const run = (lcp: number | null, inp: number | null, total: number): Run => ({
    scenario: "home",
    entry: "index.html",
    lcp,
    inp,
    longFrames: { count: total / 100, total, longest: total },
  });

  test("takes the median of each measure over the runs", () => {
    expect(summarize([run(1000, 100, 100), run(3000, 300, 300), run(2000, null, 200)])).toEqual([
      {
        scenario: "home",
        entry: "index.html",
        runs: 3,
        lcp: 2000,
        inp: 200,
        longFrames: { count: 2, total: 200, longest: 200 },
      },
    ]);
  });

  test("sorts by scenario, then entry", () => {
    const a = { ...run(1, 1, 0), scenario: "b" };
    const b = { ...run(1, 1, 0), scenario: "a", entry: "index.html" };
    const c = { ...run(1, 1, 0), scenario: "a", entry: "auth.html" };
    expect(summarize([a, b, c]).map((m) => `${m.scenario} ${m.entry}`)).toEqual([
      "a auth.html",
      "a index.html",
      "b index.html",
    ]);
  });
});

test("median", () => {
  expect(median([])).toBeNull();
  expect(median([3, 1, 2])).toBe(2);
  expect(median([4, 1, 2, 3])).toBe(2.5);
});
