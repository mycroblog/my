// SPDX-License-Identifier: AGPL-3.0-only
// The events of the server's people (ADR-0161, FR-940–952): the upcoming events the reader sees (every one, or those
// an account organizes), one event as the reader sees it (the places they do not see left out), its attendees, its
// invitations (the organizers'), the candidate dates of its date poll and who answered each, and the reader's
// reminders, as queries; and the writes of the attendee and of the organizers, each of which puts the server's answer
// into every copy the cache holds (the event's query and the lists'), so that a list says what the event's page says.
// An answer (to the event, to a candidate, an approval, the reminders) shows as the reader chose it from the press on:
// the writes of one thing go one at a time, in the order made, and once none is on its way the cache holds what the
// server last said; after a refusal it is read again, since what made the server refuse (a full event, its start, a
// date confirmed) changed it. The `.ics` file needs the access token, so it is
// fetched and handed on as a file to save, never linked. The refusals the server names are told apart by kind.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery, type QueryKey } from "@tanstack/react-query";
import { viewerQuery } from "./accounts";
import { whileOpen, type AppData } from "./app-data";
import type { ServerEvent } from "./around";
import type { SavedFile } from "./exports";
import { instanceQuery } from "./instance";
import { pagedQuery, usePagedList, wholeList, type Page, type PagedList, type Pages } from "./paged";
import { useAppData } from "./provider";
import { answered, shownWrite } from "./shown-write";

type Schemas = components["schemas"];

/** An answer to an event: going, maybe, or not going (`declined`). */
export type EventAnswer = Schemas["EventAnswer"];
/** An attendee of an event, who answered going or maybe, and whether the organizers approved them. */
export type EventAttendee = Schemas["EventAttendee"];
/** The answer of one account to a candidate date. */
export type EventBallot = Schemas["EventBallot"];
/** A candidate date of a date poll, with how many answered it each way and the reader's own answer. */
export type EventCandidate = Schemas["EventCandidate"];
/** A new candidate date: its start and end, in UTC. */
export type EventCandidateCreate = Schemas["EventCandidateCreate"];
/** Why an event was cancelled. */
export type EventCancelReason = Schemas["EventCancelReason"];
/** The cancellation of an event: why, the note given, and when. */
export type EventCancellation = Schemas["EventCancellation"];
/** A new event, with a fixed date or the candidate dates of a date poll. */
export type EventCreate = Schemas["EventCreate"];
/** A change of an event: what it leaves out keeps its value, and `places` replaces every place. */
export type EventUpdate = Schemas["EventUpdate"];
/** An invitation of a local account to an event. */
export type EventInvitation = Schemas["EventInvitation"];
/** A place of an event: a voice chat room, a place to meet in person, or a URL. */
export type EventPlace = Schemas["EventPlace"];
/** The kind of a place. */
export type EventPlaceKind = Schemas["EventPlaceKind"];
/** Who sees the in-person and URL places: the approved attendees only, or everyone who sees the event. */
export type EventPlaceDisclosure = Schemas["EventPlaceDisclosure"];
/** The reader's reminders of the events they answered going or maybe. */
export type EventReminders = Schemas["EventReminders"];
/** Why the voice chat room of an event could not open at its start. */
export type EventRoomUnavailableReason = Schemas["EventRoomUnavailableReason"];
/** Whether an event polls its date, has one, or was cancelled; one that ended is `scheduled`, its end in the past. */
export type EventState = Schemas["EventState"];
/** What an event says of the reader: whether they organize, co-organize, are invited, answered, are approved. */
export type EventViewer = Schemas["EventViewer"];
/** Who sees an event. */
export type EventVisibility = Schemas["EventVisibility"];
/** An answer to a candidate date: yes, maybe, or no. */
export type EventVote = Schemas["EventVote"];

/** The visibilities of an event, from the widest. */
export const eventVisibilities = [
  "public",
  "followers",
  "following",
  "mutuals",
  "invited",
] as const satisfies readonly EventVisibility[];

/** Who sees the in-person and URL places: the server's default first. */
export const eventPlaceDisclosures = ["approved", "viewers"] as const satisfies readonly EventPlaceDisclosure[];

/** The kinds of a place; an event has at most one of each. */
export const eventPlaceKinds = ["voice", "physical", "url"] as const satisfies readonly EventPlaceKind[];

/** The answers to an event. */
export const eventAnswers = ["going", "maybe", "declined"] as const satisfies readonly EventAnswer[];

/** The answers to a candidate date. */
export const eventVotes = ["yes", "maybe", "no"] as const satisfies readonly EventVote[];

const minuteMs = 60_000;
const dayMs = 24 * 60 * minuteMs;

/**
 * The limits of an event as the server checks them (ADR-0161 §3–§8, §12): the title in characters as a reader sees
 * them (graphemes) and in code points, the other texts in code points, the length of an event, the candidates of a
 * poll, the co-organizers, the reminders and how long before the start each may be (in minutes), and the events and
 * invitations an hour the server takes.
 */
export const eventLimits = {
  title: 100,
  titleCodePoints: 1000,
  description: 3000,
  placeName: 100,
  address: 500,
  url: 2000,
  capacity: 10_000,
  places: 3,
  coorganizers: 3,
  candidates: 20,
  cancelNote: 500,
  durationMs: 7 * dayMs,
  reminders: 3,
  reminderMinutes: { min: 5, max: 7 * 24 * 60 },
  eventsAnHour: 10,
  invitationsAnHour: 100,
} as const;

/** The reminders the server sends until the reader chooses (one day and one hour before), the earliest first. */
export const defaultEventReminders: readonly number[] = [24 * 60, 60];

/** The latest start the server takes for a date set at `now`: a year ahead, on the same day of the calendar (UTC). */
export function latestEventStart(now: Date): Date {
  const latest = new Date(now.getTime());
  latest.setUTCFullYear(latest.getUTCFullYear() + 1);
  return latest;
}

