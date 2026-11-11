// SPDX-License-Identifier: AGPL-3.0-only
// The budget of a screen (web-client §21 P3, ADR-0110 §6): what the story test (vitest.stories.config.ts) holds every
// screen story to, from the screen's L1 file (docs/architecture/web-client/screens/<screen>.md).
//
// - A story belongs to the screen whose route, the paths in backticks of the L1's first heading, matches the path the
//   story opens (`parameters.screen.path`); the most literal route wins. An L1 with `budget: null` is a part or a
//   state of another screen's route and has no stories of its own.
// - A story in English at the default viewport shows no more than the `budget` of the L1's front matter, and no
//   region of it holds more than `regionLimit` controls.
// - A story in Japanese at the default viewport shows its `h2` in the order of the L1's "3." list. An item that
//   begins with `<` (as "<the announcement's title>") stands for the headings that come from data.
// - screenBaseline.json lists the stories that broke the last two rules when the checks arrived. It only shrinks: a
//   listed story that passes fails until its entry is removed.
import { readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { ScreenMeasure } from "../src/test/screenMeasure.ts";
import { parseSetup, type ScreenBudget } from "./screens.ts";
import { screensDir } from "./tasks.ts";

/** The most controls of one region under an `h2` (ADR-0110 §6). */
const regionLimit = 7;

/** The baseline's path, relative to the repository's root. */
export const baselinePath = "app/web/l4/screenBaseline.json";

/** One story as the story test saw it. */
export type StoryRecord = {
  /** Storybook's id, as `screens-admin-rules--normal`. */
  id: string;
  /** The path the story opens the app at. */
  path: string;
  locale: string;
  /** Whether the story sets a viewport of its own (the 320 px one). */
  viewport: boolean;
  measure: ScreenMeasure;
};

/** What the checks read of one L1 file. */
export type Screen = {
  /** The file's name without `.md`. */
  name: string;
  /** The routes of the first heading, as `/admin/rules/{id}`. */
  routes: string[];
  /** null for a part or a state of another screen's route, which owns no stories. */
  budget: ScreenBudget | null | undefined;
  /** The items of the "3." list, in order. */
  regions: string[];
  /** Whether it has the section "7. 載せないもの". */
  omissions: boolean;
};

/** The stories that may break a rule, by Storybook's id. */
export type Baseline = {
  /** The controls of each region over `regionLimit`: `{"<story id>": {"<h2>": <controls>}}`. */
  regionControls: Record<string, Record<string, number>>;
  /** The stories whose `h2` are not in the order of the L1. */
  regionOrder: string[];
  /** The screens whose L1 has no section "7. 載せないもの", by the file's name. */
  noOmissions: string[];
};

const regionSection = /^###\s+3\.\s/;

/** Parses what the checks read of the L1 file `markdown` of `name`. */
export function parseScreen(name: string, markdown: string): Screen {
  const lines = markdown.split("\n");
  const heading = lines.find((line) => line.startsWith("## ")) ?? "";
  const routes = Array.from(heading.matchAll(/`(\/[^`]*)`/g), (match) => match[1] ?? "");
  const start = lines.findIndex((line) => regionSection.test(line));
  const regions: string[] = [];
  for (const line of start < 0 ? [] : lines.slice(start + 1)) {
    if (line.startsWith("#")) break;
    const item = /^\d+\.\s+(.*)$/.exec(line);
    if (item !== null) regions.push(item[1] ?? "");
  }
  const omissions = lines.some((line) => /^###\s+7\.\s/.test(line));
  return { name, routes, budget: parseSetup(name, markdown).budget, regions, omissions };
}

/** Every L1 file of the repository at `root`. */
export function readScreens(root: string): Screen[] {
  return readdirSync(join(root, screensDir))
    .filter((file) => file.endsWith(".md") && file !== "README.md")
    .toSorted()
    .map((file) => parseScreen(file.slice(0, -3), readFileSync(join(root, screensDir, file), "utf8")));
}

function segments(path: string): string[] {
  return (path.split(/[?#]/)[0] ?? "").split("/").filter((segment) => segment !== "");
}

/** A segment of a route that stands for any value: `{id}`, `$id`, `@{acct}`. */
function isParameter(segment: string): boolean {
  return /^@?(\{.*\}|\$.+)$/.test(segment);
}

/** How literally `route` matches `path`: its literal segments, or -1 when it does not match. */
function matchRoute(route: string, path: string[]): number {
  const parts = segments(route);
  if (path.length !== parts.length) return -1;
  let literal = 0;
  for (const [index, part] of parts.entries()) {
    const segment = path[index] ?? "";
    if (isParameter(part)) {
      if (part.startsWith("@") !== segment.startsWith("@")) return -1;
    } else if (part === segment) {
      literal++;
    } else {
      return -1;
    }
  }
  return literal;
}

/**
 * The screens whose route matches `path` most literally: one when the L1 files tell the story's screen, none when no
 * L1 has the route, several when L1 files share it.
 */
export function screensOf(path: string, screens: readonly Screen[]): Screen[] {
  const parts = segments(path);
  let best = -1;
  let found: Screen[] = [];
  for (const screen of screens) {
    if (screen.budget === null) continue;
    const score = Math.max(-1, ...screen.routes.map((route) => matchRoute(route, parts)));
    if (score < 0 || score < best) continue;
    if (score > best) found = [];
    best = score;
    found.push(screen);
  }
  return found;
}

/** The text an `h2` and an item of the L1's list are compared by: without markup, brackets and spaces. */
function plain(text: string): string {
  return text.replace(/[\s`*「」『』"']/gu, "");
}

/**
 * Whether the `h2` of a story are in the order of the L1's list: each names an item (the item begins with it) later
 * than the one before. Returns the first `h2` that is not.
 */
export function outOfOrder(titles: readonly string[], regions: readonly string[]): string | undefined {
  const items = regions.map(plain);
  let from = 0;
  for (const title of titles) {
    const index = items.findIndex((item, at) => at >= from && (item.startsWith("<") || item.startsWith(plain(title))));
    if (index < 0) return title;
    // An item for headings from data names every such heading in a row.
    from = items[index]?.startsWith("<") ? index : index + 1;
  }
  return undefined;
}

const l1 = (screen: Screen) => `${screensDir}/${screen.name}.md`;

/** The regions of `measure` over `regionLimit`, with their controls. */
function crowded(measure: ScreenMeasure): Record<string, number> {
  const found: Record<string, number> = {};
  for (const region of measure.regions) {
    if (region.controls > regionLimit) found[region.title] = Math.max(found[region.title] ?? 0, region.controls);
  }
  return found;
}

function same(a: Record<string, number>, b: Record<string, number>): boolean {
  return JSON.stringify(Object.entries(a).toSorted()) === JSON.stringify(Object.entries(b).toSorted());
}

/** The failures of `story` against its screen's L1 and the baseline; each says what to change. */
export function checkStory(story: StoryRecord, screens: readonly Screen[], baseline: Baseline): string[] {
  if (story.viewport) return [];
  const found = screensOf(story.path, screens);
  if (found.length > 1) {
    return [
      `${story.path} is the route of ${found.map(l1).join(" and ")}: give each L1's first heading a route of its own.`,
    ];
  }
  const [screen] = found;
  const failures: string[] = [];
  const { measure } = story;
  if (story.locale === "en") {
    for (const field of ["controls", "helpers", "words"] as const) {
      const budget = screen?.budget?.[field];
      if (screen === undefined || budget === undefined || measure[field] <= budget) continue;
      failures.push(
        `${field}: ${measure[field]} on this screen, over its budget of ${budget}. Remove or merge something on ` +
          `this screen, or raise the budget in ${l1(screen)} (the owner reviews that diff).`,
      );
    }
    const now = crowded(measure);
    const listed = baseline.regionControls[story.id];
    if (listed === undefined) {
      for (const [title, controls] of Object.entries(now)) {
        failures.push(
          `the region "${title}" has ${controls} controls, over the ${regionLimit} of a region: split the region ` +
            `or move controls to a page of their own (ADR-0110 §6).`,
        );
      }
    } else if (!same(now, listed)) {
      const entry = Object.keys(now).length === 0 ? "remove the entry" : `set it to ${JSON.stringify(now)}`;
      const worse = Object.entries(now).some(([title, controls]) => controls > (listed[title] ?? regionLimit));
      failures.push(
        worse
          ? `a region grew past what ${baselinePath} lists for "${story.id}" (${JSON.stringify(listed)} → ` +
              `${JSON.stringify(now)}): split the region instead; the baseline only shrinks (ADR-0110 §6).`
          : `the regions of "${story.id}" are smaller than ${baselinePath} lists: ${entry} under regionControls.`,
      );
    }
  }
  if (story.locale === "ja" && screen !== undefined) {
    const title = outOfOrder(
      measure.regions.map((region) => region.title),
      screen.regions,
    );
    const listed = baseline.regionOrder.includes(story.id);
    if (title !== undefined && !listed) {
      failures.push(
        `the h2 "${title}" is not where the list "3. 情報の優先順位とまとまり" of ${l1(screen)} puts it: the list ` +
          `names every h2 at the start of an item ("N. ${title}: ..."; "N. <...>: ..." for headings from data), ` +
          `in the order of the screen (ADR-0110 §6).`,
      );
    } else if (title === undefined && listed) {
      failures.push(`the h2 of "${story.id}" follow ${l1(screen)} now: remove it from regionOrder in ${baselinePath}.`);
    }
  }
  return failures;
}

/**
 * The failure of the L1 file of `screen` against the baseline: a screen says what it leaves out ("7. 載せないもの",
 * screens/README.md) unless the baseline lists it, and a listed screen that says it leaves the list.
 */
export function checkOmissions(screen: Screen, baseline: Baseline): string | undefined {
  const listed = baseline.noOmissions.includes(screen.name);
  if (!screen.omissions && !listed)
    return `${l1(screen)} has no section "### 7. 載せないもの": say what the screen leaves out on purpose, and where that is instead.`;
  if (screen.omissions && listed)
    return `${l1(screen)} has "7. 載せないもの" now: remove "${screen.name}" from noOmissions in ${baselinePath}.`;
  return undefined;
}

export function readBaseline(root: string): Baseline {
  return JSON.parse(readFileSync(join(root, baselinePath), "utf8")) as Baseline;
}

/** The baseline that lets every story of `stories` pass as it is. */
export function baselineOf(stories: readonly StoryRecord[], screens: readonly Screen[]): Baseline {
  const noOmissions = screens.filter((screen) => !screen.omissions).map((screen) => screen.name);
  const baseline: Baseline = { regionControls: {}, regionOrder: [], noOmissions };
  for (const story of stories.toSorted((a, b) => a.id.localeCompare(b.id))) {
    if (story.viewport) continue;
    const now = crowded(story.measure);
    if (story.locale === "en" && Object.keys(now).length > 0) baseline.regionControls[story.id] = now;
    const [screen, other] = screensOf(story.path, screens);
    if (story.locale !== "ja" || screen === undefined || other !== undefined) continue;
    const titles = story.measure.regions.map((region) => region.title);
    if (outOfOrder(titles, screen.regions) !== undefined) baseline.regionOrder.push(story.id);
  }
  return baseline;
}

/** The stories a budget is measured on: in English, at the default viewport. */
function measured(stories: readonly StoryRecord[]): StoryRecord[] {
  return stories.filter((story) => story.locale === "en" && !story.viewport);
}

/** The largest counts of the stories of each screen, by the screen's name. */
export function budgetsOf(stories: readonly StoryRecord[], screens: readonly Screen[]): Map<string, ScreenBudget> {
  const budgets = new Map<string, ScreenBudget>();
  for (const story of measured(stories)) {
    const [screen, other] = screensOf(story.path, screens);
    if (screen === undefined || other !== undefined) continue;
    const { controls, helpers, words } = budgets.get(screen.name) ?? { controls: 0, helpers: 0, words: 0 };
    budgets.set(screen.name, {
      controls: Math.max(controls, story.measure.controls),
      helpers: Math.max(helpers, story.measure.helpers),
      words: Math.max(words, story.measure.words),
    });
  }
  return budgets;
}

/** Writes `budget` into the front matter of the L1 file of `screen`, replacing the one it has. */
export function writeBudget(root: string, screen: string, budget: ScreenBudget): void {
  const file = join(root, screensDir, `${screen}.md`);
  const lines = readFileSync(file, "utf8").split("\n");
  const end = lines.indexOf("---", 1);
  const line = `budget: {"controls": ${budget.controls}, "helpers": ${budget.helpers}, "words": ${budget.words}}`;
  const at = lines.findIndex((text, index) => index < end && text.startsWith("budget:"));
  if (at < 0) lines.splice(end, 0, line);
  else lines[at] = line;
  writeFileSync(file, lines.join("\n"));
}

export function writeBaseline(root: string, baseline: Baseline): void {
  writeFileSync(join(root, baselinePath), `${JSON.stringify(baseline, null, 2)}\n`);
}

/**
 * What a whole run tells that one story cannot: the screens no story measured, the stories no L1 has the route of,
 * and the entries of the baseline whose story is gone (`stale`, a failure).
 */
export function summary(
  stories: readonly StoryRecord[],
  screens: readonly Screen[],
  baseline: Baseline,
): { unmeasured: string[]; unmapped: string[]; stale: string[] } {
  const seen = new Set(budgetsOf(stories, screens).keys());
  const ids = new Set(stories.map((story) => story.id));
  return {
    unmeasured: screens
      .filter((screen) => screen.budget !== null && !seen.has(screen.name))
      .map((screen) => screen.name),
    unmapped: [
      ...new Set(
        measured(stories)
          .filter((story) => screensOf(story.path, screens).length === 0)
          .map((story) => story.path.split(/[?#]/)[0] ?? ""),
      ),
    ].toSorted(),
    stale: [...Object.keys(baseline.regionControls), ...baseline.regionOrder].filter((id) => !ids.has(id)),
  };
}
