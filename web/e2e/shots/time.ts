// SPDX-License-Identifier: AGPL-3.0-only

// The time a screenshot shows, the same on every run: the server stamps what the scenario made with the moment it ran,
// so the page's clock is fixed at `shownNow`, and every time in the API's answers is moved as far as the run's start
// (`anchor`) is from `shownBase`, then down to the ten minutes it falls in. Whatever the run made within five minutes
// of its start shows made at `shownBase`'s ten minutes ("2h" ago). A time more than a day from the run's start (an
// event the scenario plans at an hour of its own, a poll's end) moves by whole days alone, keeping its hour.

import type { Page } from "@playwright/test";

/** The moment the run's start shows as, on a minute five past the ten minutes, so that ±5 minutes fall in one. */
const shownBase = Date.parse("2026-06-15T01:05:00Z");

/** The page's clock: two hours after the scenario was made. */
const shownNow = Date.parse("2026-06-15T03:00:00Z");

/** The run's start: a worker loads this module once the demo server is seeded. */
const anchor = Date.now();

const tenMinutes = 600_000;
const day = 86_400_000;

const dateTime = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(?::\d{2}(?:\.\d+)?)?(?:Z|[+-]\d{2}:\d{2})$/;
const dateOnly = /^\d{4}-\d{2}-\d{2}$/;

/** The calendar days (UTC) from the run's start to `shownBase`, in milliseconds. */
const wholeDays = (Math.floor(shownBase / day) - Math.floor(anchor / day)) * day;

/** Moves every date and time in `value`, an answer of the API, from the run's start to `shownBase`. */
function shiftTimes(value: unknown): unknown {
  if (typeof value === "string") {
    if (dateTime.test(value)) {
      const t = Date.parse(value);
      if (Math.abs(t - anchor) > day) return new Date(t + wholeDays).toISOString();
      const moved = shownBase + (t - anchor);
      return new Date(Math.floor(moved / tenMinutes) * tenMinutes).toISOString();
    }
    if (dateOnly.test(value)) return new Date(Date.parse(`${value}T00:00:00Z`) + wholeDays).toISOString().slice(0, 10);
    return value;
  }
  if (Array.isArray(value)) return value.map((v) => shiftTimes(v));
  if (value !== null && typeof value === "object")
    return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, shiftTimes(v)]));
  return value;
}

/**
 * Fixes the clock of `page` at `shownNow` and moves the times of the API's JSON answers (`shiftTimes`) for the pages
 * it loads next. The streams of the API pass as they are: a stream's answer never ends, to be read whole.
 */
export async function fixTime(page: Page, origin: string): Promise<void> {
  await page.clock.setFixedTime(shownNow);
  await page.route(
    (url) => url.origin === origin && url.pathname.startsWith("/api/") && !/\/stream(?:ing)?\b/.test(url.pathname),
    async (route) => {
      const response = await route.fetch();
      const type = response.headers()["content-type"] ?? "";
      if (!type.startsWith("application/json")) {
        await route.fulfill({ response });
        return;
      }
      const json: unknown = await response.json();
      await route.fulfill({ response, json: shiftTimes(json) });
    },
  );
}
