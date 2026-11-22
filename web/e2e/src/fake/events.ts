// SPDX-License-Identifier: AGPL-3.0-only

// The fake's events (ADR-0161): the upcoming events the reader sees, one event as they see it (the places they do not
// see left out, only their kinds named), its attendees, invitations and co-organizers, its date poll with the answers
// to each candidate, the cancellation, the `.ics` file, and the reader's reminders; each as the server decides it, by
// one check of who sees an event (§9) and its places, so that a hidden place never reaches the client. The events are
// the seed's plans (the scenario's, and the fake's own beside them, which name the reader's part in each: organizer,
// co-organizer, invited, an attendee approved or not), held in memory with what the reader writes. A voice place's room
// is the one its organizer hosts among the fake's open rooms, for an event that started with its room open. Each event's
// board (§17) is read by its organizers and those going or maybe, written on by those its `board_writers` admit until it
// is cancelled or 7 days after its end; its posts are statuses the fake makes (api.ts), which no timeline holds.

import type { ServerEventPlan } from "../scenario";
import { eventStart } from "../scenario";
import { day, idAt, minute, type PageQuery } from "./common";
import type { Handlers, Refusal, Schema } from "./spec";

type Account = Schema<"Account">;
type Event = Schema<"Event">;
type Place = Schema<"EventPlace">;
type Answer = Schema<"EventAnswer">;
type Vote = Schema<"EventVote">;

/** A day and an hour in the event's time zone, `inDays` days after the seed runs, for `hours` hours. */
type Slot = { readonly inDays: number; readonly hour: number; readonly hours: number };

/**
 * An event of a seed: the scenario's plan, and what the fake holds beyond it, each by the people's usernames. Without
 * more it is a public event of a fixed date with no place, no capacity and no attendee. `candidates` polls its date in
 * place of the plan's; `startsInMinutes` starts it that many minutes after the seed runs (one going on, one that
 * ended); `room` is what came of its voice place's room at the start: open (the room its organizer hosts), or why it
 * could not open.
 */
export type FakeEventPlan = ServerEventPlan & {
  readonly coorganizers?: readonly string[];
  readonly visibility?: Schema<"EventVisibility">;
  readonly placeDisclosure?: Schema<"EventPlaceDisclosure">;
  readonly capacity?: number;
  readonly places?: readonly Place[];
  readonly candidates?: readonly Slot[];
  readonly votes?: readonly { readonly by: string; readonly candidate: number; readonly vote: Vote }[];
  readonly attendees?: readonly { readonly by: string; readonly answer: Answer; readonly approved?: boolean }[];
  readonly invited?: readonly string[];
  readonly cancelled?: { readonly reason: Schema<"EventCancelReason">; readonly note?: string };
  readonly startsInMinutes?: number;
  readonly room?: "open" | Schema<"EventRoomUnavailableReason">;
  /** Who writes on its board; the server's default (`organizers`) without one. */
  readonly boardWriters?: Schema<"EventBoardWriters">;
  /** The posts on its board, oldest first, each by a person of the seed that many minutes before the seed runs. */
  readonly board?: readonly { readonly by: string; readonly text: string; readonly minutesAgo: number }[];
};

const trailhead: Place = {
  kind: "physical",
  name: "North ridge trailhead",
  address: "Behind the old station, by the stone steps",
  url: "https://maps.example/north-ridge",
};
const bakery: Place = { kind: "physical", name: "Sam's kitchen", address: "2-4-1 Harbor Street, flat 3" };
const stream: Place = { kind: "url", name: "The class on video", url: "https://meet.example/bread-class" };

/**
 * The fake's own events, beside the scenario's, as the reader (yuki) finds them: a poll the reader organizes; one they
 * co-organize; one they are invited to; one they go to approved, and one they may go to unapproved (its place hidden);
 * one at its capacity; a voice place before the start, with its room open, and with a room that could not open; one
 * cancelled, one that ended; and two the reader does not see (for the organizer's followers, and of an account the
 * reader blocks). Boards with posts: the class the reader co-organizes (they write), the swap they go to (only the
 * organizers write), and the cancelled one (read-only); everyone who reads it writes on the cooking night's.
 */
