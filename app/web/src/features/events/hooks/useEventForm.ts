// SPDX-License-Identifier: AGPL-3.0-only
// The editor of an event (ADR-0161 §4–§6): its title, description, visibility, date and time zone, places, who sees
// the places, and capacity, saved together. A new event is created with a fixed date; changing one sends only what
// differs, and every place when one of them changes (the server replaces them all). The dates are typed as the wall
// clock of the event's time zone (eventTime.ts), so that changing the zone keeps the typed times. The places this form
// does not edit (a voice chat room) are kept as they are. Only the organizer changes the visibility.
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useNow } from "../../../lib/clock";
import { useT } from "../../../lib/i18n";
import { useDisplaySettings } from "../../../lib/locale";
import {
  cacheEvent,
  createEventMutation,
  type Event,
  type EventCreate,
  eventLimits,
  type EventPlace,
  type EventPlaceDisclosure,
  type EventUpdate,
  type EventVisibility,
  updateEventMutation,
} from "../api/events";
import { fromZonedInput, toZonedInput } from "./eventTime";
import { useEventFailureMessage } from "./useEventFailureMessage";

/** A place to meet in person: its name, its address, and a link to a map. */
type PhysicalDraft = { enabled: boolean; name: string; address: string; url: string };
/** A link of another service, such as a stream or a meeting. */
type LinkDraft = { enabled: boolean; name: string; url: string };

type EventDraft = {
  title: string;
  description: string;
  visibility: EventVisibility;
  timeZone: string;
  /** The start and end as values of `<input type="datetime-local">` in `timeZone`. */
  startsAt: string;
  endsAt: string;
  placeDisclosure: EventPlaceDisclosure;
  /** The capacity as typed; empty or 0 for no limit. */
  capacity: string;
  physical: PhysicalDraft;
  link: LinkDraft;
};

/** The fields a check of the draft can fail on, named as the form's inputs are. */
export type EventField =
  | "title"
  | "description"
  | "starts_at"
  | "ends_at"
  | "capacity"
  | "physical_name"
  | "physical_address"
  | "physical_url"
  | "link_name"
  | "link_url";

type EventFieldErrors = Partial<Record<EventField, string>>;

/** How many characters (code points) `text` has, as the server counts them. */
function characterCount(text: string): number {
  return Array.from(text).length;
}

function graphemeCount(text: string): number {
  return Array.from(new Intl.Segmenter(undefined, { granularity: "grapheme" }).segment(text)).length;
}

/** Whether `line` is one line of text without control characters, at most `max` code points. */
function validLine(line: string, max: number): boolean {
  return !/\p{Cc}/u.test(line) && characterCount(line) <= max;
}

/** Whether `url` is an `http` or `https` URL that the server takes. */
function isEventUrl(url: string): boolean {
  if (characterCount(url) > eventLimits.url) return false;
  try {
    const parsed = new URL(url);
    return parsed.protocol === "http:" || parsed.protocol === "https:";
  } catch {
    return false;
  }
}

/** The draft of a new event in the user's time zone, or of `event` as it is. */
function draftOf(event: Event | undefined, zone: string): EventDraft {
  const physical = event?.places.find((place) => place.kind === "physical");
  const link = event?.places.find((place) => place.kind === "url");
  const timeZone = event?.time_zone ?? zone;
  const scheduled = event !== undefined && event.state !== "unscheduled";
  return {
    title: event?.title ?? "",
    description: event?.description ?? "",
    visibility: event?.visibility ?? "public",
    timeZone,
    startsAt: scheduled ? toZonedInput(event.starts_at, timeZone) : "",
    endsAt: scheduled ? toZonedInput(event.ends_at, timeZone) : "",
    placeDisclosure: event?.place_disclosure ?? "approved",
    capacity: event === undefined || event.capacity === 0 ? "" : String(event.capacity),
    physical: {
      enabled: physical !== undefined,
      name: physical?.name ?? "",
      address: physical?.address ?? "",
      url: physical?.url ?? "",
    },
    link: { enabled: link !== undefined, name: link?.name ?? "", url: link?.url ?? "" },
  };
}

/** The places of the draft, after the places of `event` that the form does not edit. */
function placesOf(draft: EventDraft, event: Event | undefined): EventPlace[] {
  const kept = (event?.places ?? []).filter((place) => place.kind !== "physical" && place.kind !== "url");
  return [
    ...kept,
    ...(draft.physical.enabled
      ? [
          {
            kind: "physical" as const,
            name: draft.physical.name.trim(),
            address: draft.physical.address.trim(),
            ...(draft.physical.url.trim() !== "" && { url: draft.physical.url.trim() }),
          },
        ]
      : []),
    ...(draft.link.enabled ? [{ kind: "url" as const, name: draft.link.name.trim(), url: draft.link.url.trim() }] : []),
  ];
}

