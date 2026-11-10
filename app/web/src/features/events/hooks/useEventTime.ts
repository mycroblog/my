// SPDX-License-Identifier: AGPL-3.0-only
// When an event is, as its card and its page say it (ADR-0161 §5): the dates and times in the viewer's time zone
// (the user's setting, or the browser's), and, when the event's own time zone is another one, its times there with
// the place the zone is named after ("Sat 9:00 PM – 11:00 PM (Tokyo)"), so that nobody has to work out the
// difference. An event whose date poll has no date yet says so instead.
import { useIntl } from "react-intl";
import { useNow } from "../../../lib/clock";
import { useT } from "../../../lib/i18n";
import type { Event } from "../api/events";
import { isKnownTimeZone, sameZone, zoneCity } from "./eventTime";

export type EventWhen = {
  /** The start, for the `dateTime` of `<time>`. */
  dateTime: string;
  /** The dates and times in the viewer's zone, or that the date is not set yet. */
  viewer: string;
  /** The times in the event's zone with its place, when that zone is not the viewer's. */
  eventZone: string | undefined;
  /** Whether the event has a date (it is not polling for one). */
  scheduled: boolean;
  /** Whether the event has ended. */
  ended: boolean;
  /** Whether the event has started (and is not over yet, or is). */
  started: boolean;
};

/** The viewer's time zone: the one the app formats in, or the browser's. */
function viewerZone(timeZone: string | undefined): string {
  return timeZone ?? new Intl.DateTimeFormat().resolvedOptions().timeZone;
}

export function useEventWhen(event: Event): EventWhen {
  const intl = useIntl();
  const t = useT();
  const now = useNow();
  const start = Date.parse(event.starts_at);
  const end = Date.parse(event.ends_at);
  const scheduled = event.state !== "unscheduled";
  const base = { dateTime: event.starts_at, scheduled, ended: end <= now, started: start <= now };
  if (!scheduled) return { ...base, viewer: t("client:events.time.unscheduled"), eventZone: undefined };
  const viewer = intl.formatDateTimeRange(start, end, {
    weekday: "short",
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
  const zone = event.time_zone;
  const eventZone =
    !isKnownTimeZone(zone) || sameZone(viewerZone(intl.timeZone), zone)
      ? undefined
      : t("client:events.time.event_zone", {
          time: intl.formatDateTimeRange(start, end, {
            timeZone: zone,
            weekday: "short",
            hour: "numeric",
            minute: "2-digit",
          }),
          zone: zoneCity(zone),
        });
  return { ...base, viewer, eventZone };
}