export const fakeEventPlans: readonly FakeEventPlan[] = [
  {
    key: "photo-walk",
    organizer: "yuki",
    coorganizers: ["hana"],
    title: "Photo walk: pick a day",
    description:
      "Up the north ridge for the morning light. Answer the days that work for you.\nhttps://photos.example/ridge",
    timeZone: "Asia/Tokyo",
    inDays: 10,
    hour: 7,
    hours: 4,
    candidates: [
      { inDays: 10, hour: 7, hours: 4 },
      { inDays: 11, hour: 7, hours: 4 },
      { inDays: 17, hour: 8, hours: 4 },
    ],
    votes: [
      { by: "sam", candidate: 0, vote: "yes" },
      { by: "sam", candidate: 1, vote: "maybe" },
      { by: "hana", candidate: 0, vote: "yes" },
      { by: "layla", candidate: 1, vote: "no" },
      { by: "layla", candidate: 2, vote: "yes" },
    ],
    places: [trailhead],
  },
  {
    key: "bread-class",
    organizer: "sam",
    coorganizers: ["yuki"],
    title: "Sourdough class",
    description: "Bring a jar of starter if you have one. We bake two loaves each.",
    timeZone: "Asia/Tokyo",
    inDays: 4,
    hour: 14,
    hours: 3,
    visibility: "followers",
    capacity: 12,
    places: [bakery, stream],
    attendees: [
      { by: "hana", answer: "going", approved: true },
      { by: "layla", answer: "going" },
      { by: "marco", answer: "maybe" },
    ],
    invited: ["mio"],
    board: [
      { by: "sam", text: "The oven fits four loaves at once, so we bake in two rounds.", minutesAgo: 26 * 60 },
      { by: "hana", text: "I can bring extra flour and a scale.", minutesAgo: 3 * 60 },
    ],
  },
  {
    key: "garden-day",
    organizer: "layla",
    title: "Community garden day",
    description: "Weeding, planting and lunch under the plum tree.",
    timeZone: "Asia/Tokyo",
    inDays: 5,
    hour: 10,
    hours: 5,
    visibility: "invited",
    placeDisclosure: "viewers",
    places: [{ kind: "physical", name: "Riverside garden", address: "Next to the footbridge" }],
    invited: ["yuki", "sam"],
    attendees: [{ by: "sam", answer: "going" }],
  },
  {
    key: "book-swap",
    organizer: "mio",
    title: "Book swap",
    description: "Bring a book, take a book.",
    timeZone: "Europe/Paris",
    inDays: 7,
    hour: 18,
    hours: 2,
    places: [
      { kind: "physical", name: "Café Lumière", address: "12 rue des Lilas" },
      { kind: "url", name: "The list of books", url: "https://books.example/swap" },
    ],
    attendees: [
      { by: "yuki", answer: "going", approved: true },
      { by: "ren", answer: "maybe" },
    ],
    board: [
      { by: "mio", text: "The café opens its back room for us from 6. Come in through the garden.", minutesAgo: 90 },
    ],
  },
  {
    key: "cooking-night",
    organizer: "hana",
    title: "Cooking night",
    description: "Dumplings from scratch.",
    timeZone: "Asia/Tokyo",
    inDays: 8,
    hour: 19,
    hours: 3,
    places: [{ kind: "physical", name: "Hana's place", address: "5-1 Hill Road" }],
    attendees: [
      { by: "yuki", answer: "maybe" },
      { by: "sam", answer: "going", approved: true },
    ],
    boardWriters: "readers",
  },
  {
    key: "tiny-concert",
    organizer: "marco",
    title: "A tiny concert",
    description: "Three seats, one guitar.",
    timeZone: "Europe/Lisbon",
    inDays: 3,
    hour: 21,
    hours: 2,
    capacity: 3,
    placeDisclosure: "viewers",
    places: [{ kind: "url", name: "The live stream", url: "https://live.example/tiny" }],
    attendees: [
      { by: "sam", answer: "going" },
      { by: "hana", answer: "going" },
      { by: "layla", answer: "going" },
    ],
  },
  {
    key: "radio-hour",
    organizer: "sam",
    title: "Radio hour",
    description: "Records and requests, live in a voice room.",
    timeZone: "Asia/Tokyo",
    inDays: 2,
    hour: 20,
    hours: 1,
    visibility: "mutuals",
    places: [{ kind: "voice", speakers: "manual" }],
  },
  {
    key: "coffee-voice",
    organizer: "sam",
    title: "Coffee chat, the event",
    description: "The morning coffee chat, planned.",
    timeZone: "Asia/Tokyo",
    inDays: 0,
    hour: 0,
    hours: 2,
    startsInMinutes: -30,
    places: [{ kind: "voice", speakers: "manual" }],
    room: "open",
    attendees: [{ by: "yuki", answer: "going" }],
  },
  {
    key: "trail-talk",
    organizer: "hana",
    title: "Trail talk",
    description: "Stories from the summer's hikes.",
    timeZone: "Asia/Tokyo",
    inDays: 0,
    hour: 0,
    hours: 2,
    startsInMinutes: -60,
    places: [{ kind: "voice", speakers: "everyone" }],
    room: "too_many_rooms",
    attendees: [{ by: "yuki", answer: "going" }],
  },
  {
    key: "flea-market",
    organizer: "layla",
    title: "Flea market",
    description: "Cancelled for the rain.",
    timeZone: "Asia/Tokyo",
    inDays: 2,
    hour: 9,
    hours: 6,
    cancelled: { reason: "organizer", note: "Rain all day. We try again next month." },
    attendees: [{ by: "yuki", answer: "going" }],
    board: [{ by: "layla", text: "Keep your things for next month: same place, same time.", minutesAgo: 50 }],
  },
  {
    key: "picnic",
    organizer: "sam",
    title: "Picnic by the river",
    description: "Thank you all for coming.",
    timeZone: "Asia/Tokyo",
    inDays: 0,
    hour: 0,
    hours: 3,
    startsInMinutes: -2 * 24 * 60,
    places: [{ kind: "physical", name: "The riverbank", address: "Below the footbridge" }],
    attendees: [{ by: "yuki", answer: "going", approved: true }],
  },
  {
    key: "followers-brunch",
    organizer: "marco",
    title: "Brunch for my followers",
    description: "Pancakes.",
    timeZone: "Asia/Tokyo",
    inDays: 6,
    hour: 11,
    hours: 2,
    visibility: "followers",
  },
  {
    key: "flash-sale",
    organizer: "quickdeals",
    title: "Flash sale meetup",
    description: "Everything must go.",
    timeZone: "UTC",
    inDays: 1,
    hour: 12,
    hours: 1,
  },
];

/** What a test does with the fake's events. */
export type FakeEvents = {
  /** The ID of the event of the seed's plan of `key`, or of the reader's that was created as its title; "none" for none. */
  readonly id: (key: string) => string;
  /** The event of `key` as the reader sees it now; undefined for one they do not see, or none. */
  readonly shown: (key: string) => Event | undefined;
};

/** What the event handlers take from the fake, by the people's usernames. */
export type EventContext = {
  readonly plans: readonly FakeEventPlan[];
  /** When the seed ran, in ms since the epoch. */
  readonly started: number;
  /** The reader's username. */
  readonly viewer: string;
  /** The account of the person of `username`; undefined for no person of the seed. */
  readonly accountOf: (username: string) => Account | undefined;
  /** The username of the account of `id`; undefined for none. */
  readonly usernameOf: (id: string) => string | undefined;
  /** Whether `username` is a local account that is active (no person of another server, none deactivated). */
  readonly active: (username: string) => boolean;
  readonly follows: (follower: string, followee: string) => boolean;
  /** Whether a block separates the two, either way. */
  readonly blocks: (a: string, b: string) => boolean;
  /** The ID of the open voice room `username` hosts; undefined for none. */
  readonly roomHostedBy: (username: string) => string | undefined;
  /** The voice chat is off. */
  readonly voiceOff: boolean;
  /** Whether the reader may host voice rooms (and so hold an event in one). */
  readonly mayHostRooms: () => boolean;
  /** Whether the reader may create events (`event_creators`, the `create_events` permission). */
  readonly mayCreate: () => boolean;
  /** The lists of events list nothing (what is around the reader is `none`). */
  readonly unlisted: boolean;
  /** The origin of the server, for the `.ics` file's URL and UID. */
  readonly origin: string;
  readonly problem: (status: number, title: string, more?: Partial<Schema<"Problem">>) => Refusal;
  readonly notFound: () => Refusal;
  readonly page: <T>(items: readonly T[], query: PageQuery, key: (item: T) => string) => { body: { items: T[] } };
  /**
   * Makes a post of `by` on the board of the event `eventId` at `at` (ms), which the statuses' operations read, edit,
   * delete and mark as any status the fake made; with `request`, once per its `Idempotency-Key`.
   */
  readonly boardPost: (
    eventId: string,
    by: string,
    body: Schema<"EventBoardPostCreate">,
    at: number,
    request?: Request,
  ) => Schema<"Status">;
  /** The posts on the board of the event `eventId` that are there now, newest first. */
  readonly boardPosts: (eventId: string) => readonly Schema<"Status">[];
};