/**
 * What the server refuses of a date set at `now` (ADR-0161 §5), by the member it names: `starts_at` for a start that is
 * not after now or more than a year ahead, `ends_at` for an end that is not after the start or more than 7 days after
 * it; null for a date it takes. The same holds for each candidate of a poll.
 */
export function eventScheduleFault(startsAt: Date, endsAt: Date, now: Date): "starts_at" | "ends_at" | null {
  const start = startsAt.getTime();
  const end = endsAt.getTime();
  if (!Number.isFinite(start) || start <= now.getTime() || start > latestEventStart(now).getTime()) return "starts_at";
  if (!Number.isFinite(end) || end <= start || end - start > eventLimits.durationMs) return "ends_at";
  return null;
}

/** Whether `url` is one the server takes for a place: an absolute `http` or `https` URL with a host, of 2,000 at most. */
export function isEventPlaceUrl(url: string): boolean {
  if (url === "" || url.length > eventLimits.url || /[\s\p{Cc}]/u.test(url)) return false;
  try {
    const parsed = new URL(url);
    return (
      (parsed.protocol === "http:" || parsed.protocol === "https:") &&
      parsed.host !== "" &&
      parsed.username === "" &&
      parsed.password === ""
    );
  } catch {
    return false;
  }
}

/**
 * Where an event is at `now`: its date polled (`polling`), cancelled, ended, started, or to come (`upcoming`). The
 * reader answers it only while it is to come, and its `.ics` exists once its date is fixed (not while it polls).
 */
export type EventPhase = "polling" | "upcoming" | "started" | "ended" | "cancelled";

/** Where `event` is at `now` (ms since the epoch). */
export function eventPhase(event: ServerEvent, now: number): EventPhase {
  if (event.state === "cancelled") return "cancelled";
  if (event.state === "unscheduled") return "polling";
  if (Date.parse(event.ends_at) <= now) return "ended";
  return Date.parse(event.starts_at) <= now ? "started" : "upcoming";
}

/** Whether the reader organizes or co-organizes `event`: they change it, invite, approve, and see every place. */
export function organizesEvent(event: ServerEvent): boolean {
  return event.viewer.organizer || event.viewer.coorganizer;
}

/**
 * The voice chat room of an event's voice place, as its page says it (ADR-0161 §10, FR-950, FR-952): no voice place,
 * or none to open for a cancelled event (`none`); before the start, that it opens then (`opens`); after it, the room it opened (`open`, which may have ended
 * since: the room's own read says), why it could not open (`unavailable`), or that it is opening (`opening`: the
 * server's tick has not come yet).
 */
export type EventRoom =
  | { readonly state: "none" }
  | { readonly state: "opens"; readonly at: string }
  | { readonly state: "opening" }
  | { readonly state: "open"; readonly roomId: string }
  | { readonly state: "unavailable"; readonly reason: EventRoomUnavailableReason };

/** The voice chat room of `event` at `now` (ms since the epoch). */
export function eventRoom(event: ServerEvent, now: number): EventRoom {
  const place = event.places.find((p) => p.kind === "voice");
  if (place === undefined) return { state: "none" };
  if (place.room_id !== undefined) return { state: "open", roomId: place.room_id };
  if (place.room_unavailable !== undefined) return { state: "unavailable", reason: place.room_unavailable };
  if (event.state === "cancelled") return { state: "none" };
  if (event.state === "unscheduled" || Date.parse(event.starts_at) > now)
    return { state: "opens", at: event.starts_at };
  return { state: "opening" };
}

// ---- Refusals ---------------------------------------------------------------------------------------------------------

/**
 * Why the server refused a write of an event, by the problem it answered (ADR-0161, the API's `event-*` problems):
 * - `not-found`: the reader does not see the event, or what the write names is gone;
 * - `full`: a new going to an event whose going reached its capacity;
 * - `closed`: the event was cancelled, or it started for an answer;
 * - `unscheduled`: an answer or the `.ics` of an event whose date poll was not confirmed yet;
 * - `scheduled`: a change of the poll of an event whose date was confirmed;
 * - `last-candidate`: the withdrawal of the last candidate of a poll;
 * - `not-eligible`: a reader who may not create events;
 * - `voice-unavailable`: a voice place while the voice chat is off or the organizer may not host rooms;
 * - `already-invited`: an account invited before (once each, a revoked one too);
 * - `invalid-invitee`, `invalid-coorganizer`: an account that is not a local active one, organizes the event, or is in
 *   a block relation with an organizer;
 * - `too-many-coorganizers`: a fourth co-organizer;
 * - `not-organizer`: a reader who neither organizes nor co-organizes it;
 * - `organizer-only`: what only the organizer does (the visibility, the co-organizers, the cancellation);
 * - `not-attending`: the approval of an account that did not answer going or maybe;
 * - `attendee-organizes`: the approval of the organizer or a co-organizer, who always see the places;
 * - `rate-limited`: more events or invitations an hour than the server takes (`eventRetryAfterMs` says how long);
 * - `invalid`: a value it does not take (`eventFaultField` names it).
 */
export type EventRefusal =
  | "not-found"
  | "full"
  | "closed"
  | "unscheduled"
  | "scheduled"
  | "last-candidate"
  | "not-eligible"
  | "voice-unavailable"
  | "already-invited"
  | "invalid-invitee"
  | "invalid-coorganizer"
  | "too-many-coorganizers"
  | "not-organizer"
  | "organizer-only"
  | "not-attending"
  | "attendee-organizes"
  | "rate-limited"
  | "invalid";

