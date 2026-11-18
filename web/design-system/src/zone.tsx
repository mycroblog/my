// SPDX-License-Identifier: AGPL-3.0-only

// The reader's time zone (ADR-0009 §3), the one source of it for everything that shows a date or a time: the zone the
// app places with TimeZoneProvider (the account's choice), the device's without one, as the locale is the one of the
// I18nProvider. A part reads it with useTimeZone() and formats with dateFormat(), in place of an Intl.DateTimeFormat
// of its own, which would show the device's zone; what reads a clock (the day a moment falls on, the time a field
// holds) goes through wallTime() and instantAt(). A zone the platform does not know reads as the device's.

import { createContext, use, type ReactNode } from "react";

const TimeZoneContext = createContext<string | undefined>(undefined);

/** Whether the platform knows the IANA time zone `timeZone`. */
function known(timeZone: string): boolean {
  try {
    new Intl.DateTimeFormat("en", { timeZone });
    return true;
  } catch {
    return false;
  }
}

/**
 * Shows every date and time under it in `timeZone`, an IANA name (Asia/Tokyo); in the device's zone with undefined, or
 * with a zone the platform does not know.
 */
export function TimeZoneProvider({ timeZone, children }: { timeZone: string | undefined; children: ReactNode }) {
  return (
    <TimeZoneContext value={timeZone !== undefined && known(timeZone) ? timeZone : undefined}>
      {children}
    </TimeZoneContext>
  );
}

/** The reader's time zone as the app placed it, an IANA name; undefined for the device's. */
export function useTimeZone(): string | undefined {
  return use(TimeZoneContext);
}

/** The IANA name of `timeZone`; of the device's zone for undefined. */
export function timeZoneName(timeZone: string | undefined): string {
  return timeZone ?? new Intl.DateTimeFormat().resolvedOptions().timeZone;
}

/**
 * A formatter of dates and times in `locale` and in `timeZone` (the device's zone for undefined), unless `options`
 * name a zone of their own (a date of the calendar, kept in UTC; an event's own zone).
 */
export function dateFormat(
  locale: string,
  timeZone: string | undefined,
  options: Intl.DateTimeFormatOptions = {},
): Intl.DateTimeFormat {
  return new Intl.DateTimeFormat(locale, { timeZone, ...options });
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

/** What the wall of `timeZone` reads at `date`; the device's for undefined. */
export function wallTime(date: Date, timeZone: string | undefined): WallTime {
  if (timeZone === undefined)
    return {
      year: date.getFullYear(),
      month: date.getMonth() + 1,
      day: date.getDate(),
      hour: date.getHours(),
      minute: date.getMinutes(),
    };
  const parts = wallOf(timeZone).formatToParts(date);
  const field = (type: Intl.DateTimeFormatPartTypes) => Number(parts.find((p) => p.type === type)?.value);
  return {
    year: field("year"),
    month: field("month"),
    day: field("day"),
    hour: field("hour"),
    minute: field("minute"),
  };
}

/** The moment the wall of UTC reads `wall`, in ms. */
const utc = (wall: WallTime) => Date.UTC(wall.year, wall.month - 1, wall.day, wall.hour, wall.minute);

/**
 * The moment the wall of `timeZone` (the device's for undefined) reads `wall`. A time the wall reads twice (clocks set
 * back) is one of its two moments; one it never reads (clocks set forward) is a moment beside the change.
 */
export function instantAt(wall: WallTime, timeZone: string | undefined): Date {
  if (timeZone === undefined) return new Date(wall.year, wall.month - 1, wall.day, wall.hour, wall.minute);
  const read = utc(wall);
  // How far the zone's wall is ahead of UTC's at a moment; asked again at the moment found, since the first guess may
  // fall on the other side of a change of the clocks.
  const ahead = (at: number) => utc(wallTime(new Date(at), timeZone)) - Math.floor(at / 60_000) * 60_000;
  return new Date(read - ahead(read - ahead(read)));
}