/** An event as the fake holds it, by usernames. */
type Held = {
  readonly key: string;
  readonly id: string;
  readonly organizer: string;
  coorganizers: string[];
  title: string;
  description: string;
  visibility: Schema<"EventVisibility">;
  timeZone: string;
  placeDisclosure: Schema<"EventPlaceDisclosure">;
  capacity: number;
  boardWriters: Schema<"EventBoardWriters">;
  places: Place[];
  /** The fixed date, in ms; undefined while the date is polled. */
  schedule: { startsAt: number; endsAt: number } | undefined;
  readonly candidates: { id: string; startsAt: number; endsAt: number; withdrawn: boolean }[];
  /** The answers to each candidate, by its ID, then by username. */
  readonly votes: Map<string, Map<string, Vote>>;
  confirmed: string | undefined;
  /** The answers to the event, in the order they were last given. */
  rsvps: { by: string; answer: Answer; approved: boolean }[];
  readonly invitations: { id: string; by: string; invitedBy: string; at: number; revoked: boolean }[];
  cancellation: Schema<"EventCancellation"> | undefined;
  readonly createdAt: number;
  revisedAt: number;
  revisions: number;
};

/** How many events a reader creates an hour, and invitations an event sends. */
const createLimit = 10;
const inviteLimit = 100;

/** The most minutes before the start a reminder may be: 7 days. */
const reminderMax = 7 * 24 * 60;

/** How many code points `text` has, as the server counts a length. */
const codePoints = (text: string) => Array.from(text).length;

const graphemes = (text: string) => [...new Intl.Segmenter("en", { granularity: "grapheme" }).segment(text)].length;

/** Whether `zone` is an IANA time zone the platform knows. */
function knownZone(zone: string): boolean {
  if (zone === "" || zone === "Local") return false;
  try {
    new Intl.DateTimeFormat("en", { timeZone: zone });
    return true;
  } catch {
    return false;
  }
}

/** Whether `url` is a URL a place takes: http or https, with a host, of 2,000 at most. */
function placeUrl(url: string | undefined): boolean {
  if (url === undefined || url === "" || url.length > 2000 || /\s/.test(url)) return false;
  try {
    const parsed = new URL(url);
    return (
      (parsed.protocol === "http:" || parsed.protocol === "https:") && parsed.host !== "" && parsed.username === ""
    );
  } catch {
    return false;
  }
}

/** A line of a setting: not empty when required, no surrounding white space or line break, `max` at most. */
const line = (text: string | undefined, max: number, required: boolean) =>
  text === undefined || text === ""
    ? !required
    : text.trim() === text && !/[\n\r]/.test(text) && codePoints(text) <= max;

/** An iCalendar text value, escaped (RFC 5545 §3.3.11). */
const icsText = (text: string) =>
  text.replaceAll("\\", "\\\\").replaceAll(";", "\\;").replaceAll(",", "\\,").replaceAll("\n", "\\n");

/** An instant as iCalendar's UTC date-time. */
const icsTime = (ms: number) =>
  new Date(ms)
    .toISOString()
    .replace(/[-:]/g, "")
    .replace(/\.\d{3}/, "");