/** The places as the server compares them: their kinds and fields in a fixed order. */
function placesKey(places: readonly EventPlace[]): string {
  return JSON.stringify(
    places
      .map((p) => [p.kind, p.name ?? "", p.address ?? "", p.url ?? "", p.speakers ?? ""])
      .toSorted((a, b) => String(a[0]).localeCompare(String(b[0]))),
  );
}

export type EventForm = {
  /** `new` creates an event; `edit` changes `event`. */
  mode: "new" | "edit";
  draft: EventDraft;
  change: (change: Partial<EventDraft>) => void;
  changePhysical: (change: Partial<PhysicalDraft>) => void;
  changeLink: (change: Partial<LinkDraft>) => void;
  /** Whether the user may change the visibility: the organizer only. */
  canChangeVisibility: boolean;
  /** Whether the event polls for its date, so that the form leaves the date to the poll unless one is typed. */
  unscheduled: boolean;
  submit: (event: SyntheticEvent<HTMLFormElement>) => void;
  isPending: boolean;
  errors: EventFieldErrors;
  /** Why the server refused the form, localized; the draft stays. */
  error: string | undefined;
};

/** The editor of a new event, or of `event`, which the user organizes or co-organizes. */
export function useEventForm(event?: Event): EventForm {
  const api = useApi();
  const t = useT();
  const notice = useNotice();
  const now = useNow();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const failed = useEventFailureMessage();
  const { timeZone } = useDisplaySettings();
  const [draft, setDraft] = useState<EventDraft>(() =>
    draftOf(event, timeZone ?? new Intl.DateTimeFormat().resolvedOptions().timeZone),
  );
  const [errors, setErrors] = useState<EventFieldErrors>({});
  const create = useMutation({
    ...createEventMutation(api),
    onSuccess: async (saved) => {
      await cacheEvent(api, queryClient, saved);
      notice.inPlace(t("client:events.form.created"));
      await navigate({ to: "/events/$id", params: { id: saved.id } });
    },
  });
  const update = useMutation({
    ...updateEventMutation(api, event?.id ?? ""),
    onSuccess: async (saved) => {
      await cacheEvent(api, queryClient, saved);
      notice.inPlace(t("client:events.form.saved"));
      await navigate({ to: "/events/$id", params: { id: saved.id } });
    },
  });
  const mutation = event === undefined ? create : update;
  const unscheduled = event?.state === "unscheduled";

  const check = (): { errors: EventFieldErrors; startsAt?: Date; endsAt?: Date } => {
    const found: EventFieldErrors = {};
    const title = draft.title.trim();
    if (title === "" || !validLine(title, eventLimits.titleCodePoints) || graphemeCount(title) > eventLimits.title) {
      found.title = t("client:events.form.title.invalid", { max: String(eventLimits.title) });
    }
    const description = characterCount(draft.description.trim());
    if (description > eventLimits.description) {
      found.description = t("client:events.form.characters_over", { count: description - eventLimits.description });
    }
    // An unscheduled event keeps polling unless both dates are typed (a date ends the poll).
    const dateless = unscheduled && draft.startsAt === "" && draft.endsAt === "";
    const startsAt = fromZonedInput(draft.startsAt, draft.timeZone);
    const endsAt = fromZonedInput(draft.endsAt, draft.timeZone);
    if (!dateless) {
      if (startsAt === undefined) found.starts_at = t("client:events.form.starts_at.required");
      else if (startsAt.getTime() <= now && dateChanged(event, startsAt, endsAt)) {
        found.starts_at = t("client:events.form.starts_at.past");
      }
      if (endsAt === undefined) found.ends_at = t("client:events.form.ends_at.required");
      else if (startsAt !== undefined && endsAt.getTime() <= startsAt.getTime()) {
        found.ends_at = t("client:events.form.ends_at.before_start");
      } else if (startsAt !== undefined && endsAt.getTime() - startsAt.getTime() > eventLimits.duration) {
        found.ends_at = t("client:events.form.ends_at.too_long");
      }
    }
    const capacity = draft.capacity.trim();
    if (capacity !== "" && !(/^\d+$/u.test(capacity) && Number(capacity) <= eventLimits.capacity)) {
      found.capacity = t("client:events.form.capacity.invalid", { max: String(eventLimits.capacity) });
    }
    if (draft.physical.enabled) {
      const name = draft.physical.name.trim();
      if (name === "" || !validLine(name, eventLimits.placeName)) {
        found.physical_name = t("client:events.form.place_name.invalid", { max: String(eventLimits.placeName) });
      }
      if (characterCount(draft.physical.address.trim()) > eventLimits.address) {
        found.physical_address = t("client:events.form.characters_over", {
          count: characterCount(draft.physical.address.trim()) - eventLimits.address,
        });
      }
      const url = draft.physical.url.trim();
      if (url !== "" && !isEventUrl(url)) found.physical_url = t("client:events.form.url.invalid");
    }
    if (draft.link.enabled) {
      const name = draft.link.name.trim();
      if (name === "" || !validLine(name, eventLimits.placeName)) {
        found.link_name = t("client:events.form.place_name.invalid", { max: String(eventLimits.placeName) });
      }
      if (!isEventUrl(draft.link.url.trim())) found.link_url = t("client:events.form.url.invalid");
    }
    return {
      errors: found,
      ...(startsAt !== undefined && { startsAt }),
      ...(endsAt !== undefined && { endsAt }),
    };
  };

  return {
    mode: event === undefined ? "new" : "edit",
    draft,
    change: (change) => {
      setDraft((previous) => ({ ...previous, ...change }));
    },
    changePhysical: (change) => {
      setDraft((previous) => ({ ...previous, physical: { ...previous.physical, ...change } }));
    },
    changeLink: (change) => {
      setDraft((previous) => ({ ...previous, link: { ...previous.link, ...change } }));
    },
    canChangeVisibility: event === undefined || event.viewer.organizer,
    unscheduled,
    submit: (submitted) => {
      submitted.preventDefault();
      if (mutation.isPending) return;
      const checked = check();
      setErrors(checked.errors);
      const first = Object.keys(checked.errors)[0];
      if (first !== undefined) {
        mutation.reset();
        // The first field that the check failed on takes focus, so that its message is read and it can be fixed.
        const field = submitted.currentTarget.elements.namedItem(first);
        if (field instanceof HTMLElement) field.focus();
        return;
      }
      const settings = {
        title: draft.title.trim(),
        description: draft.description.trim(),
        time_zone: draft.timeZone,
        place_disclosure: draft.placeDisclosure,
        capacity: draft.capacity.trim() === "" ? 0 : Number(draft.capacity.trim()),
        places: placesOf(draft, event),
      };
      const dates =
        checked.startsAt !== undefined && checked.endsAt !== undefined
          ? { starts_at: checked.startsAt.toISOString(), ends_at: checked.endsAt.toISOString() }
          : undefined;
      if (event === undefined) {
        const body: EventCreate = { ...settings, visibility: draft.visibility, ...dates };
        create.mutate(body);
        return;
      }
      update.mutate(changeOf(event, { ...settings, visibility: draft.visibility }, dates));
    },
    isPending: mutation.isPending,
    errors,
    error: mutation.isError ? failed(mutation.error) : undefined,
  };
}

