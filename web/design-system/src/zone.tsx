// SPDX-License-Identifier: AGPL-3.0-only

// The reader's time zone (ADR-0009 §3), the one source of it for everything that shows a date or a time: the zone the
// app places with TimeZoneProvider (the account's choice), the device's without one, as the locale is the one of the
// I18nProvider. A part reads it with useTimeZone() and formats with dateFormat(), in place of an Intl.DateTimeFormat
// of its own, which would show the device's zone; what reads a clock (the day a moment falls on, the time a field
// holds) goes through wallTime() and instantAt(). A zone the platform does not know reads as the device's, in the
// provider and in each function given one: a name the server keeps and this browser has not heard of breaks no screen.

import { createContext, use, type ReactNode } from "react";

const TimeZoneContext = createContext<string | undefined>(undefined);

/** Whether the platform knows each IANA name it was asked about, asked once a name. */
const knowns = new Map<string, boolean>();

function known(timeZone: string): boolean {
  let answer = knowns.get(timeZone);
  if (answer === undefined) {
    try {
      new Intl.DateTimeFormat("en", { timeZone });
      answer = true;
    } catch {
      answer = false;
    }
    knowns.set(timeZone, answer);
  }
  return answer;
}

/** `timeZone` when the platform knows it; undefined, the device's zone, for none and for one it does not know. */
const zoneOf = (timeZone: string | undefined) => (timeZone !== undefined && known(timeZone) ? timeZone : undefined);

/**
 * Shows every date and time under it in `timeZone`, an IANA name (Asia/Tokyo); in the device's zone with undefined, or
 * with a zone the platform does not know.
 */
export function TimeZoneProvider({ timeZone, children }: { timeZone: string | undefined; children: ReactNode }) {
  return <TimeZoneContext value={zoneOf(timeZone)}>{children}</TimeZoneContext>;
}

/** The reader's time zone as the app placed it, an IANA name; undefined for the device's. */
export function useTimeZone(): string | undefined {
  return use(TimeZoneContext);
}

/** The IANA name of `timeZone`; of the device's zone for undefined, and for a zone the platform does not know. */
export function timeZoneName(timeZone: string | undefined): string {
  return zoneOf(timeZone) ?? new Intl.DateTimeFormat().resolvedOptions().timeZone;
}

/**
 * A formatter of dates and times in `locale` and in `timeZone` (the device's zone for undefined, and for a zone the
 * platform does not know), unless `options` name a zone of their own (a date of the calendar, kept in UTC; an event's
 * own zone), which is the platform's to refuse.
 */
export function dateFormat(
  locale: string,
  timeZone: string | undefined,
  options: Intl.DateTimeFormatOptions = {},
): Intl.DateTimeFormat {
  return new Intl.DateTimeFormat(locale, { timeZone: zoneOf(timeZone), ...options });
}

/** What a clock and a calendar on the wall of a time zone read: the month from 1, the hour from 0 to 23. */
export type WallTime = {
  readonly year: number;
  readonly month: number;
  readonly day: number;
  readonly hour: number;
  readonly minute: number;
};

/** The formatters that read a wall's clock, one a zone: a timeline reads it for every post. */
const walls = new Map<string, Intl.DateTimeFormat>();

function wallOf(timeZone: string): Intl.DateTimeFormat {
  let wall = walls.get(timeZone);
  if (wall === undefined) {
    wall = new Intl.DateTimeFormat("en-u-ca-gregory-nu-latn", {
      timeZone,
      year: "numeric",
      month: "numeric",
      day: "numeric",
      hour: "numeric",
      minute: "numeric",
      hourCycle: "h23",
    });
    walls.set(timeZone, wall);
  }
  return wall;
}

/**
 * What the wall of `timeZone` reads at `date`; the device's for undefined, and for a zone the platform does not know.
 */
export function wallTime(date: Date, timeZone: string | undefined): WallTime {
  const zone = zoneOf(timeZone);
  if (zone === undefined)
    return {
      year: date.getFullYear(),
      month: date.getMonth() + 1,
      day: date.getDate(),
      hour: date.getHours(),
      minute: date.getMinutes(),
    };
  const parts = wallOf(zone).formatToParts(date);
  const field = (type: Intl.DateTimeFormatPartTypes) => Number(parts.find((p) => p.type === type)?.value);
  return {
    year: field("year"),
    month: field("month"),
    day: field("day"),
    hour: field("hour"),
    minute: field("minute"),
  };
}

const minute = 60_000;
const day = 24 * 60 * minute;

/** The moment the wall of UTC reads `wall`, in ms. */
const utc = (wall: WallTime) => Date.UTC(wall.year, wall.month - 1, wall.day, wall.hour, wall.minute);

/** What the wall of UTC reads at the moment `at` (ms). */
function utcWall(at: number): WallTime {
  const date = new Date(at);
  return {
    year: date.getUTCFullYear(),
    month: date.getUTCMonth() + 1,
    day: date.getUTCDate(),
    hour: date.getUTCHours(),
    minute: date.getUTCMinutes(),
  };
}

const same = (a: WallTime, b: WallTime) =>
  a.year === b.year && a.month === b.month && a.day === b.day && a.hour === b.hour && a.minute === b.minute;

/**
 * The moment the wall of `timeZone` reads `wall`; of the device's for undefined, and for a zone the platform does not
 * know. A time the wall reads twice (its clock set back) is the earlier of its two moments. A time it never reads (its
 * clock set forward) is the moment that long after the change: 02:30 of a night whose clock goes from 02:00 to 03:00
 * is its 03:30. A wall that names no date or no time of any calendar and clock (a month 13, February 30, 25:00, a
 * minute 61) names no moment: the Date is invalid.
 */
export function instantAt(wall: WallTime, timeZone: string | undefined): Date {
  const read = utc(wall);
  if (!same(utcWall(read), wall)) return new Date(Number.NaN);
  /** How far the zone's wall is ahead of UTC's at a moment. */
  const ahead = (at: number) => utc(wallTime(new Date(at), timeZone)) - Math.floor(at / minute) * minute;
  // The zone is as far ahead of UTC at the moment asked for as it was a day before or as it is a day after: where its
  // clock changes between the two, the wall's time is tried by each.
  const before = read - ahead(read - day);
  const after = read - ahead(read + day);
  const reads = (at: number) => same(wallTime(new Date(at), timeZone), wall);
  if (reads(before) && reads(after)) return new Date(Math.min(before, after));
  if (reads(before)) return new Date(before);
  if (reads(after)) return new Date(after);
  // A time the clock skipped, read as the clock ran before it was set forward: as long after the change.
  return new Date(Math.max(before, after));
}