const refusals: Readonly<Record<string, EventRefusal>> = {
  "/api/v1/problems/event-full": "full",
  "/api/v1/problems/event-closed": "closed",
  "/api/v1/problems/event-unscheduled": "unscheduled",
  "/api/v1/problems/event-scheduled": "scheduled",
  "/api/v1/problems/event-last-candidate": "last-candidate",
  "/api/v1/problems/event-not-eligible": "not-eligible",
  "/api/v1/problems/event-voice-unavailable": "voice-unavailable",
  "/api/v1/problems/event-already-invited": "already-invited",
  "/api/v1/problems/event-invalid-invitee": "invalid-invitee",
  "/api/v1/problems/event-invalid-coorganizer": "invalid-coorganizer",
  "/api/v1/problems/event-too-many-coorganizers": "too-many-coorganizers",
  "/api/v1/problems/event-not-organizer": "not-organizer",
  "/api/v1/problems/event-organizer-only": "organizer-only",
  "/api/v1/problems/event-not-attending": "not-attending",
  "/api/v1/problems/event-attendee-organizes": "attendee-organizes",
};

/** What the server's refusal of a write of an event was; null for another failure (the network, a 5xx). */
export function eventRefusal(error: unknown): EventRefusal | null {
  if (!(error instanceof ProblemError)) return null;
  const named = refusals[error.type];
  if (named !== undefined) return named;
  if (error.status === 404) return "not-found";
  if (error.status === 429) return "rate-limited";
  return error.status === 422 ? "invalid" : null;
}

/**
 * The member of the request that the server's 422 is about: a setting (`title`, `description`, `visibility`,
 * `time_zone`, `place_disclosure`, `capacity`, `places`), the date (`starts_at`, `ends_at`, the time limits of
 * `eventScheduleFault`), the poll (`candidates`: more than 20, the same date twice, a date out of the limits), the
 * cancellation's `note`, an `answer` or a `vote`, or the reminders (`minutes_before`); null for no such member.
 */
export function eventFaultField(error: unknown): string | null {
  if (!(error instanceof ProblemError) || error.status !== 422) return null;
  return error.problem.field ?? null;
}

/** How long the server asks to wait before the next event or invitation (the `Retry-After` of its 429), in ms. */
export function eventRetryAfterMs(error: unknown): number | undefined {
  if (!(error instanceof ProblemError) || error.status !== 429) return undefined;
  const value = error.headers.get("Retry-After")?.trim() ?? "";
  if (/^\d+$/.test(value)) return Number(value) * 1000;
  const at = value === "" ? Number.NaN : Date.parse(value);
  return Number.isNaN(at) ? undefined : Math.max(at - Date.now(), 0);
}

// ---- Queries ----------------------------------------------------------------------------------------------------------

/** Which upcoming events a list holds: every one the reader sees, or those the account of `organizerId` organizes. */
export type EventListFilter = { readonly organizerId?: string | undefined };

/** The upcoming events the reader sees (of `filter`), the nearest start first, a page at a time. */
export function eventsQuery({ api, queries }: AppData, filter: EventListFilter = {}) {
  const query = filter.organizerId === undefined ? {} : { organizer_id: filter.organizerId };
  return pagedQuery<ServerEvent>(
    queries.queryOptions("get", "/events", { params: { query } }).queryKey,
    async (cursor, signal) =>
      (await api.GET("/events", { params: { query: cursor === undefined ? query : { ...query, cursor } }, signal }))
        .data,
  );
}

/** The event `id`, as the reader sees it. */
export function eventQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/events/{id}", { params: { path: { id } } });
}

/** The attendees of the event `id` who answer going or maybe, in the order of their answers, a page at a time. */
export function eventAttendeesQuery({ api, queries }: AppData, id: string) {
  return pagedQuery<EventAttendee>(
    queries.queryOptions("get", "/events/{id}/attendees", { params: { path: { id } } }).queryKey,
    async (cursor, signal) =>
      (
        await api.GET("/events/{id}/attendees", {
          params: { path: { id }, query: cursor === undefined ? {} : { cursor } },
          signal,
        })
      ).data,
  );
}

/** The invitations of the event `id` that were not revoked, oldest first, a page at a time (for its organizers). */
export function eventInvitationsQuery({ api, queries }: AppData, id: string) {
  return pagedQuery<EventInvitation>(
    queries.queryOptions("get", "/events/{id}/invitations", { params: { path: { id } } }).queryKey,
    async (cursor, signal) =>
      (
        await api.GET("/events/{id}/invitations", {
          params: { path: { id }, query: cursor === undefined ? {} : { cursor } },
          signal,
        })
      ).data,
  );
}

/** The candidate dates of the event `id` that were not withdrawn, the earliest start first (none for a fixed date). */
export function eventCandidatesQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/events/{id}/candidates", { params: { path: { id } } });
}

/**
 * Who answered the candidate `candidateId` of the event `id` and how (only `vote`, with one), in the order of the
 * accounts' IDs, a page at a time.
 */
export function eventBallotsQuery({ api, queries }: AppData, id: string, candidateId: string, vote?: EventVote) {
  const query = vote === undefined ? {} : { vote };
  return pagedQuery<EventBallot>(
    queries.queryOptions("get", "/events/{id}/candidates/{candidate_id}/votes", {
      params: { path: { id, candidate_id: candidateId }, query },
    }).queryKey,
    async (cursor, signal) =>
      (
        await api.GET("/events/{id}/candidates/{candidate_id}/votes", {
          params: {
            path: { id, candidate_id: candidateId },
            query: cursor === undefined ? query : { ...query, cursor },
          },
          signal,
        })
      ).data,
  );
}

/** The reader's reminders of events. */
export function eventRemindersQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/preferences/event_reminders");
}

