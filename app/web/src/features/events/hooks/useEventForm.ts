// SPDX-License-Identifier: AGPL-3.0-only
// The editor of an event (ADR-0161 §4–§7): its title, description, visibility, date and time zone, places, who sees
// the places, and capacity, saved together. A new event is created with a fixed date or with the candidate dates of a
// date poll (§7), whose answers and confirmation are on the event's page; changing one sends only what differs, and
// every place when one of them changes (the server replaces them all). The dates are typed as the wall clock of the
// event's time zone (eventTime.ts), so that changing the zone keeps the typed times. The voice chat place is offered
// while the server has voice chat on and the user may open rooms (or the event has one already); otherwise an
// existing one is kept as it is. Only the organizer changes the visibility.
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useNow } from "../../../lib/clock";
import { useT } from "../../../lib/i18n";
import { useDisplaySettings } from "../../../lib/locale";
import { useCanStartVoiceRoom, useVoiceAvailable } from "../../voice";
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
  type VoiceRoomSpeakers,
} from "../api/events";
import { fromZonedInput, toZonedInput } from "./eventTime";
import { checkCandidate } from "./useEventPoll";
import { useEventFailureMessage } from "./useEventFailureMessage";

/** A place to meet in person: its name, its address, and a link to a map. */
type PhysicalDraft = { enabled: boolean; name: string; address: string; url: string };
/** A link of another service, such as a stream or a meeting. */
type LinkDraft = { enabled: boolean; name: string; url: string };
/** The voice chat room that opens at the start, and who may speak in it without asking. */
type VoiceDraft = { enabled: boolean; speakers: VoiceRoomSpeakers };
/** A candidate date of a new event's poll, typed as the event's dates are; `key` keeps its row across removals. */
type CandidateDraft = { key: number; startsAt: string; endsAt: string };
/** Whether a new event has its date (`fixed`) or polls for it among candidates (`poll`). */
export type EventDateMode = "fixed" | "poll";

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
  voice: VoiceDraft;
  dateMode: EventDateMode;
  candidates: CandidateDraft[];
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
  | `candidate_${number}_starts_at`
  | `candidate_${number}_ends_at`
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
  const voice = event?.places.find((place) => place.kind === "voice");
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
    voice: { enabled: voice !== undefined, speakers: voice?.speakers ?? "manual" },
    dateMode: "fixed",
    candidates: [
      { key: 0, startsAt: "", endsAt: "" },
      { key: 1, startsAt: "", endsAt: "" },
    ],
  };
}

/**
 * The places of the draft, after the place of `event` that the form does not edit: its voice place while the form
 * does not offer one (`offersVoice`).
 */
function placesOf(draft: EventDraft, event: Event | undefined, offersVoice: boolean): EventPlace[] {
  const kept = offersVoice ? [] : (event?.places ?? []).filter((place) => place.kind === "voice");
  return [
    ...kept,
    ...(offersVoice && draft.voice.enabled ? [{ kind: "voice" as const, speakers: draft.voice.speakers }] : []),
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
  changeVoice: (change: Partial<VoiceDraft>) => void;
  /** Changes the candidate whose row is `key`; adds an empty one, up to the limit; removes one, keeping at least one. */
  changeCandidate: (key: number, change: Partial<Omit<CandidateDraft, "key">>) => void;
  addCandidate: () => void;
  removeCandidate: (key: number) => void;
  /** Whether the form offers the voice chat place: voice chat is on, and the user may open rooms or the event has one. */
  offersVoice: boolean;
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
  const userZone = timeZone ?? new Intl.DateTimeFormat().resolvedOptions().timeZone;
  const [stored, setStored] = useState<EventDraft>(() => draftOf(event, userZone));
  // A new event is in the user's time zone until the user picks one: the preferences may arrive after the form
  // mounts, so the zone is derived from them rather than copied once (react.dev, "You Might Not Need an Effect").
  const [zoneChosen, setZoneChosen] = useState(event !== undefined);
  const draft = zoneChosen ? stored : { ...stored, timeZone: userZone };
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
  const voiceAvailable = useVoiceAvailable();
  const canStartVoiceRoom = useCanStartVoiceRoom();
  const offersVoice =
    voiceAvailable && (canStartVoiceRoom || event?.places.some((place) => place.kind === "voice") === true);
  const polls = event === undefined && draft.dateMode === "poll";

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
    // An unscheduled event keeps polling unless both dates are typed (a date ends the poll); a new poll has no date.
    const dateless = polls || (unscheduled && draft.startsAt === "" && draft.endsAt === "");
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
    if (polls) {
      const seen = new Set<string>();
      for (const [index, candidate] of draft.candidates.entries()) {
        const checked = checkCandidate(candidate.startsAt, candidate.endsAt, draft.timeZone, now, t);
        if (checked.starts_at !== undefined) found[`candidate_${index}_starts_at`] = checked.starts_at;
        if (checked.ends_at !== undefined) found[`candidate_${index}_ends_at`] = checked.ends_at;
        const key = `${candidate.startsAt}/${candidate.endsAt}`;
        if (checked.starts_at === undefined && checked.ends_at === undefined && seen.has(key)) {
          found[`candidate_${index}_starts_at`] = t("client:events.form.candidate.duplicate");
        }
        seen.add(key);
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
      if (change.timeZone !== undefined) setZoneChosen(true);
      setStored((previous) => ({ ...previous, ...change }));
    },
    changePhysical: (change) => {
      setStored((previous) => ({ ...previous, physical: { ...previous.physical, ...change } }));
    },
    changeLink: (change) => {
      setStored((previous) => ({ ...previous, link: { ...previous.link, ...change } }));
    },
    changeVoice: (change) => {
      setStored((previous) => ({ ...previous, voice: { ...previous.voice, ...change } }));
    },
    changeCandidate: (key, change) => {
      setStored((previous) => ({
        ...previous,
        candidates: previous.candidates.map((candidate) =>
          candidate.key === key ? { ...candidate, ...change } : candidate,
        ),
      }));
    },
    addCandidate: () => {
      setStored((previous) =>
        previous.candidates.length >= eventLimits.candidates
          ? previous
          : {
              ...previous,
              candidates: [
                ...previous.candidates,
                { key: Math.max(-1, ...previous.candidates.map((c) => c.key)) + 1, startsAt: "", endsAt: "" },
              ],
            },
      );
    },
    removeCandidate: (key) => {
      setStored((previous) =>
        previous.candidates.length <= 1
          ? previous
          : { ...previous, candidates: previous.candidates.filter((candidate) => candidate.key !== key) },
      );
    },
    offersVoice,
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
        places: placesOf(draft, event, offersVoice),
      };
      const dates =
        checked.startsAt !== undefined && checked.endsAt !== undefined
          ? { starts_at: checked.startsAt.toISOString(), ends_at: checked.endsAt.toISOString() }
          : undefined;
      if (event === undefined) {
        const when = polls
          ? {
              candidates: draft.candidates.flatMap((candidate) => {
                const start = fromZonedInput(candidate.startsAt, draft.timeZone);
                const end = fromZonedInput(candidate.endsAt, draft.timeZone);
                return start === undefined || end === undefined
                  ? []
                  : [{ starts_at: start.toISOString(), ends_at: end.toISOString() }];
              }),
            }
          : dates;
        const body: EventCreate = { ...settings, visibility: draft.visibility, ...when };
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
