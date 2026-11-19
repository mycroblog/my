// SPDX-License-Identifier: AGPL-3.0-only

// The time of an event in the reader's zone and in its own (FR-951): the place a zone is named after, a fixed offset by
// its sign from UTC, and a second time wherever the event's zone is another than the reader's, its clock alike or not.

import { describe, expect, test } from "bun:test";
import { eventMoment, zonePlace } from "./event-time";

describe("zonePlace", () => {
  test("names a zone by its place, with spaces", () => {
    expect(zonePlace("Asia/Tokyo")).toBe("Tokyo");
    expect(zonePlace("America/New_York")).toBe("New York");
    expect(zonePlace("America/Argentina/Buenos_Aires")).toBe("Buenos Aires");
    expect(zonePlace("UTC")).toBe("UTC");
  });

  test("names a fixed offset from UTC with the sign a reader reads, the other way from POSIX's", () => {
    expect(zonePlace("Etc/GMT+9")).toBe("UTC−9");
    expect(zonePlace("Etc/GMT-14")).toBe("UTC+14");
    expect(zonePlace("Etc/GMT")).toBe("UTC");
    expect(zonePlace("Etc/GMT+0")).toBe("UTC");
    expect(zonePlace("Etc/UTC")).toBe("UTC");
  });
});

describe("eventMoment", () => {
  const at = new Date("2026-11-21T12:00:00Z");

  test("gives the event's own time where its clock reads otherwise than the reader's", () => {
    expect(eventMoment("en", "America/Los_Angeles", at, "Asia/Tokyo").there).toEqual({
      time: "9:00 PM",
      zone: "Tokyo",
    });
  });

  test("gives the event's own time for another zone whose clock reads alike, named by its place", () => {
    expect(eventMoment("en", "Asia/Seoul", at, "Asia/Tokyo").there).toEqual({ time: "9:00 PM", zone: "Tokyo" });
  });

  test("gives the event's own time with its weekday where its day is another than the reader's", () => {
    // 5 PM in UTC: Thursday 6 PM in Paris, Friday 2 AM in Tokyo.
    const there = eventMoment("en", "Asia/Tokyo", new Date("2026-11-26T17:00:00Z"), "Europe/Paris").there;
    expect({ time: there?.time.replace(/\s/g, " "), zone: there?.zone }).toEqual({
      time: "Thu 6:00 PM",
      zone: "Paris",
    });
  });

  test("gives the event's own time without a weekday where the day is the reader's", () => {
    const there = eventMoment("en", "America/Los_Angeles", at, "Asia/Tokyo").there;
    expect(there?.time).not.toMatch(/[A-Za-z]{3} /);
  });

  test("gives no second time for the reader's own zone", () => {
    expect(eventMoment("en", "Asia/Tokyo", at, "Asia/Tokyo").there).toBeUndefined();
  });

  test("gives no second time for a zone the platform does not know", () => {
    expect(eventMoment("en", "Asia/Tokyo", at, "Mars/Olympus").there).toBeUndefined();
  });
});