/** The upcoming events the reader sees (of `filter`), for a list; nothing is read while `enabled` is false. */
export function useEvents(filter: EventListFilter = {}, enabled = true): PagedList<ServerEvent> {
  return usePagedList(eventsQuery(useAppData(), filter), enabled);
}

/** What a screen reads of one event. */
export type EventRead = {
  /** The event; undefined until the server answered, once it answered that the reader does not see it, and while not enabled. */
  readonly event: ServerEvent | undefined;
  /** The failure of the last read (a ProblemError or NetworkError of mykit), or null. */
  readonly error: Error | null;
  /** Whether the server answered 404: the reader does not see the event (or there is none), whatever the cache held. */
  readonly gone: boolean;
  /** Reads it again (the reader's Try again). */
  readonly refetch: () => void;
};

/** The event `id` as the reader sees it; nothing is read, and nothing given, while `enabled` is false. */
export function useEvent(id: string, enabled = true): EventRead {
  const query = useQuery({ ...eventQuery(useAppData(), id), enabled });
  const refetch = () => {
    void query.refetch();
  };
  if (!enabled) return { event: undefined, error: null, gone: false, refetch };
  const error = query.error as Error | null;
  const gone = error instanceof ProblemError && error.status === 404;
  return { event: gone ? undefined : query.data, error, gone, refetch };
}

/** The attendees of the event `id` who answer going or maybe; nothing is read while `enabled` is false. */
export function useEventAttendees(id: string, enabled = true): PagedList<EventAttendee> {
  return usePagedList(eventAttendeesQuery(useAppData(), id), enabled);
}

/** The invitations of the event `id`, for its organizers; nothing is read while `enabled` is false. */
export function useEventInvitations(id: string, enabled = true): PagedList<EventInvitation> {
  return usePagedList(eventInvitationsQuery(useAppData(), id), enabled);
}

const noCandidates: readonly EventCandidate[] = [];

/** The candidate dates of the event `id`, read whole; nothing is read, and none given, while `enabled` is false. */
export function useEventCandidates(id: string, enabled = true): PagedList<EventCandidate> {
  const query = useQuery({ ...eventCandidatesQuery(useAppData(), id), enabled });
  const list = wholeList({ ...query, data: query.data?.items });
  return enabled ? list : { ...list, items: noCandidates, status: "pending", error: null };
}

/** Who answered the candidate `candidateId` of the event `id` (only `vote`, with one); nothing read while not enabled. */
export function useEventBallots(
  id: string,
  candidateId: string,
  vote?: EventVote,
  enabled = true,
): PagedList<EventBallot> {
  return usePagedList(eventBallotsQuery(useAppData(), id, candidateId, vote), enabled);
}

/** What a screen reads of the reader's reminders. */
export type EventRemindersRead = {
  /** The reminders, as the reader chose them last; undefined until the server answered and while not enabled. */
  readonly reminders: EventReminders | undefined;
  readonly status: "pending" | "error" | "success";
  readonly error: Error | null;
  readonly refetch: () => void;
};

/** The reader's reminders of events; nothing is read, and nothing given, while `enabled` is false. */
export function useEventReminders(enabled = true): EventRemindersRead {
  const query = useQuery({ ...eventRemindersQuery(useAppData()), enabled });
  const refetch = () => {
    void query.refetch();
  };
  if (!enabled) return { reminders: undefined, status: "pending", error: null, refetch };
  return { reminders: query.data, status: query.status, error: query.error as Error | null, refetch };
}

// ---- The cache --------------------------------------------------------------------------------------------------------

/** Whether a query is a list of events: the screen's, a profile's, or the column beside the timeline's. */
const isEventList = (queryKey: QueryKey) => queryKey[0] === "get" && queryKey[1] === "/events";

/** `page` with the copy of `event` replaced, the same page when it holds none. */
function replacedIn<T extends { id: string }>(page: Page<T>, event: T): Page<T> {
  return page.items.some((e) => e.id === event.id)
    ? { ...page, items: page.items.map((e) => (e.id === event.id ? event : e)) }
    : page;
}

/** What a list of events holds, with `event` in place of its copy: a page (the column's), or the pages of a list. */
function listWith(data: unknown, event: ServerEvent): unknown {
  if (typeof data !== "object" || data === null) return data;
  if ("pages" in data) {
    const pages = data as Pages<ServerEvent>;
    return { ...pages, pages: pages.pages.map((p) => replacedIn(p, event)) };
  }
  return "items" in data ? replacedIn(data as Page<ServerEvent>, event) : data;
}

/** Shows `event` in every list of events that holds a copy of it. */
function showInLists({ queryClient }: AppData, event: ServerEvent): void {
  queryClient.setQueriesData({ predicate: (query) => isEventList(query.queryKey) }, (data: unknown) =>
    listWith(data, event),
  );
}

/** Shows `event` as its query and in every list that holds it. */
function showEvent(data: AppData, event: ServerEvent): void {
  data.queryClient.setQueryData(eventQuery(data, event.id).queryKey, event);
  showInLists(data, event);
}

/**
 * Puts `event`, as a write's answer or the stream gives it, into its query and into every list that holds a copy of
 * it, the reads of the event on their way cancelled first, since they would put back what the server held before.
 */
export async function receiveEvent(data: AppData, event: ServerEvent): Promise<void> {
  await data.queryClient.cancelQueries({ queryKey: eventQuery(data, event.id).queryKey });
  showEvent(data, event);
}

/** The lists of events may hold another set or order (an event made, its date or visibility changed): read again. */
function eventListsChanged({ queryClient }: AppData): Promise<void> {
  return queryClient.invalidateQueries({ predicate: (query) => isEventList(query.queryKey) });
}

// ---- The attendee's writes --------------------------------------------------------------------------------------------

