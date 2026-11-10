// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, test } from "vitest";
import { defaultScheduleTime } from "./useSchedule";
import { instantOf, timeZoneName, wallClockOf } from "./zonedTime";

describe("the wall clock of a time zone", () => {
  test("reads an instant in the zone, whatever the browser's", () => {
    expect(wallClockOf(new Date("2026-11-10T00:30:00Z"), "Asia/Tokyo")).toEqual({ date: "2026-11-10", time: "09:30" });
    expect(wallClockOf(new Date("2026-11-10T00:30:00Z"), "America/New_York")).toEqual({
      date: "2026-11-09",
      time: "19:30",
    });
  });

  test("finds the instant of a date and a time in the zone", () => {
    expect(instantOf({ date: "2026-11-11", time: "09:00" }, "Asia/Tokyo")?.toISOString()).toBe(
      "2026-11-11T00:00:00.000Z",
    );
    expect(instantOf({ date: "2026-07-01", time: "09:00" }, "America/New_York")?.toISOString()).toBe(
      "2026-07-01T13:00:00.000Z",
    );
    expect(instantOf({ date: "2026-12-01", time: "09:00" }, "America/New_York")?.toISOString()).toBe(
      "2026-12-01T14:00:00.000Z",
    );
  });

  test("reads a time the clock skips after the change, and a repeated time as its first", () => {
    // New York moves from 02:00 to 03:00 on 2026-03-08, and back from 02:00 to 01:00 on 2026-11-01.
    expect(instantOf({ date: "2026-03-08", time: "02:30" }, "America/New_York")?.toISOString()).toBe(
      "2026-03-08T07:30:00.000Z",
    );
    expect(instantOf({ date: "2026-11-01", time: "01:30" }, "America/New_York")?.toISOString()).toBe(
      "2026-11-01T05:30:00.000Z",
    );
  });

  test("refuses fields that hold no date or no time", () => {
    expect(instantOf({ date: "", time: "09:00" }, "UTC")).toBeNull();
    expect(instantOf({ date: "2026-02-30", time: "09:00" }, "UTC")).toBeNull();
    expect(instantOf({ date: "2026-02-01", time: "" }, "UTC")).toBeNull();
  });

  test("names the zone in the display language", () => {
    expect(timeZoneName("Asia/Tokyo", "en", new Date("2026-11-10T00:00:00Z"))).toBe("Japan Standard Time");
  });
});

describe("the time a new schedule starts from", () => {
  test("is the next full hour at least the least lead time away", () => {
    const now = Date.parse("2026-11-10T08:56:00Z");
    expect(defaultScheduleTime(now, 5).toISOString()).toBe("2026-11-10T10:00:00.000Z");
    expect(defaultScheduleTime(Date.parse("2026-11-10T08:20:00Z"), 5).toISOString()).toBe("2026-11-10T09:00:00.000Z");
  });
});
