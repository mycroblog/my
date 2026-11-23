// SPDX-License-Identifier: AGPL-3.0-only

// An event as a line of a list (ADR-0161 §9, FR-951): the events' screen and the profile of its organizer draw it alike,
// the design system's EventLine, from the event the API sends, whose shape is written here: the package depends on no
// API client. Its date's box and its time are in the reader's zone, with the event's own time where its zone is
// another; a date polled says so in their place; its places are named by kind, the ones the reader does not see too.

import {
  dateFormat,
  EventLine,
  EventTime,
  useLocale,
  useTimeZone,
  type Headcount,
  type PlaceKind,
} from "@my/design-system";
import { nameOf } from "./account";
import { useEventWhen } from "./event-time";
import { stringsIn } from "./locale";
import { eventPath } from "./paths";
import type { PostAccount } from "./status";

/** An event, as its line reads it (the API's Event). */
export type EventShown = {
  readonly id: string;
  readonly title: string;
  readonly organizer: PostAccount;
  readonly time_zone: string;
  readonly starts_at: string;
  readonly ends_at: string;
  readonly state: "unscheduled" | "scheduled" | "canceled";
  readonly places: readonly { readonly kind: PlaceKind }[];
  readonly hidden_places: readonly PlaceKind[];
  readonly going_count: number;
};

/** The kinds of places in the order every screen names them. */
const kinds: readonly PlaceKind[] = ["voice", "physical", "url"];

/** How many answered going to an event, as its figure and its words: going, or went once it ended. */
export function useGoingCount(count: number, ended: boolean): Headcount {
  const { locale } = useLocale();
  const { t } = stringsIn(locale);
  return {
    shown: new Intl.NumberFormat(locale).format(count),
    label: t(ended ? "event.went" : "event.going", { count }),
  };
}

/** An event as a line that leads to its page: its day, title, state, time, places, organizer and who is going. */
export function EventListLine({ event, now }: { event: EventShown; now: number }) {
  const { locale } = useLocale();
  const timeZone = useTimeZone();
  const { t } = stringsIn(locale);
  const when = useEventWhen(event);
  const ended = event.state !== "canceled" && event.state !== "unscheduled" && Date.parse(event.ends_at) <= now;
  const going = useGoingCount(event.going_count, ended);
  const polled = event.state === "unscheduled";
  const start = new Date(event.starts_at);
  const present = new Set([...event.places.map((p) => p.kind), ...event.hidden_places]);
  return (
    <EventLine
      href={eventPath(event.id)}
      title={event.title}
      date={
        polled
          ? undefined
          : {
              month: dateFormat(locale, timeZone, { month: "short" }).format(start),
              day: dateFormat(locale, timeZone, { day: "numeric" }).format(start),
            }
      }
      time={polled ? t("event.date_polled") : <EventTime {...when} />}
      places={kinds.filter((k) => present.has(k)).map((kind) => ({ kind, label: t("event.place_kind", { kind }) }))}
      organizer={t("event.by", { name: nameOf(event.organizer) })}
      going={event.going_count > 0 ? going : undefined}
      state={
        event.state === "canceled"
          ? { kind: "canceled", label: t("event.state", { state: "canceled" }) }
          : ended
            ? { kind: "ended", label: t("event.state", { state: "ended" }) }
            : undefined
      }
    />
  );
}