/** The handlers of the events, and what a test does with them. */
export function eventHandlers(c: EventContext): { handlers: Handlers; events: FakeEvents } {
  let ids = 0;
  const newId = () => idAt(Date.now(), 42_000 + (ids += 1));
  const iso = (ms: number) => new Date(ms).toISOString();
  const people = (names: readonly string[] | undefined) => (names ?? []).filter((u) => c.accountOf(u) !== undefined);

  const held: Held[] = [];
  c.plans.forEach((plan, i) => {
    if (c.accountOf(plan.organizer) === undefined) return;
    const at = (slot: Slot) => eventStart({ ...plan, ...slot }, c.started).getTime();
    const candidates = (plan.candidates ?? []).map((slot, j) => {
      const startsAt = at(slot);
      return {
        id: idAt(startsAt, 41_000 + i * 20 + j),
        startsAt,
        endsAt: startsAt + slot.hours * 60 * minute,
        withdrawn: false,
      };
    });
    const startsAt = plan.startsInMinutes === undefined ? at(plan) : c.started + plan.startsInMinutes * minute;
    const schedule = candidates.length > 0 ? undefined : { startsAt, endsAt: startsAt + plan.hours * 60 * minute };
    const votes = new Map(candidates.map((cand) => [cand.id, new Map<string, Vote>()]));
    for (const v of plan.votes ?? []) {
      const cand = candidates[v.candidate];
      if (cand !== undefined && c.accountOf(v.by) !== undefined) votes.get(cand.id)?.set(v.by, v.vote);
    }
    const room = plan.room === "open" ? c.roomHostedBy(plan.organizer) : undefined;
    const places = (plan.places ?? []).map((p): Place => {
      if (p.kind !== "voice" || plan.room === undefined) return { ...p };
      if (plan.room === "open") return room === undefined ? { ...p } : { ...p, room_id: room };
      return { ...p, room_unavailable: plan.room };
    });
    const created = c.started - day;
    const id = idAt(schedule?.startsAt ?? candidates[0]?.startsAt ?? startsAt, 40_000 + i);
    for (const post of plan.board ?? [])
      if (c.accountOf(post.by) !== undefined)
        c.boardPost(id, post.by, { text: post.text }, c.started - post.minutesAgo * minute);
    held.push({
      key: plan.key,
      id,
      organizer: plan.organizer,
      coorganizers: people(plan.coorganizers),
      title: plan.title,
      description: plan.description,
      visibility: plan.visibility ?? "public",
      timeZone: plan.timeZone,
      placeDisclosure: plan.placeDisclosure ?? "approved",
      capacity: plan.capacity ?? 0,
      boardWriters: plan.boardWriters ?? "organizers",
      places,
      schedule,
      candidates,
      votes,
      confirmed: undefined,
      rsvps: (plan.attendees ?? [])
        .filter((a) => c.accountOf(a.by) !== undefined)
        .map((a) => ({ by: a.by, answer: a.answer, approved: a.approved === true && a.answer !== "declined" })),
      invitations: people(plan.invited).map((u, j) => ({
        id: idAt(created + j * minute, 43_000 + i * 20 + j),
        by: u,
        invitedBy: plan.organizer,
        at: created + j * minute,
        revoked: false,
      })),
      cancellation:
        plan.cancelled === undefined
          ? undefined
          : {
              reason: plan.cancelled.reason,
              ...(plan.cancelled.note === undefined ? {} : { note: plan.cancelled.note }),
              canceled_at: iso(c.started - 60 * minute),
            },
      createdAt: created,
      revisedAt: created,
      revisions: 1,
    });
  });

  /** The reader's reminders, in minutes before the start, the earliest first; undefined for the defaults. */
  let reminders: number[] | undefined;
  /** When the reader created each event, and when each event sent each invitation, for the limits an hour. */
  const creations: number[] = [];
  const invitationsSent = new Map<string, number[]>();

  const eventOf = (id: string) => held.find((e) => e.id === id);
  const viewer = c.viewer;
  const organizes = (e: Held, u = viewer) => e.organizer === u || e.coorganizers.includes(u);
  const invited = (e: Held, u = viewer) => e.invitations.some((i) => i.by === u && !i.revoked);
  const rsvpOf = (e: Held, u = viewer) => e.rsvps.find((r) => r.by === u);
  const voted = (e: Held, u = viewer) => [...e.votes.values()].some((byUser) => byUser.has(u));
  /** Who sees `e` (ADR-0161 §9 CanSee), in the server's order of the checks. */
  const sees = (e: Held, u = viewer): boolean => {
    if (!c.active(u)) return false;
    if ([e.organizer, ...e.coorganizers].some((o) => o !== u && c.blocks(o, u))) return false;
    if (organizes(e, u) || invited(e, u) || rsvpOf(e, u) !== undefined || voted(e, u)) return true;
    switch (e.visibility) {
      case "public":
        return true;
      case "followers":
        return c.follows(u, e.organizer);
      case "following":
        return c.follows(e.organizer, u);
      case "mutuals":
        return c.follows(u, e.organizer) && c.follows(e.organizer, u);
      case "invited":
        return false;
    }
  };
  /** Whether the reader sees the place `p` of `e`, which they see (§9 CanSeePlace). */
  const seesPlace = (e: Held, p: Place) =>
    organizes(e) || p.kind === "voice" || e.placeDisclosure === "viewers" || rsvpOf(e)?.approved === true;
  const live = (e: Held) => e.candidates.filter((cand) => !cand.withdrawn);
  const startOf = (e: Held) => e.schedule?.startsAt ?? Math.min(...live(e).map((cand) => cand.startsAt));
  const endOf = (e: Held) => e.schedule?.endsAt ?? Math.max(...live(e).map((cand) => cand.endsAt));
  const goingCount = (e: Held) => e.rsvps.filter((r) => r.answer === "going").length;
  const accountsOf = (names: readonly string[]) => names.flatMap((u) => c.accountOf(u) ?? []);
  const accountIn = (u: string): Account => {
    const account = c.accountOf(u);
    if (account === undefined) throw new Error(`fake: no account of ${u}`);
    return account;
  };

  /** Whether the reader reads the board of `e` (§17): they organize it, or answer going or maybe. */
  const readsBoard = (e: Held) => organizes(e) || rsvpOf(e)?.answer === "going" || rsvpOf(e)?.answer === "maybe";
  /** Whether the board of `e` is read-only: the event was cancelled, or 7 days passed since its end. */
  const boardClosed = (e: Held) => e.cancellation !== undefined || Date.now() > endOf(e) + 7 * day;
  /** Whether the board's writers admit the reader, whatever its period says. */
  const admitted = (e: Held) =>
    e.boardWriters === "readers" ||
    e.organizer === viewer ||
    (e.boardWriters === "organizers" && e.coorganizers.includes(viewer));
  const writesBoard = (e: Held) => readsBoard(e) && admitted(e) && !boardClosed(e);

  /** The event as the reader sees it. */
  const shown = (e: Held): Event => {
    const rsvp = rsvpOf(e);
    return {
      id: e.id,
      organizer: accountIn(e.organizer),
      coorganizers: accountsOf(e.coorganizers),
      title: e.title,
      description: e.description,
      visibility: e.visibility,
      time_zone: e.timeZone,
      place_disclosure: e.placeDisclosure,
      capacity: e.capacity,
      board_writers: e.boardWriters,
      places: e.places.filter((p) => seesPlace(e, p)).map((p) => ({ ...p })),
      hidden_places: e.places.filter((p) => !seesPlace(e, p)).map((p) => p.kind),
      starts_at: iso(startOf(e)),
      ends_at: iso(endOf(e)),
      state: e.cancellation !== undefined ? "canceled" : e.schedule === undefined ? "unscheduled" : "scheduled",
      ...(e.cancellation === undefined ? {} : { cancellation: e.cancellation }),
      going_count: goingCount(e),
      created_at: iso(e.createdAt),
      revised_at: iso(e.revisedAt),
      revision_count: e.revisions,
      viewer: {
        organizer: e.organizer === viewer,
        coorganizer: e.coorganizers.includes(viewer),
        invited: invited(e),
        ...(rsvp === undefined ? {} : { answer: rsvp.answer }),
        approved: organizes(e) || rsvp?.approved === true,
        board: { readable: readsBoard(e), writable: writesBoard(e) },
      },
    };
  };
  const ok = (e: Held) => ({ status: 200 as const, body: shown(e) });

  const refused = (status: number, type: string, title: string) =>
    c.problem(status, title, { type: `/api/v1/problems/${type}` });
  const invalid = (field: string) => c.problem(422, "This value is not valid", { field });
  const closed = () => refused(409, "event-closed", "The event is closed");
  const notOrganizer = () => refused(403, "event-not-organizer", "Only the organizers can do this");
  const organizerOnly = () => refused(403, "event-organizer-only", "Only the organizer can do this");
  const scheduledAlready = () => refused(409, "event-scheduled", "The date of the event is confirmed");
  const notReader = () =>
    refused(
      403,
      "event-board-not-reader",
      "Only the organizer, the co-organizers, and the people who answered going or maybe can read this event's board. Set your answer to going or maybe to read it.",
    );
  /** The server's 429 of a limit an hour whose oldest request counted was `since`, with how long to wait. */
  const rateLimited = (since: number): Refusal => ({
    ...c.problem(429, "Too many requests"),
    headers: { "Retry-After": String(Math.max(1, Math.ceil((since + 60 * minute - Date.now()) / 1000))) },
  });

  /** The event of `id` the reader sees, or the 404 the server answers for one they do not see. */
  const seen = (id: string): Held | Refusal => {
    const e = eventOf(id);
    return e === undefined || !sees(e) ? c.notFound() : e;
  };
  /** The event of `id` the reader organizes or co-organizes and that was not cancelled, or the refusal. */
  const managed = (id: string, organizerAlone = false): Held | Refusal => {
    const e = seen(id);
    if ("problem" in e) return e;
    if (!organizes(e)) return notOrganizer();
    if (organizerAlone && e.organizer !== viewer) return organizerOnly();
    return e.cancellation === undefined ? e : closed();
  };

  /** What the server refuses of a date set now, by the member it names. */
  const scheduleFault = (startsAt: string | undefined, endsAt: string | undefined, prefix = ""): Refusal | null => {
    const start = Date.parse(startsAt ?? "");
    const end = Date.parse(endsAt ?? "");
    const now = Date.now();
    const latest = new Date(now);
    latest.setUTCFullYear(latest.getUTCFullYear() + 1);
    if (!Number.isFinite(start) || start <= now || start > latest.getTime()) return invalid(`${prefix}starts_at`);
    if (!Number.isFinite(end) || end <= start || end - start > 7 * day) return invalid(`${prefix}ends_at`);
    return null;
  };
  /** Whether the organizer of an event may hold it in a voice room: every organizer but a reader who may not host. */
  const voiceAllowed = (organizer: string) => !c.voiceOff && (organizer !== viewer || c.mayHostRooms());
  /** What the server refuses of an event's settings (§5, §6), as Settings.Validate does, in its order. */
  const settingsFault = (s: {
    title: string;
    description: string;
    visibility: string;
    timeZone: string;
    placeDisclosure: string;
    capacity: number;
    places: readonly Place[];
  }): Refusal | null => {
    if (!line(s.title, 1000, true) || graphemes(s.title) > 100) return invalid("title");
    if (codePoints(s.description) > 3000) return invalid("description");
    if (!["public", "followers", "following", "mutuals", "invited"].includes(s.visibility))
      return invalid("visibility");
    if (!knownZone(s.timeZone)) return invalid("time_zone");
    if (!["approved", "viewers"].includes(s.placeDisclosure)) return invalid("place_disclosure");
    if (!Number.isInteger(s.capacity) || s.capacity < 0 || s.capacity > 10_000) return invalid("capacity");
    const kinds = s.places.map((p) => p.kind);
    const valid = (p: Place) => {
      switch (p.kind) {
        case "voice":
          return p.name === undefined && p.address === undefined && p.url === undefined;
        case "physical":
          return (
            p.speakers === undefined &&
            line(p.name, 100, true) &&
            codePoints(p.address ?? "") <= 500 &&
            (p.url === undefined || p.url === "" || placeUrl(p.url))
          );
        case "url":
          return p.speakers === undefined && line(p.name, 100, true) && p.address === undefined && placeUrl(p.url);
      }
    };
    if (s.places.length > 3 || new Set(kinds).size !== kinds.length || !s.places.every(valid)) return invalid("places");
    return null;
  };
  /** A place as a request gives it, without what only the server says of it. */
  const placeIn = (p: Place): Place => {
    const copy = { ...p };
    delete copy.room_id;
    delete copy.room_unavailable;
    if (copy.kind === "voice") copy.speakers ??= "manual";
    return copy;
  };
  const candidateShown = (e: Held, cand: Held["candidates"][number]): Schema<"EventCandidate"> => {
    const byUser = e.votes.get(cand.id) ?? new Map<string, Vote>();
    const count = (v: Vote) => [...byUser.values()].filter((x) => x === v).length;
    const own = byUser.get(viewer);
    return {
      id: cand.id,
      starts_at: iso(cand.startsAt),
      ends_at: iso(cand.endsAt),
      yes_count: count("yes"),
      maybe_count: count("maybe"),
      no_count: count("no"),
      ...(own === undefined ? {} : { vote: own }),
      confirmed: e.confirmed === cand.id,
    };
  };
  /** An attendee of `e`: its organizer and co-organizers always see the places, approved whatever was recorded (§6). */
  const attendeeShown = (e: Held, r: Held["rsvps"][number]): Schema<"EventAttendee"> => ({
    account: accountIn(r.by),
    answer: r.answer,
    approved: organizes(e, r.by) || r.approved,
    organizes: organizes(e, r.by),
  });
  const attendeeOrganizes = () =>
    refused(
      422,
      "event-attendee-organizes",
      "The organizer and the co-organizers always see the places, so their answers are not approved.",
    );
  const invitationShown = (i: Held["invitations"][number]): Schema<"EventInvitation"> => ({
    id: i.id,
    account: accountIn(i.by),
    invited_by_id: accountIn(i.invitedBy).id,
    invited_at: iso(i.at),
  });
  /** What the reader's reminders are. */
  const remindersShown = (): Schema<"EventReminders"> =>
    reminders === undefined
      ? { minutes_before: [1440, 60], defaults: true }
      : { minutes_before: reminders, defaults: false };
  /** The account the reader names by `id`, when it is a local active one, with its username. */
  const named = (id: string) => {
    const u = c.usernameOf(id);
    return u !== undefined && c.active(u) ? u : undefined;
  };
  /** Whether a block separates `u` from the organizer or a co-organizer of `e`. */
  const separated = (e: Held, u: string) => [e.organizer, ...e.coorganizers].some((o) => o !== u && c.blocks(o, u));

  /** The calendar file of `e`, with only the places the reader sees (§15). */
  const calendar = (e: Held, schedule: { startsAt: number; endsAt: number }) => {
    const where = e.places
      .filter((p) => seesPlace(e, p))
      .map((p) =>
        p.kind === "voice" ? `${c.origin}/events/${e.id}` : [p.name, p.address, p.url].filter(Boolean).join(" "),
      );
    return [
      "BEGIN:VCALENDAR",
      "VERSION:2.0",
      "PRODID:-//mycroblog//my//EN",
      "METHOD:PUBLISH",
      "BEGIN:VEVENT",
      `UID:${e.id}@${new URL(c.origin).host}`,
      `DTSTAMP:${icsTime(e.revisedAt)}`,
      `DTSTART:${icsTime(schedule.startsAt)}`,
      `DTEND:${icsTime(schedule.endsAt)}`,
      `SEQUENCE:${String(e.revisions)}`,
      `SUMMARY:${icsText(e.title)}`,
      ...(e.description === "" ? [] : [`DESCRIPTION:${icsText(e.description)}`]),
      ...(where.length === 0 ? [] : [`LOCATION:${icsText(where.join(" / "))}`]),
      `URL:${c.origin}/events/${e.id}`,
      ...(e.cancellation === undefined ? [] : ["STATUS:CANCELLED"]),
      "END:VEVENT",
      "END:VCALENDAR",
      "",
    ].join("\r\n");
  };

  const handlers: Handlers = {
    "GET /events": ({ query }) => {
      const organizer = query.get("organizer_id");
      const now = Date.now();
      const items = c.unlisted
        ? []
        : held
            .filter((e) => sees(e) && endOf(e) > now)
            .filter((e) => organizer === null || c.usernameOf(organizer) === e.organizer)
            .toSorted((a, b) => startOf(a) - startOf(b) || a.id.localeCompare(b.id))
            .map(shown);
      return { status: 200, body: c.page(items, query, (e) => e.id).body };
    },
    "POST /events": ({ body }) => {
      if (!c.active(viewer) || !c.mayCreate()) return refused(403, "event-not-eligible", "You may not create events");
      const settings = {
        title: body.title,
        description: body.description ?? "",
        visibility: body.visibility ?? "public",
        timeZone: body.time_zone ?? "UTC",
        placeDisclosure: body.place_disclosure ?? "approved",
        capacity: body.capacity ?? 0,
        places: (body.places ?? []).map(placeIn),
      };
      const fault = settingsFault(settings);
      if (fault !== null) return fault;
      const boardWriters = body.board_writers ?? "organizers";
      if (!["organizer", "organizers", "readers"].includes(boardWriters)) return invalid("board_writers");
      const fixed = body.starts_at !== undefined || body.ends_at !== undefined;
      const polled = body.candidates !== undefined;
      if (fixed === polled) return invalid("candidates");
      if (fixed) {
        const dateFault = scheduleFault(body.starts_at, body.ends_at);
        if (dateFault !== null) return dateFault;
      } else {
        const candidates = body.candidates ?? [];
        const dates = candidates.map((cand) => `${cand.starts_at}/${cand.ends_at}`);
        if (candidates.length === 0 || candidates.length > 20 || new Set(dates).size !== dates.length)
          return invalid("candidates");
        for (const cand of candidates)
          if (scheduleFault(cand.starts_at, cand.ends_at) !== null) return invalid("candidates");
      }
      if (settings.places.some((p) => p.kind === "voice") && !voiceAllowed(viewer))
        return refused(422, "event-voice-unavailable", "The event cannot be held in a voice chat room");
      const now = Date.now();
      const recent = creations.filter((at) => at > now - 60 * minute);
      if (recent.length >= createLimit) return rateLimited(recent[0] ?? now);
      creations.push(now);
      const id = newId();
      const e: Held = {
        key: body.title,
        id,
        organizer: viewer,
        coorganizers: [],
        ...settings,
        boardWriters,
        schedule: fixed
          ? { startsAt: Date.parse(body.starts_at ?? ""), endsAt: Date.parse(body.ends_at ?? "") }
          : undefined,
        candidates: (body.candidates ?? []).map((cand) => ({
          id: newId(),
          startsAt: Date.parse(cand.starts_at),
          endsAt: Date.parse(cand.ends_at),
          withdrawn: false,
        })),
        votes: new Map(),
        confirmed: undefined,
        rsvps: [],
        invitations: [],
        cancellation: undefined,
        createdAt: now,
        revisedAt: now,
        revisions: 1,
      };
      for (const cand of e.candidates) e.votes.set(cand.id, new Map());
      held.push(e);
      return { status: 201, body: shown(e) };
    },
    "GET /events/{id}": ({ params }) => {
      const e = seen(params.id);
      return "problem" in e ? e : ok(e);
    },
    "PATCH /events/{id}": ({ params, body }) => {
      const e = managed(params.id);
      if ("problem" in e) return e;
      if (body.visibility !== undefined && body.visibility !== e.visibility && e.organizer !== viewer)
        return organizerOnly();
      if (body.board_writers !== undefined && body.board_writers !== e.boardWriters && e.organizer !== viewer)
        return organizerOnly();
      if (body.board_writers !== undefined && !["organizer", "organizers", "readers"].includes(body.board_writers))
        return invalid("board_writers");
      const settings = {
        title: body.title ?? e.title,
        description: body.description ?? e.description,
        visibility: body.visibility ?? e.visibility,
        timeZone: body.time_zone ?? e.timeZone,
        placeDisclosure: body.place_disclosure ?? e.placeDisclosure,
        capacity: body.capacity ?? e.capacity,
        places: body.places === undefined ? e.places : body.places.map(placeIn),
      };
      const fault = settingsFault(settings);
      if (fault !== null) return fault;
      const dated = body.starts_at !== undefined || body.ends_at !== undefined;
      if (dated) {
        const dateFault = scheduleFault(body.starts_at, body.ends_at);
        if (dateFault !== null) return dateFault;
      }
      const addsVoice = settings.places.some((p) => p.kind === "voice") && !e.places.some((p) => p.kind === "voice");
      if (addsVoice && !voiceAllowed(e.organizer))
        return refused(422, "event-voice-unavailable", "The event cannot be held in a voice chat room");
      // A voice place kept keeps what came of its room.
      const room = e.places.find((p) => p.kind === "voice");
      Object.assign(e, settings);
      e.places = settings.places.map((p) =>
        p.kind === "voice" && room !== undefined
          ? {
              ...p,
              ...(room.room_id === undefined ? {} : { room_id: room.room_id }),
              ...(room.room_unavailable === undefined ? {} : { room_unavailable: room.room_unavailable }),
            }
          : p,
      );
      if (dated) e.schedule = { startsAt: Date.parse(body.starts_at ?? ""), endsAt: Date.parse(body.ends_at ?? "") };
      if (body.board_writers !== undefined) e.boardWriters = body.board_writers;
      e.revisions += 1;
      e.revisedAt = Date.now();
      return ok(e);
    },
    "POST /events/{id}/cancel": ({ params, body }) => {
      const e = seen(params.id);
      if ("problem" in e) return e;
      if (!organizes(e)) return notOrganizer();
      if (e.organizer !== viewer) return organizerOnly();
      if (e.cancellation !== undefined) return closed();
      const note = body?.note;
      if (note !== undefined && codePoints(note) > 500) return invalid("note");
      e.cancellation = {
        reason: "organizer",
        ...(note === undefined || note === "" ? {} : { note }),
        canceled_at: iso(Date.now()),
      };
      return ok(e);
    },
    "PUT /events/{id}/rsvp": ({ params, body }) => {
      const e = seen(params.id);
      if ("problem" in e) return e;
      if (!["going", "maybe", "declined"].includes(body.answer)) return invalid("answer");
      if (e.cancellation !== undefined) return closed();
      if (e.schedule === undefined)
        return refused(409, "event-unscheduled", "The date of the event is not confirmed yet");
      if (e.schedule.startsAt <= Date.now()) return closed();
      const before = rsvpOf(e);
      if (body.answer === "going" && before?.answer !== "going" && e.capacity > 0 && goingCount(e) >= e.capacity)
        return refused(409, "event-full", "The event is full");
      e.rsvps = [
        ...e.rsvps.filter((r) => r.by !== viewer),
        { by: viewer, answer: body.answer, approved: body.answer !== "declined" && before?.approved === true },
      ];
      return ok(e);
    },
    "GET /events/{id}/attendees": ({ params, query }) => {
      const e = seen(params.id);
      if ("problem" in e) return e;
      const items = e.rsvps.filter((r) => r.answer !== "declined").map((r) => attendeeShown(e, r));
      return { status: 200, body: c.page(items, query, (a) => a.account.id).body };
    },
    "PUT /events/{id}/attendees/{account_id}/approval": ({ params }) => {
      const e = managed(params.id);
      if ("problem" in e) return e;
      const u = c.usernameOf(params.account_id);
      if (u !== undefined && organizes(e, u)) return attendeeOrganizes();
      const r = u === undefined ? undefined : rsvpOf(e, u);
      if (r === undefined || r.answer === "declined")
        return refused(422, "event-not-attending", "This account does not attend the event");
      r.approved = true;
      return { status: 200, body: attendeeShown(e, r) };
    },
    "DELETE /events/{id}/attendees/{account_id}/approval": ({ params }) => {
      const e = managed(params.id);
      if ("problem" in e) return e;
      const u = c.usernameOf(params.account_id);
      if (u !== undefined && organizes(e, u)) return attendeeOrganizes();
      const r = u === undefined ? undefined : rsvpOf(e, u);
      if (r === undefined) return c.notFound();
      r.approved = false;
      return { status: 200, body: attendeeShown(e, r) };
    },
    "GET /events/{id}/calendar.ics": ({ params }) => {
      const e = seen(params.id);
      if ("problem" in e) return e;
      if (e.schedule === undefined)
        return refused(409, "event-unscheduled", "The date of the event is not confirmed yet");
      return {
        status: 200,
        type: "text/calendar",
        raw: calendar(e, e.schedule),
        headers: { "Content-Disposition": `attachment; filename="${e.id}.ics"` },
      };
    },
    "GET /events/{id}/candidates": ({ params }) => {
      const e = seen(params.id);
      if ("problem" in e) return e;
      const items = live(e)
        .toSorted((a, b) => a.startsAt - b.startsAt)
        .map((cand) => candidateShown(e, cand));
      return { status: 200, body: { items } };
    },
    "POST /events/{id}/candidates": ({ params, body }) => {
      const e = managed(params.id);
      if ("problem" in e) return e;
      if (e.schedule !== undefined) return scheduledAlready();
      const fault = scheduleFault(body.starts_at, body.ends_at);
      if (fault !== null) return fault;
      const [startsAt, endsAt] = [Date.parse(body.starts_at), Date.parse(body.ends_at)];
      if (live(e).length >= 20 || live(e).some((cand) => cand.startsAt === startsAt && cand.endsAt === endsAt))
        return invalid("candidates");
      const cand = { id: newId(), startsAt, endsAt, withdrawn: false };
      e.candidates.push(cand);
      e.votes.set(cand.id, new Map());
      return { status: 201, body: candidateShown(e, cand) };
    },
    "DELETE /events/{id}/candidates/{candidate_id}": ({ params }) => {
      const e = managed(params.id);
      if ("problem" in e) return e;
      if (e.schedule !== undefined) return scheduledAlready();
      const cand = live(e).find((x) => x.id === params.candidate_id);
      if (cand === undefined) return c.notFound();
      if (live(e).length === 1) return refused(409, "event-last-candidate", "The poll keeps its last candidate");
      cand.withdrawn = true;
      return { status: 204 };
    },
    "POST /events/{id}/candidates/{candidate_id}/confirm": ({ params }) => {
      const e = managed(params.id);
      if ("problem" in e) return e;
      if (e.schedule !== undefined) return scheduledAlready();
      const cand = live(e).find((x) => x.id === params.candidate_id);
      if (cand === undefined) return c.notFound();
      if (cand.startsAt <= Date.now()) return invalid("starts_at");
      e.schedule = { startsAt: cand.startsAt, endsAt: cand.endsAt };
      e.confirmed = cand.id;
      return ok(e);
    },
    "PUT /events/{id}/candidates/{candidate_id}/vote": ({ params, body }) => {
      const e = seen(params.id);
      if ("problem" in e) return e;
      if (e.cancellation !== undefined) return closed();
      if (e.schedule !== undefined) return scheduledAlready();
      const cand = live(e).find((x) => x.id === params.candidate_id);
      if (cand === undefined) return c.notFound();
      if (!["yes", "maybe", "no"].includes(body.vote)) return invalid("vote");
      e.votes.get(cand.id)?.set(viewer, body.vote);
      return { status: 200, body: candidateShown(e, cand) };
    },
    "GET /events/{id}/candidates/{candidate_id}/votes": ({ params, query }) => {
      const e = seen(params.id);
      if ("problem" in e) return e;
      const cand = e.candidates.find((x) => x.id === params.candidate_id);
      if (cand === undefined) return c.notFound();
      const only = query.get("vote");
      const items = [...(e.votes.get(cand.id) ?? new Map<string, Vote>())]
        .filter(([, v]) => only === null || v === only)
        .flatMap(([u, vote]) => {
          const account = c.accountOf(u);
          return account === undefined ? [] : [{ account, vote }];
        })
        .toSorted((a, b) => a.account.id.localeCompare(b.account.id));
      return { status: 200, body: c.page(items, query, (b) => b.account.id).body };
    },
    "PUT /events/{id}/coorganizers/{account_id}": ({ params }) => {
      const e = managed(params.id, true);
      if ("problem" in e) return e;
      const u = named(params.account_id);
      if (u !== undefined && e.coorganizers.includes(u)) return ok(e);
      if (u === undefined || u === e.organizer || separated(e, u))
        return refused(422, "event-invalid-coorganizer", "This account cannot co-organize the event");
      if (e.coorganizers.length >= 3)
        return refused(409, "event-too-many-coorganizers", "The event has as many co-organizers as it can");
      e.coorganizers.push(u);
      return ok(e);
    },
    "DELETE /events/{id}/coorganizers/{account_id}": ({ params }) => {
      const e = managed(params.id, true);
      if ("problem" in e) return e;
      const u = c.usernameOf(params.account_id);
      e.coorganizers = e.coorganizers.filter((x) => x !== u);
      return ok(e);
    },
    "GET /events/{id}/invitations": ({ params, query }) => {
      const e = seen(params.id);
      if ("problem" in e) return e;
      if (!organizes(e)) return notOrganizer();
      const items = e.invitations.filter((i) => !i.revoked).map(invitationShown);
      return { status: 200, body: c.page(items, query, (i) => i.id).body };
    },
    "POST /events/{id}/invitations": ({ body, params }) => {
      const e = managed(params.id);
      if ("problem" in e) return e;
      const u = c.usernameOf(body.account_id);
      if (u !== undefined && e.invitations.some((i) => i.by === u))
        return refused(409, "event-already-invited", "This account was invited to the event before");
      if (u === undefined || !c.active(u) || u === e.organizer || separated(e, u))
        return refused(422, "event-invalid-invitee", "This account cannot be invited to the event");
      const now = Date.now();
      const sent = (invitationsSent.get(e.id) ?? []).filter((at) => at > now - 60 * minute);
      if (sent.length >= inviteLimit) return rateLimited(sent[0] ?? now);
      invitationsSent.set(e.id, [...sent, now]);
      const invitation = { id: newId(), by: u, invitedBy: viewer, at: now, revoked: false };
      e.invitations.push(invitation);
      return { status: 201, body: invitationShown(invitation) };
    },
    "DELETE /events/{id}/invitations/{account_id}": ({ params }) => {
      const e = managed(params.id);
      if ("problem" in e) return e;
      const u = c.usernameOf(params.account_id);
      const kept = e.invitations.find((i) => i.by === u && !i.revoked);
      if (kept === undefined) return c.notFound();
      kept.revoked = true;
      return { status: 204 };
    },
    "GET /events/{id}/board": ({ params, query }) => {
      const e = seen(params.id);
      if ("problem" in e) return e;
      if (!readsBoard(e)) return notReader();
      return { status: 200, body: c.page(c.boardPosts(e.id), query, (s) => s.id).body };
    },
    "POST /events/{id}/board": ({ params, body, request }) => {
      const e = seen(params.id);
      if ("problem" in e) return e;
      if (!readsBoard(e)) return notReader();
      if (boardClosed(e))
        return refused(
          409,
          "event-board-closed",
          "This event's board is read-only: the event was cancelled, or 7 days have passed since it ended.",
        );
      if (!admitted(e))
        return refused(
          403,
          "event-board-not-writer",
          "Only the people this event's organizer chose can write on its board.",
        );
      if (body.in_reply_to_id !== undefined && !c.boardPosts(e.id).some((s) => s.id === body.in_reply_to_id))
        return invalid("in_reply_to_id");
      if (body.text.trim() === "" && (body.media ?? []).length === 0) return invalid("text");
      return { status: 201, body: c.boardPost(e.id, viewer, body, Date.now(), request) };
    },
    "GET /preferences/event_reminders": () => ({ status: 200, body: remindersShown() }),
    "PUT /preferences/event_reminders": ({ body }) => {
      if (body.defaults) {
        reminders = undefined;
        return { status: 200, body: remindersShown() };
      }
      const chosen = body.minutes_before ?? [];
      if (
        chosen.length > 3 ||
        new Set(chosen).size !== chosen.length ||
        chosen.some((m) => !Number.isInteger(m) || m < 5 || m > reminderMax)
      )
        return invalid("minutes_before");
      reminders = chosen.toSorted((a, b) => b - a);
      return { status: 200, body: remindersShown() };
    },
  };

  const byKey = (key: string) => held.find((e) => e.key === key);
  const events: FakeEvents = {
    id: (key) => byKey(key)?.id ?? "none",
    shown: (key) => {
      const e = byKey(key);
      return e === undefined || !sees(e) ? undefined : shown(e);
    },
  };
  return { handlers, events };
}
