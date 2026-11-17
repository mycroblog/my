// SPDX-License-Identifier: AGPL-3.0-only

// The words of a scheduled post's time: the moment in the reader's locale and zone, the zone itself, and the value a
// `datetime-local` field holds. The zone is the process's, so the tests set it and put it back.

import { afterAll, beforeAll, expect, test } from "bun:test";
import { localInput, readerZone, scheduleTime } from "./drafts";

const zone = process.env.TZ;

beforeAll(() => {
  process.env.TZ = "Asia/Tokyo";
});

afterAll(() => {
  if (zone === undefined) delete process.env.TZ;
  else process.env.TZ = zone;
});

test("a scheduled time reads as the weekday, the date and the time, in the reader's locale and zone", () => {
  // 09:00 UTC is 18:00 in Tokyo.
  expect(scheduleTime("2026-11-16T09:00:00Z", "en")).toBe("Mon, Nov 16, 6:00 PM");
  expect(scheduleTime(new Date("2026-11-16T09:00:00Z"), "ja")).toBe("11月16日(月) 18:00");
  // Late in the day it is already the next day there.
  expect(scheduleTime("2026-11-16T15:30:00Z", "en")).toBe("Tue, Nov 17, 12:30 AM");
});

test("the reader's zone reads as its name and its offset", () => {
  expect(readerZone("en", new Date("2026-11-16T09:00:00Z"))).toBe("Asia/Tokyo · GMT+9");
  expect(readerZone("ja", new Date("2026-11-16T09:00:00Z"))).toBe("Asia/Tokyo · GMT+9");
});

test("a local date and time is written as a datetime-local field holds it, to the minute", () => {
  expect(localInput(new Date(2026, 10, 16, 9, 0, 42))).toBe("2026-11-16T09:00");
  expect(localInput(new Date(2026, 0, 2, 23, 5))).toBe("2026-01-02T23:05");
  expect(localInput(new Date("2026-11-16T15:30:00Z"))).toBe("2026-11-17T00:30");
});
