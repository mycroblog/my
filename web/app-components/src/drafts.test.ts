// SPDX-License-Identifier: AGPL-3.0-only

// The words of a scheduled post's time: the moment in the reader's locale and zone, the zone itself, and the value a
// `datetime-local` field holds. The zone is the process's, so the tests set it and put it back.

import { afterAll, beforeAll, expect, test } from "bun:test";
import { localInput, localInstant, readerZone, scheduleTime } from "./drafts";

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

// The reader's own zone, where it is not the device's (Tokyo here).

test("in the reader's zone, a scheduled time reads on that zone's clock and calendar", () => {
  // 03:30 UTC on Monday is 22:30 on Sunday in New York.
  expect(scheduleTime("2026-11-16T03:30:00Z", "en", "America/New_York")).toBe("Sun, Nov 15, 10:30 PM");
  expect(scheduleTime("2026-11-16T03:30:00Z", "ja", "America/New_York")).toBe("11月15日(日) 22:30");
  expect(scheduleTime("2026-11-16T03:30:00Z", "en", "Asia/Kolkata")).toBe("Mon, Nov 16, 9:00 AM");
});

test("the reader's zone reads as its own name and its offset at that moment", () => {
  expect(readerZone("en", new Date("2026-11-16T09:00:00Z"), "America/New_York")).toBe("America/New_York · GMT-5");
  // In summer its clock is an hour ahead.
  expect(readerZone("en", new Date("2026-07-16T09:00:00Z"), "America/New_York")).toBe("America/New_York · GMT-4");
  expect(readerZone("ja", new Date("2026-11-16T09:00:00Z"), "Asia/Kolkata")).toBe("Asia/Kolkata · GMT+5:30");
});

// Each moment, a zone, and what a datetime-local field holds for it there.
const fields: readonly (readonly [string, string | undefined, string])[] = [
  ["2026-11-16T03:30:00Z", "America/New_York", "2026-11-15T22:30"],
  ["2026-11-16T03:30:00Z", "UTC", "2026-11-16T03:30"],
  // Half an hour off the hour, and a summer time of half an hour.
  ["2026-11-16T09:00:00Z", "Asia/Kolkata", "2026-11-16T14:30"],
  ["2026-01-15T00:00:00Z", "Australia/Lord_Howe", "2026-01-15T11:00"],
  ["2026-07-15T00:00:00Z", "Australia/Lord_Howe", "2026-07-15T10:30"],
  // Either side of the night New York's clock goes forward, and after it went back.
  ["2026-03-08T06:59:00Z", "America/New_York", "2026-03-08T01:59"],
  ["2026-03-08T07:00:00Z", "America/New_York", "2026-03-08T03:00"],
  ["2026-11-01T07:00:00Z", "America/New_York", "2026-11-01T02:00"],
  // Across the year's end.
  ["2026-01-01T04:59:00Z", "America/New_York", "2025-12-31T23:59"],
  // Without a zone, the device's.
  ["2026-11-16T15:30:00Z", undefined, "2026-11-17T00:30"],
];

for (const [moment, zone, value] of fields)
  test(`the field holds ${value} for ${moment} in ${zone ?? "the device's zone"}, and names that moment again`, () => {
    expect(localInput(new Date(moment), zone)).toBe(value);
    expect(localInstant(value, zone).toISOString()).toBe(new Date(moment).toISOString());
  });

test("a moment read back from its field is the same to the minute, its seconds dropped", () => {
  const moment = new Date("2026-11-16T03:30:42.500Z");

  expect(localInstant(localInput(moment, "America/New_York"), "America/New_York").toISOString()).toBe(
    "2026-11-16T03:30:00.000Z",
  );
});

/** Whether `value` names no moment in `zone`. */
const namesNone = (value: string, zone?: string) => Number.isNaN(localInstant(value, zone).getTime());

test("a field that holds no date and time names no moment", () => {
  for (const value of ["", "2026-11-16", "tomorrow"])
    expect([value, namesNone(value, "America/New_York")]).toEqual([value, true]);
  expect(namesNone("")).toBe(true);
});

// A finding of #3199: the field's value is read as a moment even when the calendar or the clock has no such date or
// time (February 30 reads as March 2), where the function is said to give no moment. A browser's own field never
// holds one; a field of a browser without it is a text field, which does.
test.failing("a date or a time that does not exist names no moment", () => {
  for (const value of ["2026-02-30T10:00", "2026-13-01T10:00", "2026-11-16T25:00", "2026-11-16T10:61"])
    expect([value, namesNone(value, "America/New_York")]).toEqual([value, true]);
});
