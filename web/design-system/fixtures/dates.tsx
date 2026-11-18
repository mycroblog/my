// SPDX-License-Identifier: AGPL-3.0-only
// restrict/zone-dates: every date and time shows in the reader's time zone (ADR-0009 §3), which zone.tsx alone reads.
// A part formats with dateFormat and useTimeZone, and reads a clock with wallTime and instantAt; a formatter of its
// own and the methods of a Date that read, set or write out the device's wall clock are refused.
// web/devtools/src/restrictions.test.js lints it; the lint of the package leaves it out.

import { dateFormat, instantAt, useTimeZone, wallTime } from "../src/zone";

// The one source: the reader's zone, a formatter in it, the wall clock of it, and a moment from one.
export function Stamp({ at, locale }: { at: Date; locale: string }) {
  const timeZone = useTimeZone();
  const wall = wallTime(at, timeZone);
  const morning = instantAt({ ...wall, hour: 9, minute: 0 }, timeZone);
  return (
    <time dateTime={at.toISOString()}>{dateFormat(locale, timeZone, { dateStyle: "medium" }).format(morning)}</time>
  );
}

// What reads no wall clock stays open: a moment, its UTC parts, its ISO form, a number, a relative time.
export const open = (at: Date, locale: string) => [
  new Date(),
  new Date(at.getTime() + 86_400_000),
  new Date("2026-11-18T09:00:00Z"),
  Date.now(),
  Date.UTC(2026, 10, 18),
  at.getUTCHours(),
  at.setUTCDate(1),
  at.toISOString(),
  at.valueOf(),
  new Intl.NumberFormat(locale).format(1234),
  new Intl.RelativeTimeFormat(locale).format(-1, "day"),
  Intl.DateTimeFormat.supportedLocalesOf([locale]),
];

// A formatter of its own shows the device's zone.
export const own = (at: Date, locale: string) => [
  new Intl.DateTimeFormat(locale, { dateStyle: "medium" }).format(at), // error: date Intl.DateTimeFormat
  Intl.DateTimeFormat(locale).format(at), // error: date Intl.DateTimeFormat
  new Intl.DateTimeFormat().resolvedOptions().timeZone, // error: date Intl.DateTimeFormat
  new (Intl.DateTimeFormat as typeof Intl.DateTimeFormat)(locale), // error: date Intl.DateTimeFormat
];

// The device's clock and calendar, read, set, written out, or given as the parts of a moment.
export function local(at: Date, days: number, count: number) {
  const next = new Date(at);
  next.setDate(days); // error: date .setDate()
  next.setHours(9, 0, 0, 0); // error: date .setHours()
  next.setFullYear(2027); // error: date .setFullYear()
  return [
    at.getDate(), // error: date .getDate()
    at.getFullYear(), // error: date .getFullYear()
    at.getMonth(), // error: date .getMonth()
    at.getDay(), // error: date .getDay()
    at.getHours(), // error: date .getHours()
    at.getMinutes(), // error: date .getMinutes()
    at["getSeconds"](), // error: date .getSeconds()
    at.getTimezoneOffset(), // error: date .getTimezoneOffset()
    at.toLocaleString(), // error: date .toLocaleString()
    at.toLocaleDateString("en"), // error: date .toLocaleDateString()
    at.toLocaleTimeString("en", { timeZone: "UTC" }), // error: date .toLocaleTimeString()
    at.toDateString(), // error: date .toDateString()
    count.toLocaleString("en"), // error: date .toLocaleString()
    new Date(2026, 10, 18), // error: date Date(y,m,…)
    new Date(at.getUTCFullYear(), 0), // error: date Date(y,m,…)
    next,
  ];
}

// A binding of the file that takes a global's name is the file's own.
export function shadowed(
  Intl: { DateTimeFormat: (locale: string) => string },
  Date: new (y: number, m: number) => object,
) {
  return [Intl.DateTimeFormat("en"), new Date(2026, 10)];
}
