// SPDX-License-Identifier: AGPL-3.0-only
// A setup file of the story tests (vitest.stories.config.ts): after a screen story has rendered and played, measures
// what it shows (screenMeasure.ts) and fails it on what the `screenBudget` command answers, the checks of
// l4/budget.ts against the screen's L1 file. The command runs in Node.js, where the L1 files are.
import { afterEach } from "vitest";
import { commands } from "vitest/browser";
import { measureScreen, type ScreenMeasure } from "./screenMeasure";

declare module "vitest/browser" {
  interface BrowserCommands {
    screenBudget: (story: {
      id: string;
      path: string;
      locale: string;
      viewport: boolean;
      measure: ScreenMeasure;
    }) => Promise<string[]>;
  }
}

// The requests of the page that have not answered yet: a region that loads after the page's heading (a profile's
// private note) is measured once it is there.
let pending = 0;
const send = window.fetch;
window.fetch = async (...args) => {
  pending++;
  try {
    return await send(...args);
  } finally {
    pending--;
  }
};

/**
 * The measure of the screen once it rests: two measures 100 ms apart agree and no request is pending. A request that
 * never answers (a story that shows its loading, a stream) is waited for half a second, and a screen that keeps
 * changing is measured as it is after 3 s.
 */
async function measureAtRest(): Promise<ScreenMeasure> {
  let last = measureScreen();
  let same = 0;
  for (const end = Date.now() + 3_000; Date.now() < end;) {
    await new Promise((done) => setTimeout(done, 100));
    const now = measureScreen();
    same = JSON.stringify(now) === JSON.stringify(last) ? same + 1 : 0;
    if (same >= (pending === 0 ? 1 : 5)) return now;
    last = now;
  }
  return last;
}

type ComposedStory = { id: string; globals: Record<string, unknown>; parameters: Record<string, unknown> };

afterEach(async (context) => {
  // Only story tests carry a composed story; it stays mounted until the next one renders.
  const { story } = context as typeof context & { story?: ComposedStory };
  if (story === undefined || context.task.result?.state === "fail") return;
  const screen = story.parameters["screen"] as { path?: unknown } | undefined;
  if (typeof screen?.path !== "string") return;
  const viewport = story.globals["viewport"] as { value?: unknown } | undefined;
  const locale = typeof story.globals["locale"] === "string" ? story.globals["locale"] : "en";
  const ownViewport = typeof viewport?.value === "string" && viewport.value !== "";
  // The checks read the stories in English and Japanese at the default viewport (l4/budget.ts): no wait for others.
  const checked = !ownViewport && (locale === "en" || locale === "ja");
  const failures = await commands.screenBudget({
    id: story.id,
    path: screen.path,
    locale,
    viewport: ownViewport,
    measure: checked ? await measureAtRest() : measureScreen(),
  });
  if (failures.length > 0) throw new Error(`${story.id}: ${failures.join("\n")}`);
});
