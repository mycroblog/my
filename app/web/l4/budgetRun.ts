// SPDX-License-Identifier: AGPL-3.0-only
// The Node.js side of the screen budgets in the story tests (vitest.stories.config.ts): the `screenBudget` command
// that src/test/storyBudget.ts calls for every story, and, as the run's global setup, what a whole run tells at its
// end (budget.ts `summary`). The stories of a run are kept in .tmp/web-budget/stories.jsonl between the two.
//
// BUDGET=init (scripts/task.sh web:budget-init) fails no story: at the end it writes a budget into every L1 file
// that has none, the largest counts of the screen's stories, and the baseline when the file is missing.
import { appendFileSync, existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import {
  type Baseline,
  baselineOf,
  baselinePath,
  budgetsOf,
  checkStory,
  readBaseline,
  readScreens,
  type Screen,
  type StoryRecord,
  summary,
  writeBaseline,
  writeBudget,
} from "./budget.ts";
import { screensDir } from "./tasks.ts";

const root = resolve(import.meta.dirname, "../../..");
const records = join(root, ".tmp/web-budget/stories.jsonl");
const init = process.env["BUDGET"] === "init";
/** The arguments of `bun run test:stories`: a run with others (a shard, a file) sees only some stories. */
const whole = process.argv.slice(2).every((arg) => ["run", "--config", "vitest.stories.config.ts"].includes(arg));

const noBaseline: Baseline = { regionControls: {}, regionOrder: [], noOmissions: [] };

function baseline(): Baseline {
  return existsSync(join(root, baselinePath)) ? readBaseline(root) : noBaseline;
}

let loaded: { screens: Screen[]; baseline: Baseline } | undefined;

/** The command of a story: records it and answers its failures. */
export function screenBudget(_context: unknown, story: StoryRecord): string[] {
  loaded ??= { screens: readScreens(root), baseline: baseline() };
  appendFileSync(records, `${JSON.stringify(story)}\n`);
  return init ? [] : checkStory(story, loaded.screens, loaded.baseline);
}

function readRecords(): StoryRecord[] {
  return readFileSync(records, "utf8")
    .split("\n")
    .filter((line) => line !== "")
    .map((line) => JSON.parse(line) as StoryRecord);
}

/** Vitest's global setup: empties the record of the last run, and returns what runs after the last story. */
export default function setup(): () => void {
  mkdirSync(dirname(records), { recursive: true });
  writeFileSync(records, "");
  return () => {
    const stories = readRecords();
    const screens = readScreens(root);
    if (init) {
      for (const [screen, budget] of budgetsOf(stories, screens)) {
        if (screens.find((s) => s.name === screen)?.budget === undefined) writeBudget(root, screen, budget);
      }
      if (!existsSync(join(root, baselinePath))) writeBaseline(root, baselineOf(stories, screens));
      return;
    }
    if (!whole) return;
    const { unmeasured, unmapped, stale } = summary(stories, screens, baseline());
    if (unmeasured.length > 0) {
      console.log(
        `screen budgets: ${unmeasured.length} of ${screens.length} screens of ${screensDir} have no story in ` +
          `English at the default viewport, so nothing measures them: ${unmeasured.join(", ")}`,
      );
    }
    if (unmapped.length > 0) {
      console.log(`screen budgets: no L1 file has the route of the stories at: ${unmapped.join(", ")}`);
    }
    if (stale.length > 0) {
      console.error(`screen budgets: remove from ${baselinePath} the stories that are gone: ${stale.join(", ")}`);
      process.exitCode = 1;
    }
  };
}
