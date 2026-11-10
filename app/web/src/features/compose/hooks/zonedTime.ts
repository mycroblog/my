// SPDX-License-Identifier: AGPL-3.0-only
// The wall-clock time of a time zone, for the date and time fields of a schedule (FR-007): the fields hold a date and
// a time in the user's time zone, whatever the browser's, and the server takes an instant. Intl.DateTimeFormat gives
// the zone's wall clock at an instant; the instant of a wall clock is found from the zone's offset there. The values are
// those of `<input type="date">` (YYYY-MM-DD) and `<input type="time">` (HH:mm), never shown as text.

/** A date and a time of day as the date and time fields hold them. */
export type WallClock = { date: string; time: string };

const pad = (n: number) => String(n).padStart(2, "0");

/** The fields of `at`'s wall clock in `timeZone`. */
function wallClockParts(at: Date, timeZone: string) {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone,
    hourCycle: "h23",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).formatToParts(at);
  const part = (type: Intl.DateTimeFormatPartTypes) => Number(parts.find((p) => p.type === type)?.value ?? "0");
  return {
    year: part("year"),
    month: part("month"),
    day: part("day"),
    hour: part("hour"),
    minute: part("minute"),
    second: part("second"),
  };
}

/** How far `timeZone` is ahead of UTC at `at`, in milliseconds. */
function offsetAt(at: Date, timeZone: string): number {
  const p = wallClockParts(at, timeZone);
  const asUtc = Date.UTC(p.year, p.month - 1, p.day, p.hour, p.minute, p.second);
  return asUtc - Math.floor(at.getTime() / 1000) * 1000;
}

/** The wall clock of `at` in `timeZone`, to the minute. */
export function wallClockOf(at: Date, timeZone: string): WallClock {
  const p = wallClockParts(at, timeZone);
  return {
    date: `${String(p.year).padStart(4, "0")}-${pad(p.month)}-${pad(p.day)}`,
    time: `${pad(p.hour)}:${pad(p.minute)}`,
  };
}

/**
 * The instant that the wall clock `clock` means in `timeZone`, or null when the fields do not hold a date and a time.
 * A time that the zone skips (the hour a clock moves forward) is read after the change, as the clock would show it;
 * a time that happens twice (the hour a clock moves back) is the first.
 */
export function instantOf(clock: WallClock, timeZone: string): Date | null {
  const date = /^(\d{4})-(\d{2})-(\d{2})$/.exec(clock.date);
  const time = /^(\d{2}):(\d{2})$/.exec(clock.time);
  if (date === null || time === null) return null;
  const [year, month, day] = [Number(date[1]), Number(date[2]), Number(date[3])];
  const [hour, minute] = [Number(time[1]), Number(time[2])];
  const asUtc = Date.UTC(year, month - 1, day, hour, minute);
  const check = new Date(asUtc);
  if (check.getUTCMonth() !== month - 1 || check.getUTCDate() !== day || hour > 23 || minute > 59) return null;
  // The offset at the guess, then at the instant it gives: they differ only around a change of the offset.
  const first = asUtc - offsetAt(new Date(asUtc), timeZone);
  const second = asUtc - offsetAt(new Date(first), timeZone);
  if (first === second) return new Date(first);
  // Around a change: the earlier reading of a repeated time, or, for a skipped one, the instant after the change.
  const earlier = Math.min(first, second);
  const later = Math.max(first, second);
  const readsBack = (at: number) => {
    const c = wallClockOf(new Date(at), timeZone);
    return c.date === clock.date && c.time === clock.time;
  };
  if (readsBack(earlier)) return new Date(earlier);
  if (readsBack(later)) return new Date(later);
  return new Date(later);
}

/** The time zone the user's dates are shown in: the setting (`timeZone`), or the browser's when it follows the browser. */
export function effectiveTimeZone(timeZone: string | undefined): string {
  return timeZone ?? new Intl.DateTimeFormat().resolvedOptions().timeZone;
}

/** The name of `timeZone` in `locale` (CLDR's generic name, "Japan Standard Time"), or its IANA name when CLDR has none. */
export function timeZoneName(timeZone: string, locale: string, at: Date): string {
  try {
    const parts = new Intl.DateTimeFormat(locale, { timeZone, timeZoneName: "longGeneric" }).formatToParts(at);
    return parts.find((part) => part.type === "timeZoneName")?.value ?? timeZone;
  } catch {
    return timeZone;
  }
}