/** Whether the typed dates differ from the event's (always for a new event). */
function dateChanged(event: Event | undefined, startsAt: Date, endsAt: Date | undefined): boolean {
  if (event === undefined || event.state === "unscheduled") return true;
  return (
    startsAt.getTime() !== Date.parse(event.starts_at) ||
    (endsAt !== undefined && endsAt.getTime() !== Date.parse(event.ends_at))
  );
}

/** What the form changes of `event`; only what differs is sent. */
function changeOf(
  event: Event,
  settings: Required<Omit<EventUpdate, "starts_at" | "ends_at">>,
  dates: { starts_at: string; ends_at: string } | undefined,
): EventUpdate {
  const change: EventUpdate = {};
  if (settings.title !== event.title) change.title = settings.title;
  if (settings.description !== event.description) change.description = settings.description;
  if (event.viewer.organizer && settings.visibility !== event.visibility) change.visibility = settings.visibility;
  if (settings.time_zone !== event.time_zone) change.time_zone = settings.time_zone;
  if (settings.place_disclosure !== event.place_disclosure) change.place_disclosure = settings.place_disclosure;
  if (settings.capacity !== event.capacity) change.capacity = settings.capacity;
  if (placesKey(settings.places) !== placesKey(event.places)) change.places = settings.places;
  if (dates !== undefined) {
    const startsAt = new Date(dates.starts_at);
    if (dateChanged(event, startsAt, new Date(dates.ends_at))) {
      change.starts_at = dates.starts_at;
      change.ends_at = dates.ends_at;
    }
  }
  return change;
}
