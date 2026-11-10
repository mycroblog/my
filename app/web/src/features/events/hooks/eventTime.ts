// SPDX-License-Identifier: AGPL-3.0-only
// The times of an event in two time zones (ADR-0161 §5): the server keeps them in UTC with the event's IANA time zone.
// The pages show them in the viewer's zone, and also in the event's when the two differ ("Sat 9:00 PM (Tokyo)"); the
// editor's date fields hold the wall clock of the event's zone, so an organizer travelling abroad still types the
// local time of the place. These functions are pure: the hooks format with them.

/** The parts of a wall clock, as `Intl.DateTimeFormat` gives them in `en-US` with a 24-hour clock. */
function wallClock(
  zone: string,
  at: number,
): { year: number; month: number; day: number; hour: number; minute: number; second: number } {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: zone,
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

/** How far the wall clock of `zone` is ahead of UTC at the instant `at` (ms since the epoch), in ms. */
export function zoneOffset(zone: string, at: number): number {
  const c = wallClock(zone, at);
  const asUtc = Date.UTC(c.year, c.month - 1, c.day, c.hour, c.minute, c.second);
  return asUtc - Math.floor(at / 1000) * 1000;
}

/** Whether `zone` is an IANA time zone that this browser knows. */
export function isKnownTimeZone(zone: string): boolean {
  try {
    new Intl.DateTimeFormat("en", { timeZone: zone });
    return true;
  } catch {
    return false;
  }
}

/** Whether two IANA names are one zone, such as Asia/Calcutta and Asia/Kolkata. */
export function sameZone(a: string, b: string): boolean {
  const canonical = (zone: string) => {
    try {
      return new Intl.DateTimeFormat("en", { timeZone: zone }).resolvedOptions().timeZone;
    } catch {
      return zone;
    }
  };
  return a === b || canonical(a) === canonical(b);
}

/**
 * The place an IANA zone is named after, which the pages show beside the event's own time: the last part of the name
 * with spaces ("America/New_York" → "New York", "Asia/Tokyo" → "Tokyo"), or the whole name for one of one part (UTC).
 */
export function zoneCity(zone: string): string {
  const last = zone.split("/").at(-1) ?? zone;
  return last.replaceAll("_", " ");
}

const pad = (n: number) => String(n).padStart(2, "0");

/** A day in ms. */
const dayMs = 24 * 60 * 60_000;

/** An instant (ISO 8601) as the value of an `<input type="datetime-local">` in `zone`, to the minute; "" for none. */
export function toZonedInput(iso: string | undefined, zone: string): string {
  if (iso === undefined) return "";
  const at = Date.parse(iso);
  if (Number.isNaN(at) || !isKnownTimeZone(zone)) return "";
  const c = wallClock(zone, at);
  return `${String(c.year)}-${pad(c.month)}-${pad(c.day)}T${pad(c.hour)}:${pad(c.minute)}`;
}

/** The match of a value of `<input type="datetime-local">`: a date and a time to the minute, with optional seconds. */
const inputPattern = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/u;

/**
 * The instant that a value of `<input type="datetime-local">` means as the wall clock of `zone`; undefined when the
 * value is not one or the zone is unknown. A time that the zone skips (the hour a clock springs forward) is read with
 * the offset before the change, and a time it has twice (falling back) is the first of the two.
 */
export function fromZonedInput(value: string, zone: string): Date | undefined {
  const match = inputPattern.exec(value.trim());
  if (match === null || !isKnownTimeZone(zone)) return undefined;
  const [, year, month, day, hour, minute, second] = match;
  const guess = Date.UTC(
    Number(year),
    Number(month) - 1,
    Number(day),
    Number(hour),
    Number(minute),
    Number(second ?? "0"),
  );
  if (Number.isNaN(guess)) return undefined;
  // The zone's offsets a day around the wall clock: one offset, or the two of a change between them.
  const offsets = [...new Set([zoneOffset(zone, guess - dayMs), zoneOffset(zone, guess + dayMs)])];
  const wanted = toZonedInput(new Date(guess).toISOString(), "UTC");
  const instants = offsets.map((offset) => guess - offset);
  const valid = instants.filter((at) => toZonedInput(new Date(at).toISOString(), zone) === wanted);
  // A time the zone has twice is the first; one it skips is read with the smaller offset of before the change.
  return new Date(valid.length > 0 ? Math.min(...valid) : Math.max(...instants));
}