/** `event` as it shows once the reader answered it `answer`: their answer, the going count, and an approval ended. */
function answeredAs(event: ServerEvent, answer: EventAnswer): ServerEvent {
  const was = event.viewer.answer;
  return {
    ...event,
    going_count: event.going_count + (answer === "going" ? 1 : 0) - (was === "going" ? 1 : 0),
    viewer: { ...event.viewer, answer, approved: answer === "declined" ? false : event.viewer.approved },
  };
}

/**
 * Answers the event `id` `answer`, which shows from the press on in the event and every list; the event as the server
 * answers it once the answers on their way ended, its attendees read again. It rejects with the server's refusal
 * (`full` at capacity, `closed` once it started or was cancelled, `unscheduled` while it polls its date), and the event
 * shows as the server holds it.
 */
export async function answerEvent(data: AppData, id: string, answer: EventAnswer): Promise<ServerEvent> {
  const key = eventQuery(data, id).queryKey;
  await data.queryClient.cancelQueries({ queryKey: key });
  const event = await shownWrite<ServerEvent, ServerEvent>(data, `event-rsvp:${id}`, {
    keys: [key],
    read: () => data.queryClient.getQueryData<ServerEvent>(key),
    show: () => {
      const now = data.queryClient.getQueryData<ServerEvent>(key);
      if (now !== undefined) showEvent(data, answeredAs(now, answer));
    },
    write: async () =>
      answered(await data.api.PUT("/events/{id}/rsvp", { params: { path: { id } }, body: { answer } }), "the answer"),
    server: (answer) => answer,
    put: (server) => {
      showEvent(data, server);
    },
    // Refused: the event filled up, started, was cancelled or is polled; the lists show it as it is now too.
    refresh: () => {
      void data.queryClient.invalidateQueries({ queryKey: key });
      void eventListsChanged(data);
    },
  });
  void data.queryClient.invalidateQueries({ queryKey: eventAttendeesQuery(data, id).queryKey });
  return event;
}

/** `candidate` as it shows once the reader answered it `vote`: their answer, and the counts. */
function votedAs(candidate: EventCandidate, vote: EventVote): EventCandidate {
  const count = (v: EventVote) => (vote === v ? 1 : 0) - (candidate.vote === v ? 1 : 0);
  return {
    ...candidate,
    vote,
    yes_count: candidate.yes_count + count("yes"),
    maybe_count: candidate.maybe_count + count("maybe"),
    no_count: candidate.no_count + count("no"),
  };
}

type CandidateList = Schemas["EventCandidateList"];

/** The candidates of the event `id` with `candidate` in place of its copy. */
function showCandidate(data: AppData, id: string, candidate: EventCandidate): void {
  data.queryClient.setQueryData<CandidateList>(eventCandidatesQuery(data, id).queryKey, (now) =>
    now === undefined ? now : { ...now, items: now.items.map((c) => (c.id === candidate.id ? candidate : c)) },
  );
}

/** Whether a query is of who answered a candidate of the event `id` (any answer, any candidate when none is named). */
const isBallotsOf = (queryKey: QueryKey, id: string, candidateId?: string) => {
  if (queryKey[1] !== "/events/{id}/candidates/{candidate_id}/votes") return false;
  const path = (queryKey[2] as { params?: { path?: { id?: string; candidate_id?: string } } } | undefined)?.params
    ?.path;
  return path?.id === id && (candidateId === undefined || path.candidate_id === candidateId);
};

/**
 * Answers the candidate `candidateId` of the event `id` `vote`, which shows from the press on with its counts; the
 * candidate as the server answers it once the answers on their way ended, who answered it read again. It rejects with
 * the server's refusal (`scheduled` once a date was confirmed, `closed` for a cancelled event).
 */
export async function voteEventCandidate(
  data: AppData,
  id: string,
  candidateId: string,
  vote: EventVote,
): Promise<EventCandidate> {
  const key = eventCandidatesQuery(data, id).queryKey;
  await data.queryClient.cancelQueries({ queryKey: key });
  const of = () => data.queryClient.getQueryData<CandidateList>(key)?.items.find((c) => c.id === candidateId);
  const candidate = await shownWrite<EventCandidate, EventCandidate>(data, `event-vote:${id}:${candidateId}`, {
    keys: [key],
    read: of,
    show: () => {
      const now = of();
      if (now !== undefined) showCandidate(data, id, votedAs(now, vote));
    },
    write: async () =>
      answered(
        await data.api.PUT("/events/{id}/candidates/{candidate_id}/vote", {
          params: { path: { id, candidate_id: candidateId } },
          body: { vote },
        }),
        "the vote",
      ),
    server: (answer) => answer,
    put: (server) => {
      showCandidate(data, id, server);
    },
    // Refused: the date was confirmed or the event cancelled, which the event says, its poll gone or closed.
    refresh: () => {
      void data.queryClient.invalidateQueries({ queryKey: key });
      void data.queryClient.invalidateQueries({ queryKey: eventQuery(data, id).queryKey });
    },
  });
  void data.queryClient.invalidateQueries({ predicate: (query) => isBallotsOf(query.queryKey, id, candidateId) });
  return candidate;
}

/**
 * The reader's reminders as they show once the change is made: the minutes chosen, the earliest first, or the
 * defaults.
 */
function remindersAs(change: EventRemindersChange): EventReminders {
  return "defaults" in change
    ? { minutes_before: [...defaultEventReminders], defaults: true }
    : { minutes_before: change.minutesBefore.toSorted((a, b) => b - a), defaults: false };
}

/** A change of the reader's reminders: the minutes before the start of each (none for no reminders), or the defaults. */
export type EventRemindersChange = { readonly minutesBefore: readonly number[] } | { readonly defaults: true };

/**
 * Changes the reader's reminders, which show as chosen from now on; the changes go one at a time, and once none is on
 * its way the reminders are what the server last said. It rejects with the server's 422 for more than 3, one out of 5
 * minutes to 7 days, or the same twice (`minutes_before`), and nothing changes then.
 */
