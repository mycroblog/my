// SPDX-License-Identifier: AGPL-3.0-only

// A page at rest, for a check that knows nothing of the story it reads (specs/stories): no element to wait for, so
// it waits for the page itself. Time is the test's: the page's clock stands still from its first script
// (`page.clock`), so nothing happens on the page because time passed, and the same story reads the same on a busy
// machine and on an idle one. The page is at rest when nothing it started is still under way: no request in flight,
// no font loading, no animation running (a layer still moving in reads as low contrast to axe), and nothing waiting
// for the main thread. What a page does after a delay of its own (React shows what was suspended 300 ms after its
// fallback, a field asks once the typing stopped) comes when the test moves the clock, a step at a time, the page at
// rest again after each.

import { expect, type Page, type Request } from "@playwright/test";

/**
 * Where the page's clock stands: noon in the middle of a month, so that a story's "3 hours ago" stays in the same
 * day, month and year in every time zone.
 */
const moment = new Date("2026-06-15T12:00:00Z");

/** How far `rest` moves the page's clock in all, and by how much at a time, in ms. */
const second = 1000;
const step = 100;

/** A page whose clock the test holds. */
export type Held = {
  /**
   * Returns once the page is at rest a second of its own time after it was drawn: what it shows then is what a
   * reader looks at.
   */
  rest: () => Promise<void>;
  /**
   * Answers `work`, something the page does for the test that waits on timers (axe), moving the clock a millisecond
   * at a time until it answered.
   */
  during: <T>(work: Promise<T>) => Promise<T>;
};

/** What of the page is still under way, in words; empty at rest. Answers once nothing waits for the main thread. */
const underWay = (page: Page): Promise<string[]> =>
  page.evaluate(async () => {
    // A background task runs once no other task waits: React's rendering, which yields between its slices, is done.
    await new Promise<void>((done) => {
      void scheduler.postTask(done, { priority: "background" });
    });
    const running = document
      .getAnimations()
      .filter(
        (animation) => animation.playState === "running" && animation.effect?.getComputedTiming().endTime !== Infinity,
      );
    await Promise.allSettled(running.map((animation) => animation.finished));
    return [
      ...(running.length > 0 ? [`${String(running.length)} animations ran`] : []),
      ...(document.fonts.status === "loaded" ? [] : ["a font is loading"]),
    ];
  });

/**
 * Takes the page's time before it loads anything: call it before the story is opened. The clock stands at one
 * moment until `rest` or `during` moves it.
 */
export async function hold(page: Page): Promise<Held> {
  const inFlight = new Set<Request>();
  page.on("request", (request) => inFlight.add(request));
  page.on("requestfinished", (request) => inFlight.delete(request));
  page.on("requestfailed", (request) => inFlight.delete(request));
  await page.clock.install({ time: moment });
  await page.clock.pauseAt(moment);

  const still = async () => {
    await expect
      .poll(async () => [...(await underWay(page)), ...[...inFlight].map((request) => `waiting for ${request.url()}`)])
      .toEqual([]);
  };

  return {
    rest: async () => {
      await still();
      for (let moved = 0; moved < second; moved += step) {
        await page.clock.runFor(step);
        await still();
      }
    },
    during: async (work) => {
      const answer: { settled?: PromiseSettledResult<Awaited<typeof work>> } = {};
      void Promise.allSettled([work]).then(([settled]) => {
        answer.settled = settled;
      });
      while (answer.settled === undefined) await page.clock.runFor(1);
      if (answer.settled.status === "rejected") throw answer.settled.reason;
      return answer.settled.value;
    },
  };
}
