// SPDX-License-Identifier: AGPL-3.0-only

// The time of an event as every screen says it (ADR-0161 §5, FR-951): the server keeps it in UTC with the event's own
// IANA time zone; a screen shows it in the reader's zone, and when the event's zone is another, its time there too,
// with the place the zone is named after ("Sat 9:00 PM · 2:00 PM (Tokyo)"). The column beside the timeline, the list of
// events and an event's page read it here; the shapes are written here, since the package depends on no API client.

import { dateFormat, timeZoneName, useLocale, useTimeZone, wallTime } from "@my/design-system";
import { stringsIn } from "./locale";
import { zoneOf } from "./zone-links";

/** What the time of an event reads of it (the API's Event, and a candidate date with its event's zone). */
export type EventTimed = { readonly starts_at: string; readonly time_zone: string };

/**
 * The place an IANA time zone is named after, as a reader knows it: the last part of its name, with spaces
 * (`America/New_York` is New York, `Asia/Tokyo` Tokyo); the whole name for one of one part (UTC). A zone of a fixed
 * offset is named by it from UTC, with the sign a reader reads: POSIX writes it the other way, so `Etc/GMT+9` is
 * UTC−9; `Etc/UTC` and `Etc/GMT` are UTC.
 */
export function zonePlace(zone: string): string {
  const fixed = /^Etc\/(?:GMT|UTC)(?:([+-])(\d{1,2}))?$/.exec(zone);
  if (fixed !== null || /^Etc\/(?:Universal|Zulu|UCT|Greenwich)$/.test(zone))
    return fixed?.[1] === undefined || fixed[2] === "0" ? "UTC" : `UTC${fixed[1] === "+" ? "−" : "+"}${fixed[2]}`;
  return (zone.split("/").at(-1) ?? zone).replaceAll("_", " ");
}

/** A moment of an event: in the reader's zone, and in the event's own with its place when that zone is another. */
export type EventMoment = {
  /** The moment in the reader's zone. */
  readonly here: string;
  /**
   * The moment's time in the event's zone, its weekday too when the day there is another, and the zone's place; none
   * when the event's zone is the reader's, or one the platform does not know.
   */
  readonly there?: { readonly time: string; readonly zone: string } | undefined;
};

/** How a moment shows in the reader's zone unless a screen says otherwise: the weekday, the day, and the time. */
const momentOptions: Intl.DateTimeFormatOptions = {
  weekday: "short",
  month: "short",
  day: "numeric",
  hour: "numeric",
  minute: "2-digit",
};

/**
 * The IANA zone that `zone` names (it throws for a zone the platform does not know): the name the platform resolves it
 * to, then the zone of that name when it is another name of one (zone-links.ts), so that two names of one zone
 * (Asia/Calcutta, Asia/Kolkata) are one on every platform, while two zones whose clocks agree (Tokyo, Seoul) are never
 * taken for one.
 */
const resolved = (locale: string, zone: string) =>
  zoneOf(dateFormat(locale, undefined, { timeZone: zone }).resolvedOptions().timeZone);

/**
 * The moment `at` of an event of the zone `eventZone`, in `locale`, for a reader in `timeZone` (an IANA name; the
 * device's for undefined), its time in the reader's zone written with `options`.
 */
export function eventMoment(
  locale: string,
  timeZone: string | undefined,
  at: Date,
  eventZone: string,
  options: Intl.DateTimeFormatOptions = momentOptions,
): EventMoment {
  const here = dateFormat(locale, timeZone, options).format(at);
  const reader = timeZoneName(timeZone);
  try {
    if (reader === eventZone) return { here };
    // The event's zone is the options' own, which the platform refuses when it does not know it.
    if (resolved(locale, reader) === resolved(locale, eventZone)) return { here };
    const sameDay = wallTime(at, reader).day === wallTime(at, eventZone).day;
    const time = dateFormat(locale, timeZone, {
      ...(sameDay ? {} : { weekday: "short" }),
      hour: "numeric",
      minute: "2-digit",
      timeZone: eventZone,
    }).format(at);
    return { here, there: { time, zone: zonePlace(eventZone) } };
  } catch {
    // A time zone the platform does not know: the reader's time alone.
    return { here };
  }
}

/** The moment `at` of an event of the zone `eventZone`, for the reader, in the zone and the locale the app placed. */
export function useEventMoment(at: string, eventZone: string, options?: Intl.DateTimeFormatOptions): EventMoment {
  const { locale } = useLocale();
  return eventMoment(locale, useTimeZone(), new Date(at), eventZone, options);
}

/** A moment as one line: the reader's time, then the event's time and place when its zone is another. */
export function useMomentLine(moment: EventMoment): string {
  const { t } = stringsIn(useLocale().locale);
  return moment.there === undefined
    ? moment.here
    : t("event.elsewhere", { here: moment.here, there: moment.there.time, zone: moment.there.zone });
}

/** When `event` starts, as one line: in the reader's zone, and in the event's own with its place when that is another. */
export function useEventTime(event: EventTimed): string {
  return useMomentLine(useEventMoment(event.starts_at, event.time_zone));
}

/** What the time of an event's whole span reads of it (an event, or a candidate date with its event's zone). */
export type EventSpan = EventTimed & { readonly ends_at: string };

/**
 * When an event is, as the design system's `EventTime` takes it: its start for the `<time>`, its start to its end in
 * the reader's zone ("Sat, Nov 21, 9:00 – 11:00 PM"), and its start's time in the event's own zone with the zone's
 * place where that zone is another.
 */
export type EventWhen = {
  readonly dateTime: string;
  readonly shown: string;
  readonly own?: { readonly shown: string; readonly zone: string } | undefined;
};

/** When the span `span` is, in `locale`, for a reader in `timeZone` (an IANA name; the device's for undefined). */
export function eventWhen(locale: string, timeZone: string | undefined, span: EventSpan): EventWhen {
  const start = new Date(span.starts_at);
  const end = new Date(span.ends_at);
  const shown =
    Date.parse(span.ends_at) > Date.parse(span.starts_at)
      ? dateFormat(locale, timeZone, momentOptions).formatRange(start, end)
      : dateFormat(locale, timeZone, momentOptions).format(start);
  const { there } = eventMoment(locale, timeZone, start, span.time_zone);
  return {
    dateTime: span.starts_at,
    shown,
    ...(there === undefined ? {} : { own: { shown: there.time, zone: there.zone } }),
  };
}

/** When the span `span` is, for the reader, in the zone and the locale the app placed. */
export function useEventWhen(span: EventSpan): EventWhen {
  const { locale } = useLocale();
  return eventWhen(locale, useTimeZone(), span);
}
