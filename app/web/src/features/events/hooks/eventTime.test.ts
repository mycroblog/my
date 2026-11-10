// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, it } from "vitest";
import { fromZonedInput, sameZone, toZonedInput, zoneCity, zoneOffset } from "./eventTime";

describe("zoneCity", () => {
  it("names a zone after its place", () => {
    expect(zoneCity("Asia/Tokyo")).toBe("Tokyo");
    expect(zoneCity("America/New_York")).toBe("New York");
    expect(zoneCity("America/Argentina/Buenos_Aires")).toBe("Buenos Aires");
    expect(zoneCity("UTC")).toBe("UTC");
  });
});

describe("zoneOffset", () => {
  it("is the zone's difference from UTC at the instant", () => {
    expect(zoneOffset("Asia/Tokyo", Date.parse("2026-12-05T12:00:00Z"))).toBe(9 * 3_600_000);
    expect(zoneOffset("America/New_York", Date.parse("2026-12-05T12:00:00Z"))).toBe(-5 * 3_600_000);
    expect(zoneOffset("America/New_York", Date.parse("2026-07-05T12:00:00Z"))).toBe(-4 * 3_600_000);
  });
});

describe("toZonedInput and fromZonedInput", () => {
  it("hold the wall clock of the event's zone", () => {
    expect(toZonedInput("2026-12-05T12:00:00Z", "Asia/Tokyo")).toBe("2026-12-05T21:00");
    expect(fromZonedInput("2026-12-05T21:00", "Asia/Tokyo")?.toISOString()).toBe("2026-12-05T12:00:00.000Z");
    expect(fromZonedInput("2026-07-05T09:30", "America/New_York")?.toISOString()).toBe("2026-07-05T13:30:00.000Z");
  });

  it("read a time the zone has twice as the first, and one it skips with the offset before the change", () => {
    // New York falls back at 2:00 on 2026-11-01: 1:30 comes first in daylight time.
    expect(fromZonedInput("2026-11-01T01:30", "America/New_York")?.toISOString()).toBe("2026-11-01T05:30:00.000Z");
    // Berlin falls back at 3:00 on 2026-10-25: 2:30 comes first in summer time.
    expect(fromZonedInput("2026-10-25T02:30", "Europe/Berlin")?.toISOString()).toBe("2026-10-25T00:30:00.000Z");
    // New York springs forward at 2:00 on 2026-03-08: 2:30 does not exist and reads as 3:30 daylight time.
    expect(fromZonedInput("2026-03-08T02:30", "America/New_York")?.toISOString()).toBe("2026-03-08T07:30:00.000Z");
  });

  it("refuse what is not a date and a time, or a zone the browser does not know", () => {
    expect(fromZonedInput("", "Asia/Tokyo")).toBeUndefined();
    expect(fromZonedInput("tomorrow", "Asia/Tokyo")).toBeUndefined();
    expect(fromZonedInput("2026-12-05T21:00", "Mars/Olympus")).toBeUndefined();
    expect(toZonedInput(undefined, "Asia/Tokyo")).toBe("");
  });
});

describe("sameZone", () => {
  it("tells the names of one zone from other zones", () => {
    expect(sameZone("Asia/Calcutta", "Asia/Kolkata")).toBe(true);
    expect(sameZone("Asia/Tokyo", "Asia/Seoul")).toBe(false);
  });
});