export async function changeEventReminders(data: AppData, change: EventRemindersChange): Promise<EventReminders> {
  const key = eventRemindersQuery(data).queryKey;
  await data.queryClient.cancelQueries({ queryKey: key });
  return shownWrite<EventReminders, EventReminders>(data, "event-reminders", {
    keys: [key],
    read: () => data.queryClient.getQueryData<EventReminders>(key),
    show: () => {
      data.queryClient.setQueryData(key, remindersAs(change));
    },
    write: async () =>
      answered(
        await data.api.PUT("/preferences/event_reminders", {
          body:
            "defaults" in change ? { defaults: true } : { minutes_before: [...change.minutesBefore], defaults: false },
        }),
        "the change of the reminders",
      ),
    server: (answer) => answer,
    put: (server) => {
      data.queryClient.setQueryData(key, server);
    },
    refresh: () => {
      void data.queryClient.invalidateQueries({ queryKey: key });
    },
  });
}

/** The name in a Content-Disposition header, else `fallback`. */
function fileName(disposition: string | null, fallback: string): string {
  const name = /filename="?([^";]+)"?/.exec(disposition ?? "")?.[1];
  return name === undefined || name === "" ? fallback : name;
}

/**
 * The event `id` as an iCalendar file to save (`<id>.ics`), with only the places the reader sees; it rejects with the
 * server's refusal (`unscheduled` while its date is polled, `not-found`). Once the session of `data` ended, it never
 * settles: no file is saved for a session that is over (`whileOpen`).
 */
export function downloadEventCalendar(data: AppData, id: string): Promise<SavedFile> {
  return whileOpen(data, async () => {
    const { data: blob, response } = await data.api.GET("/events/{id}/calendar.ics", {
      params: { path: { id } },
      parseAs: "blob",
    });
    return {
      blob: blob ?? new Blob([], { type: "text/calendar" }),
      name: fileName(response.headers.get("Content-Disposition"), `${id}.ics`),
    };
  });
}

// ---- The organizers' writes -------------------------------------------------------------------------------------------

/**
 * Creates an event the reader organizes, which the cache holds at once; the lists are read again. It rejects with the
 * server's refusal (`not-eligible`, `voice-unavailable`, `rate-limited`, `invalid` with its field).
 */
export async function createEvent(data: AppData, event: EventCreate): Promise<ServerEvent> {
  const made = answered(await data.api.POST("/events", { body: event }), "the new event");
  data.queryClient.setQueryData(eventQuery(data, made.id).queryKey, made);
  void eventListsChanged(data);
  return made;
}

/**
 * Changes the event `id` (its organizer or a co-organizer; the visibility its organizer only), which every copy shows
 * at once; the lists, and the poll a new date ends, are read again.
 */
export async function updateEvent(data: AppData, id: string, update: EventUpdate): Promise<ServerEvent> {
  const event = answered(
    await data.api.PATCH("/events/{id}", { params: { path: { id } }, body: update }),
    "the change of the event",
  );
  await receiveEvent(data, event);
  void eventListsChanged(data);
  if (update.starts_at !== undefined)
    void data.queryClient.invalidateQueries({ queryKey: eventCandidatesQuery(data, id).queryKey });
  return event;
}

/** Cancels the event `id` (its organizer), with `note` as the reason when it says one; every copy shows it at once. */
export async function cancelEvent(data: AppData, id: string, note?: string): Promise<ServerEvent> {
  const event = answered(
    await data.api.POST("/events/{id}/cancel", {
      params: { path: { id } },
      body: note === undefined || note.trim() === "" ? {} : { note },
    }),
    "the cancellation",
  );
  await receiveEvent(data, event);
  return event;
}

/** The attendees of the event `id` with `change` made to the attendee of `accountId`. */
function changeAttendee(data: AppData, id: string, accountId: string, change: (a: EventAttendee) => EventAttendee) {
  data.queryClient.setQueryData<Pages<EventAttendee>>(eventAttendeesQuery(data, id).queryKey, (now) =>
    now === undefined
      ? now
      : {
          ...now,
          pages: now.pages.map((p) => ({
            ...p,
            items: p.items.map((a) => (a.account.id === accountId ? change(a) : a)),
          })),
        },
  );
}

/**
 * Approves the answer of the attendee `accountId` of the event `id` (`approved`), or ends the approval, which shows
 * from the press on; the attendee as the server answers it once the writes on their way ended. It rejects with the
 * server's refusal (`not-attending` for an account that did not answer going or maybe, `attendee-organizes` for the
 * organizer or a co-organizer, `not-found` for one that did not answer, `closed`), and the attendee shows as the server
 * holds it.
 */
export async function setEventApproval(
  data: AppData,
  id: string,
  accountId: string,
  approved: boolean,
): Promise<EventAttendee> {
  const key = eventAttendeesQuery(data, id).queryKey;
  await data.queryClient.cancelQueries({ queryKey: key });
  const of = () =>
    data.queryClient
      .getQueryData<Pages<EventAttendee>>(key)
      ?.pages.flatMap((p) => p.items)
      .find((a) => a.account.id === accountId);
  return shownWrite<EventAttendee, EventAttendee>(data, `event-approval:${id}:${accountId}`, {
    keys: [key],
    read: of,
    show: () => {
      changeAttendee(data, id, accountId, (a) => ({ ...a, approved }));
    },
    write: async () => {
      const params = { params: { path: { id, account_id: accountId } } };
      return answered(
        approved
          ? await data.api.PUT("/events/{id}/attendees/{account_id}/approval", params)
          : await data.api.DELETE("/events/{id}/attendees/{account_id}/approval", params),
        "the approval",
      );
    },
    server: (answer) => answer,
    put: (server) => {
      changeAttendee(data, id, accountId, () => server);
    },
    refresh: () => {
      void data.queryClient.invalidateQueries({ queryKey: key });
    },
  });
}

