// SPDX-License-Identifier: AGPL-3.0-only

// The reader's time zone: what a moment reads as on a zone's clock and calendar, the moment a date and a time of the
// zone name, and the zone a part of the page is given. Without a zone it is the device's, which is the process's
// here, so the tests set it and put it back.

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import type { ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { dateFormat, instantAt, TimeZoneProvider, timeZoneName, useTimeZone, wallTime, type WallTime } from "./zone";

const device = process.env.TZ;

beforeAll(() => {
  process.env.TZ = "Asia/Tokyo";
});

afterAll(() => {
  if (device === undefined) delete process.env.TZ;
  else process.env.TZ = device;
});

const wall = (year: number, month: number, day: number, hour: number, minute: number): WallTime => ({
  year,
  month,
  day,
  hour,
  minute,
});

describe("a moment and what a zone's clock reads at it", () => {
  // Each moment, the zone, and the date and the time there.
  const table: readonly (readonly [string, string | undefined, WallTime])[] = [
    ["2026-11-16T15:30:00Z", "UTC", wall(2026, 11, 16, 15, 30)],
    // Late in the day it is already the next day in Tokyo, and still the day before in New York.
    ["2026-11-16T15:30:00Z", "Asia/Tokyo", wall(2026, 11, 17, 0, 30)],
    ["2026-11-16T03:30:00Z", "America/New_York", wall(2026, 11, 15, 22, 30)],
    // A zone half an hour off the hour.
    ["2026-11-16T09:00:00Z", "Asia/Kolkata", wall(2026, 11, 16, 14, 30)],
    // A zone whose summer time moves the clock by half an hour: +11:00 in January, +10:30 in July.
    ["2026-01-15T00:00:00Z", "Australia/Lord_Howe", wall(2026, 1, 15, 11, 0)],
    ["2026-07-15T00:00:00Z", "Australia/Lord_Howe", wall(2026, 7, 15, 10, 30)],
    // Its clock goes from 01:59 to 02:30 when summer time begins.
    ["2026-10-03T15:29:00Z", "Australia/Lord_Howe", wall(2026, 10, 4, 1, 59)],
    ["2026-10-03T15:30:00Z", "Australia/Lord_Howe", wall(2026, 10, 4, 2, 30)],
    // New York's goes from 01:59 to 03:00 in March, and is back on winter time at 02:00 in November.
    ["2026-03-08T06:59:00Z", "America/New_York", wall(2026, 3, 8, 1, 59)],
    ["2026-03-08T07:00:00Z", "America/New_York", wall(2026, 3, 8, 3, 0)],
    ["2026-11-01T04:59:00Z", "America/New_York", wall(2026, 11, 1, 0, 59)],
    ["2026-11-01T07:00:00Z", "America/New_York", wall(2026, 11, 1, 2, 0)],
    // Across the year's end.
    ["2026-01-01T04:59:00Z", "America/New_York", wall(2025, 12, 31, 23, 59)],
    // Without a zone, the device's.
    ["2026-11-16T15:30:00Z", undefined, wall(2026, 11, 17, 0, 30)],
  ];

  for (const [moment, zone, reads] of table) {
    test(`${moment} in ${zone ?? "the device's zone"}`, () => {
      expect(wallTime(new Date(moment), zone)).toEqual(reads);
      expect(instantAt(reads, zone).toISOString()).toBe(new Date(moment).toISOString());
    });
  }

  test("the seconds of a moment are not part of what the clock reads", () => {
    expect(wallTime(new Date("2026-11-16T09:00:42.500Z"), "Asia/Kolkata")).toEqual(wall(2026, 11, 16, 14, 30));
  });

  test("a time the zone's clock reads twice names one of its two moments", () => {
    // 01:30 on the night New York's clock goes back: 05:30 and 06:30 UTC both read so.
    const twice = wall(2026, 11, 1, 1, 30);
    const named = instantAt(twice, "America/New_York");

    expect(["2026-11-01T05:30:00.000Z", "2026-11-01T06:30:00.000Z"]).toContain(named.toISOString());
    expect(wallTime(named, "America/New_York")).toEqual(twice);
  });

  test("a time the zone's clock skips names a moment beside the change, never none", () => {
    // 02:30 on the night New York's clock goes forward: the hour before the change, or the hour after.
    const named = instantAt(wall(2026, 3, 8, 2, 30), "America/New_York");

    expect(["2026-03-08T06:30:00.000Z", "2026-03-08T07:30:00.000Z"]).toContain(named.toISOString());
  });
});

describe("a date written for a reader", () => {
  const at = new Date("2026-11-16T03:30:00Z");

  test("reads in the zone given, in the reader's locale", () => {
    const options = { dateStyle: "long", timeStyle: "short" } as const;

    expect(dateFormat("en", "America/New_York", options).format(at)).toBe("November 15, 2026 at 10:30 PM");
    expect(dateFormat("ja", "America/New_York", options).format(at)).toBe("2026年11月15日 22:30");
    expect(dateFormat("en", "Asia/Kolkata", options).format(at)).toBe("November 16, 2026 at 9:00 AM");
  });

  test("without a zone, in the device's", () => {
    expect(dateFormat("en", undefined, { dateStyle: "long", timeStyle: "short" }).format(at)).toBe(
      "November 16, 2026 at 12:30 PM",
    );
  });

  test("the zone its options name wins over the reader's", () => {
    expect(dateFormat("en", "America/New_York", { timeStyle: "short", timeZone: "UTC" }).format(at)).toBe("3:30 AM");
  });
});

test("a zone's name is its own, and the device's without one", () => {
  expect(timeZoneName("America/New_York")).toBe("America/New_York");
  expect(timeZoneName(undefined)).toBe("Asia/Tokyo");
});

describe("the zone a part of the page reads its times in", () => {
  const devices = "the device's";

  /** What `useTimeZone` gives under `around`, as `devices` when it gives none. */
  function zoneUnder(around: (children: ReactNode) => ReactNode): string | undefined {
    const seen: { zone?: string } = {};
    function Zone() {
      seen.zone = useTimeZone() ?? devices;
      return null;
    }
    renderToStaticMarkup(around(<Zone />));
    return seen.zone;
  }

  test("is the one its provider gives", () => {
    expect(zoneUnder((part) => <TimeZoneProvider timeZone="America/New_York">{part}</TimeZoneProvider>)).toBe(
      "America/New_York",
    );
  });

  test("is the device's with no provider, and under one that names none", () => {
    expect(zoneUnder((part) => part)).toBe(devices);
    expect(zoneUnder((part) => <TimeZoneProvider timeZone={undefined}>{part}</TimeZoneProvider>)).toBe(devices);
  });

  test("is the device's when the platform does not know the zone given", () => {
    expect(zoneUnder((part) => <TimeZoneProvider timeZone="Mars/Olympus_Mons">{part}</TimeZoneProvider>)).toBe(devices);
  });

  test("is the nearest provider's", () => {
    expect(
      zoneUnder((part) => (
        <TimeZoneProvider timeZone="America/New_York">
          <TimeZoneProvider timeZone="Asia/Kolkata">{part}</TimeZoneProvider>
        </TimeZoneProvider>
      )),
    ).toBe("Asia/Kolkata");
  });
});
