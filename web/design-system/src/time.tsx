// SPDX-License-Identifier: AGPL-3.0-only

// The relative time of a post: a <time> that counts how long ago it was in the reader's locale, "3m ago" / "3分前",
// "1d ago" / "1日前", and never says a word such as "yesterday"; after a week, the date, "Oct 23" / "10月23日", with the
// year only when it is not the current one. Its datetime attribute and its tooltip hold the full date. The days and
// the dates are those of the reader's time zone (zone.tsx). It updates while the page is open, until it reads a date.
// Every word comes from the platform's Intl.RelativeTimeFormat and Intl.DateTimeFormat, so it needs no strings of its
// own.

import { useEffect, useState } from "react";
import { useReader } from "./locale";
import { dateFormat, useTimeZone, wallTime } from "./zone";

export type RelativeTimeProps = {
  /** The moment, as a Date or an ISO 8601 string. */
  date: Date | string;
  /** The reader's locale; the I18nProvider's by default. */
  locale?: string | undefined;
};

const second = 1000;
const minute = 60 * second;
const hour = 60 * minute;
const day = 24 * hour;

/**
 * What the time reads, and whether it may read differently later while the page stays open; its days and dates are
 * those of `timeZone`, the device's zone without one.
 */
export function relativeTime(
  date: Date,
  now: Date,
  locale: string,
  timeZone?: string,
): { text: string; changes: boolean } {
  const age = Math.max(0, now.getTime() - date.getTime());
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: "always", style: "narrow" });
  const ago = (n: number, unit: Intl.RelativeTimeFormatUnit) => ({ text: rtf.format(-n, unit), changes: true });
  if (age < minute) return ago(Math.max(1, Math.floor(age / second)), "second");
  if (age < hour) return ago(Math.floor(age / minute), "minute");
  if (age < day) return ago(Math.floor(age / hour), "hour");
  const from = wallTime(date, timeZone);
  const to = wallTime(now, timeZone);
  // Whole calendar days between the two moments, in the reader's time zone.
  const days = Math.round(
    (Date.UTC(to.year, to.month - 1, to.day) - Date.UTC(from.year, from.month - 1, from.day)) / day,
  );
  if (days < 7) return ago(Math.max(1, days), "day");
  const year = from.year === to.year ? {} : { year: "numeric" as const };
  const text = dateFormat(locale, timeZone, { month: "short", day: "numeric", ...year }).format(date);
  return { text, changes: false };
}

/** How long until the text may change: every second in the first minute, then every minute, then every hour. */
const nextIn = (age: number) => (age < minute ? second : age < hour ? minute : hour);

export function RelativeTime({ date, locale }: RelativeTimeProps) {
  const at = typeof date === "string" ? new Date(date) : date;
  const { lang } = useReader(locale);
  const timeZone = useTimeZone();
  const [now, setNow] = useState(() => new Date());
  const { text, changes } = relativeTime(at, now, lang, timeZone);
  const time = at.getTime();

  useEffect(() => {
    if (!changes) return;
    const timer = setTimeout(
      () => {
        setNow(new Date());
      },
      nextIn(now.getTime() - time),
    );
    return () => {
      clearTimeout(timer);
    };
  }, [changes, now, time]);

  const full = dateFormat(lang, timeZone, { dateStyle: "long", timeStyle: "short" }).format(at);
  return (
    <time dateTime={at.toISOString()} title={full} lang={lang}>
      {text}
    </time>
  );
}