/**
 * Invites the local account `accountId` to the event `id`; the invitation joins the end of the list the cache holds
 * when it holds its last page, and the list is read again otherwise. It rejects with the server's refusal
 * (`already-invited`, `invalid-invitee`, `rate-limited`).
 */
export async function inviteToEvent(data: AppData, id: string, accountId: string): Promise<EventInvitation> {
  const invitation = answered(
    await data.api.POST("/events/{id}/invitations", { params: { path: { id } }, body: { account_id: accountId } }),
    "the invitation",
  );
  const key = eventInvitationsQuery(data, id).queryKey;
  await data.queryClient.cancelQueries({ queryKey: key });
  const now = data.queryClient.getQueryData<Pages<EventInvitation>>(key);
  const last = now?.pages.at(-1);
  if (now === undefined || last === undefined || (last.next_cursor ?? "") !== "")
    void data.queryClient.invalidateQueries({ queryKey: key });
  else
    data.queryClient.setQueryData<Pages<EventInvitation>>(key, {
      ...now,
      pages: [...now.pages.slice(0, -1), { ...last, items: [...last.items, invitation] }],
    });
  return invitation;
}

/**
 * Revokes the invitation of `accountId` to the event `id`, which leaves the list the cache holds; one the server no
 * longer holds (a 404) is revoked too, and the list is read again.
 */
export async function revokeEventInvitation(data: AppData, id: string, accountId: string): Promise<void> {
  const key = eventInvitationsQuery(data, id).queryKey;
  let gone = false;
  try {
    await data.api.DELETE("/events/{id}/invitations/{account_id}", { params: { path: { id, account_id: accountId } } });
  } catch (error) {
    if (!(error instanceof ProblemError && error.status === 404)) throw error;
    gone = true;
  }
  await data.queryClient.cancelQueries({ queryKey: key });
  data.queryClient.setQueryData<Pages<EventInvitation>>(key, (now) =>
    now === undefined
      ? now
      : { ...now, pages: now.pages.map((p) => ({ ...p, items: p.items.filter((i) => i.account.id !== accountId) })) },
  );
  if (gone) void data.queryClient.invalidateQueries({ queryKey: key });
}

/**
 * Names the local account `accountId` a co-organizer of the event `id` (`assigned`), or removes them, by its
 * organizer; the event as the server answers it shows in every copy. It rejects with the server's refusal
 * (`too-many-coorganizers`, `invalid-coorganizer`, `organizer-only`).
 */
export async function setEventCoorganizer(
  data: AppData,
  id: string,
  accountId: string,
  assigned: boolean,
): Promise<ServerEvent> {
  const params = { params: { path: { id, account_id: accountId } } };
  const event = answered(
    assigned
      ? await data.api.PUT("/events/{id}/coorganizers/{account_id}", params)
      : await data.api.DELETE("/events/{id}/coorganizers/{account_id}", params),
    "the co-organizer",
  );
  await receiveEvent(data, event);
  return event;
}

/** The poll of the event `id` changed its dates: the event, whose start and end span its candidates, is read again. */
function pollChanged(data: AppData, id: string): void {
  void data.queryClient.invalidateQueries({ queryKey: eventQuery(data, id).queryKey });
  void eventListsChanged(data);
}

/**
 * Adds a candidate date to the poll of the event `id`, which joins the candidates the cache holds in the order of their
 * starts; the event is read again. It rejects with the server's refusal (`invalid` of `candidates`, `starts_at` or
 * `ends_at`: a 21st, the same date twice, one out of the limits; `scheduled` once a date was confirmed).
 */
export async function addEventCandidate(
  data: AppData,
  id: string,
  candidate: EventCandidateCreate,
): Promise<EventCandidate> {
  const added = answered(
    await data.api.POST("/events/{id}/candidates", { params: { path: { id } }, body: candidate }),
    "the new candidate",
  );
  const key = eventCandidatesQuery(data, id).queryKey;
  await data.queryClient.cancelQueries({ queryKey: key });
  if (data.queryClient.getQueryData(key) === undefined) void data.queryClient.invalidateQueries({ queryKey: key });
  else
    data.queryClient.setQueryData<CandidateList>(key, (now) =>
      now === undefined
        ? now
        : {
            ...now,
            items: [...now.items.filter((c) => c.id !== added.id), added].toSorted(
              (a, b) => Date.parse(a.starts_at) - Date.parse(b.starts_at),
            ),
          },
    );
  pollChanged(data, id);
  return added;
}

/**
 * Withdraws the candidate `candidateId` of the poll of the event `id`, which leaves the candidates the cache holds; one
 * the server no longer holds (a 404) is withdrawn too, and the candidates are read again. It rejects with the server's
 * refusal (`last-candidate`, `scheduled`).
 */
export async function withdrawEventCandidate(data: AppData, id: string, candidateId: string): Promise<void> {
  const key = eventCandidatesQuery(data, id).queryKey;
  let gone = false;
  try {
    await data.api.DELETE("/events/{id}/candidates/{candidate_id}", {
      params: { path: { id, candidate_id: candidateId } },
    });
  } catch (error) {
    if (!(error instanceof ProblemError && error.status === 404)) throw error;
    gone = true;
  }
  await data.queryClient.cancelQueries({ queryKey: key });
  data.queryClient.setQueryData<CandidateList>(key, (now) =>
    now === undefined ? now : { ...now, items: now.items.filter((c) => c.id !== candidateId) },
  );
  data.queryClient.removeQueries({ predicate: (query) => isBallotsOf(query.queryKey, id, candidateId) });
  if (gone) void data.queryClient.invalidateQueries({ queryKey: key });
  pollChanged(data, id);
}

