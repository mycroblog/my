// SPDX-License-Identifier: AGPL-3.0-only

// The measures of the lab (ADR-0167 §1, §4): what the probe of a page (probe.ts) heard in one run of a scenario, by
// the HTML entry of the document it heard it in, and the median of the runs of each scenario and entry. The probe
// hears the platform's vitals (@my/platform/measure, the definitions the field measurement uses): the largest
// contentful paint of each load, the time from each interaction to its next paint, and each long animation frame.

/** What the probe of one document sends: the document's module scripts once, then each value it hears, in ms. */
export type Sample =
  | { readonly doc: string; readonly kind: "document"; readonly scripts: readonly string[] }
  | { readonly doc: string; readonly kind: "lcp" | "interaction" | "long_frame"; readonly value: number };

/** The long animation frames of a run: how many, their durations added up, and the longest, in ms. */
type LongFrames = { readonly count: number; readonly total: number; readonly longest: number };

/** What one run of a scenario measured in the documents of one entry. */
export type Run = {
  readonly scenario: string;
  /** The entry of Vite's manifest the documents loaded (`index.html`, `auth.html`, `pay.html`). */
  readonly entry: string;
  /** The worst largest contentful paint of the entry's loads; null when none was heard. */
  readonly lcp: number | null;
  /** The worst interaction, the run's INP; null when no interaction took 40 ms or more (the probe hears no less). */
  readonly inp: number | null;
  readonly longFrames: LongFrames;
};

/** The median of the runs of one scenario and entry. */
export type Measure = Omit<Run, "longFrames"> & { readonly runs: number; readonly longFrames: LongFrames };

/** The entry whose script, a file of Vite's manifest (`assets/index-abc.js`), one of the document's `scripts` is. */
export type EntryOf = (scripts: readonly string[]) => string | undefined;

/** The entry of `manifest` that a document whose module scripts are `scripts` (URLs) loaded. */
export function entryOfScripts(manifest: Readonly<Record<string, { file: string; isEntry?: boolean }>>): EntryOf {
  const entries = Object.entries(manifest).filter(([key, chunk]) => chunk.isEntry === true && key.endsWith(".html"));
  return (scripts) => {
    for (const script of scripts) {
      const path = new URL(script, "http://document.invalid/").pathname;
      const found = entries.find(([, chunk]) => path.endsWith(`/${chunk.file}`));
      if (found !== undefined) return found[0];
    }
    return undefined;
  };
}

/** The runs of `scenario` in `samples`, one per entry that a document loaded; a document of no entry is left out. */
export function runsOf(scenario: string, samples: readonly Sample[], entryOf: EntryOf): Run[] {
  const docs = new Map<string, string>();
  for (const s of samples) {
    if (s.kind !== "document") continue;
    const entry = entryOf(s.scripts);
    if (entry !== undefined) docs.set(s.doc, entry);
  }
  type Held = { lcp: number | null; inp: number | null; frames: number[] };
  const byEntry = new Map<string, Held>();
  for (const entry of new Set(docs.values())) byEntry.set(entry, { lcp: null, inp: null, frames: [] });
  for (const s of samples) {
    if (s.kind === "document") continue;
    const entry = docs.get(s.doc);
    const held = entry === undefined ? undefined : byEntry.get(entry);
    if (held === undefined) continue;
    if (s.kind === "lcp") held.lcp = Math.max(held.lcp ?? 0, s.value);
    else if (s.kind === "interaction") held.inp = Math.max(held.inp ?? 0, s.value);
    else held.frames.push(s.value);
  }
  return [...byEntry].map(([entry, held]) => ({
    scenario,
    entry,
    lcp: held.lcp,
    inp: held.inp,
    longFrames: {
      count: held.frames.length,
      total: held.frames.reduce((a, b) => a + b, 0),
      longest: held.frames.reduce((a, b) => Math.max(a, b), 0),
    },
  }));
}

/** The median of `values`, the mean of the middle two of an even count; null for none. */
export function median(values: readonly number[]): number | null {
  if (values.length === 0) return null;
  const sorted = [...values].sort((a, b) => a - b);
  const mid = Math.floor(sorted.length / 2);
  return sorted.length % 2 === 1 ? (sorted[mid] ?? null) : ((sorted[mid - 1] ?? 0) + (sorted[mid] ?? 0)) / 2;
}

/**
 * The median of each measure over the runs of each scenario and entry, sorted by scenario then entry. A run that heard
 * no LCP or no interaction leaves that measure's median to the runs that did.
 */
export function summarize(runs: readonly Run[]): Measure[] {
  const groups = new Map<string, Run[]>();
  for (const run of runs) {
    const key = `${run.scenario}\u0000${run.entry}`;
    groups.set(key, [...(groups.get(key) ?? []), run]);
  }
  const round = (v: number | null) => (v === null ? null : Math.round(v));
  const nums = (vs: readonly (number | null)[]) => vs.filter((v): v is number => v !== null);
  return [...groups.values()]
    .map((group) => {
      const first = group[0] as Run;
      return {
        scenario: first.scenario,
        entry: first.entry,
        runs: group.length,
        lcp: round(median(nums(group.map((r) => r.lcp)))),
        inp: round(median(nums(group.map((r) => r.inp)))),
        longFrames: {
          count: round(median(group.map((r) => r.longFrames.count))) ?? 0,
          total: round(median(group.map((r) => r.longFrames.total))) ?? 0,
          longest: round(median(group.map((r) => r.longFrames.longest))) ?? 0,
        },
      };
    })
    .sort((a, b) => a.scenario.localeCompare(b.scenario) || a.entry.localeCompare(b.entry));
}