/**
 * Fixes the date of the event `id` to the candidate `candidateId`, which ends its poll: the event as the server
 * answers it shows in every copy, and the candidates and the lists are read again. It rejects with the server's
 * refusal (`scheduled` for a date confirmed already, `invalid` for a candidate that started).
 */
export async function confirmEventCandidate(data: AppData, id: string, candidateId: string): Promise<ServerEvent> {
  const event = answered(
    await data.api.POST("/events/{id}/candidates/{candidate_id}/confirm", {
      params: { path: { id, candidate_id: candidateId } },
    }),
    "the confirmation",
  );
  await receiveEvent(data, event);
  void data.queryClient.invalidateQueries({ queryKey: eventCandidatesQuery(data, id).queryKey });
  void eventListsChanged(data);
  return event;
}

// ---- From a screen ----------------------------------------------------------------------------------------------------

/** The writes of the event `id`, from its screens, each as the function of the same name says. */
export type EventWrites = {
  readonly answer: (answer: EventAnswer) => Promise<ServerEvent>;
  readonly vote: (candidateId: string, vote: EventVote) => Promise<EventCandidate>;
  readonly calendar: () => Promise<SavedFile>;
  readonly update: (update: EventUpdate) => Promise<ServerEvent>;
  readonly cancel: (note?: string) => Promise<ServerEvent>;
  readonly approve: (accountId: string, approved: boolean) => Promise<EventAttendee>;
  readonly invite: (accountId: string) => Promise<EventInvitation>;
  readonly revokeInvitation: (accountId: string) => Promise<void>;
  readonly coorganizer: (accountId: string, assigned: boolean) => Promise<ServerEvent>;
  readonly addCandidate: (candidate: EventCandidateCreate) => Promise<EventCandidate>;
  readonly withdrawCandidate: (candidateId: string) => Promise<void>;
  readonly confirmCandidate: (candidateId: string) => Promise<ServerEvent>;
  /**
   * Reads the event, its dates and its attendees again: after a refusal that says they changed under the reader (a
   * date confirmed, the event cancelled, an attendee who answered otherwise).
   */
  readonly reread: () => void;
};

/** The writes of the event `id`, from a screen. */
export function useEventWrites(id: string): EventWrites {
  const data = useAppData();
  return {
    answer: (answer) => answerEvent(data, id, answer),
    vote: (candidateId, vote) => voteEventCandidate(data, id, candidateId, vote),
    calendar: () => downloadEventCalendar(data, id),
    update: (update) => updateEvent(data, id, update),
    cancel: (note) => cancelEvent(data, id, note),
    approve: (accountId, approved) => setEventApproval(data, id, accountId, approved),
    invite: (accountId) => inviteToEvent(data, id, accountId),
    revokeInvitation: (accountId) => revokeEventInvitation(data, id, accountId),
    coorganizer: (accountId, assigned) => setEventCoorganizer(data, id, accountId, assigned),
    addCandidate: (candidate) => addEventCandidate(data, id, candidate),
    withdrawCandidate: (candidateId) => withdrawEventCandidate(data, id, candidateId),
    confirmCandidate: (candidateId) => confirmEventCandidate(data, id, candidateId),
    reread: () => {
      for (const { queryKey } of [
        eventQuery(data, id),
        eventCandidatesQuery(data, id),
        { queryKey: eventAttendeesQuery(data, id).queryKey },
      ])
        void data.queryClient.invalidateQueries({ queryKey });
    },
  };
}

/**
 * Whether the reader may create events (FR-940, ADR-0161 §3): the server lets every local account create one
 * (`event_creators` is `everyone`) or the reader's roles have `create_events`, and no moderator limited the reader's
 * account (a limit makes every post of theirs followers-only, which their posting limits say). The server checks the
 * rest as the event is created.
 */
export type MayCreateEvents = {
  /** Whether the reader may; undefined until the server said what it is from (the instance and the reader's account). */
  readonly may: boolean | undefined;
  /** What it is from could not be read: `retry` reads it again. */
  readonly failed: boolean;
  readonly retry: () => void;
};

/** Whether the reader may create events, for a screen (`MayCreateEvents`). */
export function useMayCreateEvents(): MayCreateEvents {
  const data = useAppData();
  const instance = useQuery(instanceQuery(data));
  const viewer = useQuery(viewerQuery(data));
  const retry = () => {
    if (instance.data === undefined) void instance.refetch();
    if (viewer.data === undefined) void viewer.refetch();
  };
  if (instance.data === undefined || viewer.data === undefined)
    return {
      may: undefined,
      failed: (instance.data === undefined && instance.isError) || (viewer.data === undefined && viewer.isError),
      retry,
    };
  const may =
    viewer.data.posting.visibility_ceiling === "public" &&
    (instance.data.event_creators === "everyone" || viewer.data.permissions.includes("create_events"));
  return { may, failed: false, retry };
}

/** What the form that creates an event waits for: the instance and the reader's account, which say whether they may. */
export async function loadMayCreateEvents(data: AppData): Promise<void> {
  await Promise.all([
    data.queryClient.query({ ...instanceQuery(data), staleTime: Infinity }).catch(() => undefined),
    data.queryClient.query({ ...viewerQuery(data), staleTime: Infinity }).catch(() => undefined),
  ]);
}

/** Creates an event from a screen (`createEvent`). */
export function useCreateEvent(): (event: EventCreate) => Promise<ServerEvent> {
  const data = useAppData();
  return (event) => createEvent(data, event);
}

/** Changes the reader's reminders from a screen (`changeEventReminders`). */
export function useChangeEventReminders(): (change: EventRemindersChange) => Promise<EventReminders> {
  const data = useAppData();
  return (change) => changeEventReminders(data, change);
}
