// SPDX-License-Identifier: AGPL-3.0-only

// The fake's voice chat rooms (ADR-0100 §5, §6, §7, §13, §14; ADR-0101 §3, §5, §6): the open rooms, one room and the
// rooms the reader hosted, joining with a new session and leaving, the session's heartbeat, its stream, the
// microphone, a new grant, the signals of P2P (taken, and carried nowhere), reactions, speaking and the raised hand;
// and the host's side: opening a room (its start post taken and posted nowhere), changing its settings, ending it,
// what the host and the co-hosts do to a participant, handing it over, the invitations, and the host's leave, which
// names the next host or ends the room. The grant is
// P2P's with no ICE servers, so that a call in a story connects only within the machine, and the fake's rooms have no
// other live session to connect to. The state is the fake's own, in memory; a test makes the stream say what the
// server would (`FakeVoice.send`), or drops it (`FakeVoice.drop`).

import type { Handlers, Refusal, Schema } from "./spec";

type Room = Schema<"VoiceRoom">;
type Participant = Schema<"VoiceRoomParticipant">;

/** The longest text of a start post the fake takes, the room's link aside. */
const startPostLimit = 500;

/** The most co-hosts a room has. */
const cohostLimit = 3;

/** How long the fake's grants last: as the server's TURN credentials by default. */
const grantLifetime = 6 * 60 * 60_000;

/** An event the fake's room stream sends, as the server writes it: `state` sends the room as the fake holds it. */
export type FakeVoiceEvent =
  | { readonly event: "state" }
  | { readonly event: "mic"; readonly data: Schema<"VoiceStreamMic"> }
  | { readonly event: "reaction"; readonly data: Schema<"VoiceStreamReaction"> }
  | { readonly event: "signal"; readonly data: Schema<"VoiceStreamSignal"> }
  | { readonly event: "muted"; readonly data: Schema<"VoiceStreamMuted"> }
  | { readonly event: "grant"; readonly data: Schema<"VoiceStreamGrant"> }
  | { readonly event: "removed" | "replaced" | "ended"; readonly data: Schema<"VoiceStreamEnd"> };

/** What a test does with the reader's call on the fake. */
export type FakeVoice = {
  /** The ID of the fake's open room at `index`, in the order the list gives them. */
  readonly roomId: (index: number) => string;
  /** The reader's live session, and its room; null while the reader is in no room. */
  readonly joined: () => { readonly room: string; readonly session: string } | null;
  /**
   * Sends `event` on the stream of the reader's session. `removed`, `replaced` and `ended` then close it, and the
   * server holds the session no more (its heartbeat and stream answer 404); `ended` ends the room too.
   */
  readonly send: (event: FakeVoiceEvent) => void;
  /** Closes the stream of the reader's session without an event, as a network or a proxy does; the session stays. */
  readonly drop: () => void;
  /** How many times the stream of the reader's session was opened, since the session joined. */
  readonly opened: () => number;
  /** The ID of the fake's room that ended: its summary says so, and the room answers 404. */
  readonly endedRoomId: () => string;
  /** The ID of the fake's open room the reader does not see: its summary says it is open, and the room answers 404. */
  readonly hiddenRoomId: () => string;
  /** The participant of `accountId` leaves the reader's room: its stream says the room again. */
  readonly participantLeaves: (accountId: string) => void;
  /** A host lets the reader speak in the room they are in (an explicit grant): its stream says the room again. */
  readonly grantReader: () => void;
  /**
   * The host of the reader's room, another than the reader, sets its maximum to `max`, below the people in it too
   * (nobody leaves): its stream says the room again.
   */
  readonly hostSetsMax: (max: number) => void;
  /** The ID the next room the reader opens takes, known before it opens (a story's address). */
  readonly nextRoomId: () => string;
  /** The start posts of the rooms the reader opened, in order: the fake posts them nowhere. */
  readonly startPosts: () => readonly { readonly room: string; readonly text: string }[];
  /** The open room where the account of `id` hosts or speaks, as its profile has it; undefined for none. */
  readonly entryOf: (id: string) => Schema<"VoiceRoomEntry"> | undefined;
};

/**
 * More of the lists the voice operations page, past a page of the server's: `rooms` more open rooms (copies of the
 * fake's second, named Open room 1, 2, …, after the others), `history` more rooms the reader hosted (ended, older than
 * the others), and the accounts of `invited` (usernames of this server) invited as listeners to each room the reader
 * hosts, the oldest invitation first.
 */
export type FakeVoiceMany = {
  readonly rooms?: number;
  readonly history?: number;
  readonly invited?: readonly string[];
};

/**
 * The most people a room holds: the server's (`server`, the instance's `voice_max_participants`, 10 by default), each
 * open room's by its place in the list (`rooms`, the server's where none), which a join at it refuses (409
 * `voice-room-full`); and whether the media node has no room for more now (`noRoom`), which refuses a room's maximum
 * raised (503 `voice-unavailable`).
 */
export type FakeVoiceMax = {
  readonly server?: number;
  readonly rooms?: readonly (number | undefined)[];
  readonly noRoom?: boolean;
};

/** Why the fake refuses to open a room, as the server's problem types name it. */
export type FakeVoiceOpenRefusal = "not-eligible" | "too-many-rooms" | "hosting-elsewhere";

/** What the voice handlers take from the fake. */
export type VoiceContext = {
  /** The open rooms, which the handlers change as the reader joins and leaves. */
  readonly rooms: Room[];
  /** The reader's account. */
  readonly viewer: () => Schema<"Account">;
  /** The voice rooms are off: every operation answers 404. */
  readonly off: boolean;
  /** The rooms the list shows: all, or none. */
  readonly listed: () => readonly Room[];
  /** The server shows the summaries of its rooms (`voice_public_summary`). */
  readonly summaries: boolean;
  /** A room that ended. */
  readonly ended: { readonly id: string; readonly name: string };
  /** An open room the reader does not see (its host blocks them, it is for the host's followers). */
  readonly hidden: { readonly id: string; readonly name: string };
  /** The rooms the reader hosted, newest first; a room the reader opens comes first. */
  readonly history: Schema<"VoiceRoomHistoryEntry">[];
  /** Why the server refuses every opening of a room; none by default. */
  readonly openRefusal?: FakeVoiceOpenRefusal | undefined;
  /** More rooms, hosted rooms and invitations than a page holds; none by default. */
  readonly many?: FakeVoiceMany | undefined;
  /** The server's maximum of the people in a room, the bound of a room's own. */
  readonly max: number;
  /** The media node has no room for more now: a room's maximum raised is refused. */
  readonly noRoom: boolean;
  /** The account of the person of `username` of this server; undefined for none. */
  readonly accountByName: (username: string) => Schema<"Account"> | undefined;
  /** A new ID, for a room or an invitation, newer than every one before. */
  readonly newId: () => string;
  /** The account of `id`, of this server or another; undefined for none. */
  readonly accountById: (id: string) => Schema<"Account"> | undefined;
  /** Whether the account of `follower` follows the account of `followee`, by their IDs. */
  readonly follows: (follower: string, followee: string) => boolean;
  /** Whether the reader blocks the account of `id`, or it blocks the reader. */
  readonly blocked: (id: string) => boolean;
  readonly problem: (status: number, title: string, more?: Partial<Schema<"Problem">>) => Refusal;
  readonly notFound: () => Refusal;
  readonly page: <T extends { id: string }>(
    items: readonly T[],
    query: { get: (name: "cursor" | "limit") => string | null },
  ) => { body: { items: T[]; next_cursor?: string } };
};

/** The handlers of the voice operations, and what a test does with them. */
export function voiceHandlers(c: VoiceContext): { handlers: Handlers; voice: FakeVoice } {
  const encoder = new TextEncoder();
  let sessions = 0;
  let joined: { room: string; session: string } | null = null;
  let stream: ReadableStreamDefaultController<Uint8Array> | null = null;
  let opened = 0;
  /** The invitations of the rooms, revoked ones too: a revoked account is not invited again. */
  const invitations: { room: string; invitation: Schema<"VoiceRoomInvitation">; revoked: boolean }[] = [];
  /** The accounts removed from a room (`room>account`), which are not invited to it again. */
  const removed = new Set<string>();
  /** The start posts the rooms opened with, which the fake posts nowhere. */
  const startPosts: { room: string; text: string }[] = [];
  /** The rooms the reader was let speak in by a host (an explicit grant), by their IDs. */
  const grants = new Set<string>();
  /** The ID the next room opened takes, once a test asked for it. */
  let reserved: string | undefined;
  /** The rooms that ended while the fake ran, by their IDs: their summary says so. */
  const endedRooms = new Map<string, string>([[c.ended.id, c.ended.name]]);

  const roomOf = (id: string) => c.rooms.find((r) => r.id === id);

  // The lists made longer than a page (`many`).
  const model = c.rooms[1] ?? c.rooms[0];
  if (model !== undefined)
    for (let i = 0; i < (c.many?.rooms ?? 0); i++)
      c.rooms.push({ ...model, id: c.newId(), name: `Open room ${String(i + 1)}` });
  const lastHosted = c.history.at(-1);
  for (let i = 0; i < (c.many?.history ?? 0); i++) {
    const opened = Date.parse(lastHosted?.opened_at ?? new Date().toISOString()) - (i + 1) * 24 * 60 * 60_000;
    c.history.push({
      id: c.newId(),
      name: `Evening room ${String(i + 1)}`,
      opened_at: new Date(opened).toISOString(),
      ended_at: new Date(opened + 60 * 60_000).toISOString(),
      end_reason: "host",
    });
  }
  for (const room of c.rooms.filter((r) => r.host.id === c.viewer().id))
    for (const username of c.many?.invited ?? []) {
      const account = c.accountByName(username);
      if (account === undefined) continue;
      const invitation: Schema<"VoiceRoomInvitation"> = {
        id: c.newId(),
        account,
        role: "listener",
        invited_by_id: c.viewer().id,
        invited_at: new Date().toISOString(),
      };
      invitations.push({ room: room.id, invitation, revoked: false });
    }
  const grant = (): Schema<"VoiceGrant"> => ({
    kind: "p2p",
    options: {},
    ice_servers: [],
    relay_only: false,
    expires_at: new Date(Date.now() + grantLifetime).toISOString(),
  });
  /** The room without the sessions, as the operations other than the stream show it. */
  /**
   * Whether the reader may speak in `room` without asking (the server's `can_speak`, per viewer): its host and
   * co-hosts; one granted to (by a host, or invited as a speaker); and one the room's speakers cover; never one not in
   * it.
   */
  const canSpeak = (room: Room): boolean => {
    const self = viewerIn(room);
    if (self === undefined) return false;
    if (self.hosting || self.cohosting || grants.has(room.id)) return true;
    switch (room.speakers) {
      case "everyone":
        return true;
      case "followers":
        return c.follows(c.viewer().id, room.host.id);
      case "following":
        return c.follows(room.host.id, c.viewer().id);
      case "mutuals":
        return c.follows(c.viewer().id, room.host.id) && c.follows(room.host.id, c.viewer().id);
      case "manual":
        return false;
    }
  };
  /** The room as the reader's stream says it: its sessions, and whether the reader may speak. */
  const streamed = (room: Room): Room => ({ ...room, can_speak: canSpeak(room) });
  const shown = (room: Room): Room => ({
    ...room,
    can_speak: canSpeak(room),
    participants: room.participants.map((p) => {
      const copy = { ...p };
      delete copy.session;
      return copy;
    }),
  });
  const viewerIn = (room: Room) => room.participants.find((p) => p.account.id === c.viewer().id);
  const live = (room: string, session: string | undefined) =>
    joined !== null && joined.room === room && joined.session === session;

  const write = (name: string, data: unknown) => {
    stream?.enqueue(encoder.encode(`event: ${name}\ndata: ${JSON.stringify(data)}\n\n`));
  };
  const closeStream = () => {
    const open = stream;
    stream = null;
    try {
      open?.close();
    } catch {
      // The reader gave the stream up already.
    }
  };
  /** The reader's participant leaves `room`. */
  const takeOut = (room: Room) => {
    const before = room.participants.length;
    room.participants = room.participants.filter((p) => p.account.id !== c.viewer().id);
    room.participants_count -= before - room.participants.length;
  };
  /** The server holds the reader's session no more, after its stream said why. */
  const end = (event: "removed" | "replaced" | "ended", reason: Schema<"VoiceStreamEnd">["reason"]) => {
    write(event, { reason });
    closeStream();
    const room = joined === null ? undefined : roomOf(joined.room);
    if (room !== undefined) {
      if (event === "ended") {
        c.rooms.splice(c.rooms.indexOf(room), 1);
        endedRooms.set(room.id, room.name);
      } else takeOut(room);
    }
    joined = null;
  };
  /** Changes the reader's participant in `id`, sends the room on the stream, and answers it. */
  const changeSelf = (id: string, change: Partial<Participant>) => {
    const room = roomOf(id);
    const self = room === undefined ? undefined : viewerIn(room);
    if (room === undefined || self === undefined) return c.notFound();
    Object.assign(self, change);
    if (joined?.room === id) write("state", streamed(room));
    return { status: 200 as const, body: shown(room) };
  };
  const sessionOf = (body: unknown) => (body as { session?: string } | undefined)?.session;

  /** The reader joins `room` with a new session, as its host or as a listener; what the join answers. */
  const enter = (room: Room, hosting: boolean): Schema<"VoiceRoomJoin"> => {
    // Joining leaves the room the reader was in, and replaces the session of a reader in this one already.
    if (joined !== null)
      end(joined.room === room.id ? "replaced" : "removed", joined.room === room.id ? "replaced" : "moved");
    sessions += 1;
    const session = `fake-session-${String(sessions)}`;
    room.participants.push({
      account: c.viewer(),
      role: hosting ? "speaker" : "listener",
      hosting,
      cohosting: false,
      hand_raised: false,
      blocked: false,
      session,
    });
    room.participants_count += 1;
    joined = { room: room.id, session };
    opened = 0;
    return { room: shown(room), session, grant: grant() };
  };
  /** The room ends: it is no longer open, and the reader's history says when and why. */
  const closeRoom = (room: Room, reason: Schema<"VoiceEndReason">) => {
    const at = c.rooms.indexOf(room);
    if (at >= 0) c.rooms.splice(at, 1);
    const kept = c.history.find((h) => h.id === room.id);
    if (kept !== undefined && kept.ended_at === undefined)
      Object.assign(kept, { ended_at: new Date().toISOString(), end_reason: reason });
  };
  /** The host ends `room` for everyone: the stream of the reader in it says so. */
  const endRoom = (room: Room) => {
    if (joined?.room === room.id) end("ended", "host");
    closeRoom(room, "host");
  };
  /** `next` becomes the host of `room`; the host before stays in it as a participant. */
  const handOver = (room: Room, next: Participant) => {
    for (const p of room.participants) p.hosting = false;
    next.hosting = true;
    next.cohosting = false;
    next.role = "speaker";
    next.hand_raised = false;
    room.host = next.account;
    if (next.account.id === c.viewer().id && !c.history.some((h) => h.id === room.id))
      c.history.unshift({ id: room.id, name: room.name, opened_at: room.opened_at });
  };
  /** Whether a participant may host rooms: every local account of the fake may. */
  const eligible = (p: Participant) => !p.account.acct.includes("@");
  /** Whether the reader is the host or a co-host of `room`. */
  const moderates = (room: Room) => {
    const self = viewerIn(room);
    return self !== undefined && (self.hosting || self.cohosting);
  };
  const notHost = () => c.problem(403, "Only the host can do this", { type: "/api/v1/problems/voice-not-host" });
  const notHostOrCohost = () =>
    c.problem(403, "Only the host or a co-host can do this", { type: "/api/v1/problems/voice-not-host-or-cohost" });
  const invalidParticipant = () =>
    c.problem(422, "This participant cannot be operated on", { type: "/api/v1/problems/voice-invalid-participant" });
  const openRefused = (why: FakeVoiceOpenRefusal) =>
    why === "not-eligible"
      ? c.problem(403, "You may not open rooms", { type: "/api/v1/problems/voice-not-eligible" })
      : c.problem(409, why === "too-many-rooms" ? "Too many rooms are open" : "You host another room", {
          type: `/api/v1/problems/voice-${why}`,
        });
  /** The room sent on the stream of the reader in it, and answered. */
  const changed = (room: Room) => {
    if (joined?.room === room.id) write("state", streamed(room));
    return { status: 200 as const, body: shown(room) };
  };
  /**
   * Runs `change` on the participant `accountId` of the room `id`, by the reader, its host or a co-host (`hostOnly`:
   * its host), as the server's rules say: nobody acts on the host or on themselves. A refusal `change` returns is the
   * answer; else the room after it.
   */
  const operate = (
    id: string,
    accountId: string,
    hostOnly: boolean,
    change: (room: Room, p: Participant) => Refusal | undefined,
  ) => {
    const room = c.off ? undefined : roomOf(id);
    if (room === undefined) return c.notFound();
    const self = viewerIn(room);
    if (hostOnly ? self?.hosting !== true : !moderates(room)) return hostOnly ? notHost() : notHostOrCohost();
    const p = room.participants.find((q) => q.account.id === accountId);
    if (p === undefined || p.hosting || p.account.id === c.viewer().id) return invalidParticipant();
    const refused = change(room, p);
    return refused ?? changed(room);
  };
  /** The icon of the settings, as the room holds it. */
  const iconOf = (icon: string | undefined): Pick<Room, "icon"> => {
    const name = icon?.replace(/^:|:$/g, "") ?? "";
    return name === "" ? {} : { icon: { name } };
  };
  const tagsOf = (tags: readonly string[] | undefined) => (tags ?? []).map((tag) => tag.replace(/^#/, ""));
  /**
   * What the server refuses of a room's settings: a name with nothing in it or too long, more than 5 hashtags, a
   * maximum out of 2 to the server's.
   */
  const settingsFault = (body: {
    name: string;
    hashtags?: readonly string[] | undefined;
    max_participants?: number | undefined;
  }): Refusal | null => {
    const name = body.name.trim();
    // The server counts a name's characters as a reader sees them (graphemes).
    const length = [...new Intl.Segmenter("en", { granularity: "grapheme" }).segment(name)].length;
    if (name === "" || length > 100 || name.includes("\n"))
      return c.problem(422, "This value is not valid", { field: "name" });
    if ((body.hashtags?.length ?? 0) > 5) return c.problem(422, "This value is not valid", { field: "hashtags" });
    const max = body.max_participants;
    if (max !== undefined && (!Number.isInteger(max) || max < 2 || max > c.max))
      return c.problem(422, "This value is not valid", { field: "max_participants" });
    return null;
  };

  const handlers: Handlers = {
    "GET /voice/rooms": ({ query }) => {
      if (c.off) return c.notFound();
      const tag = query.get("tag");
      const rooms = c.listed().filter((r) => tag === null || r.hashtags.includes(tag));
      return { status: 200, body: c.page(rooms.map(shown), query).body };
    },
    "GET /voice/rooms/{id}": ({ params }) => {
      const room = c.off ? undefined : roomOf(params.id);
      return room === undefined ? c.notFound() : { status: 200, body: shown(room) };
    },
    "GET /voice/rooms/{id}/summary": ({ params }) => {
      if (c.off) return c.notFound();
      const room = roomOf(params.id);
      if (room !== undefined) {
        const shows = c.summaries && room.visibility === "local";
        const about = shows
          ? {
              name: room.name,
              ...(room.icon === undefined ? {} : { icon: room.icon }),
              description: room.description,
              host: room.host,
            }
          : {};
        return { status: 200, body: { id: room.id, state: "open" as const, ...about } };
      }
      if (params.id === c.hidden.id) return { status: 200, body: { id: c.hidden.id, state: "open" as const } };
      const ended = endedRooms.get(params.id);
      if (ended !== undefined)
        return {
          status: 200,
          body: { id: params.id, state: "ended" as const, ...(c.summaries ? { name: ended } : {}) },
        };
      return c.notFound();
    },
    "GET /voice/history": ({ query }) => (c.off ? c.notFound() : { status: 200, body: c.page(c.history, query).body }),
    "POST /voice/rooms": ({ body }) => {
      if (c.off) return c.notFound();
      if (c.openRefusal !== undefined) return openRefused(c.openRefusal);
      // A host in an open room with others in it leaves it first (a room that waits for its host holds no one of theirs).
      const hostsElsewhere = c.rooms.some((r) => {
        const self = viewerIn(r);
        return self?.hosting === true && r.participants.some((p) => p.account.id !== c.viewer().id);
      });
      if (hostsElsewhere) return openRefused("hosting-elsewhere");
      const fault = settingsFault(body);
      if (fault !== null) return fault;
      const visibility = body.visibility ?? "local";
      if (body.post !== undefined && visibility !== "local" && visibility !== "followers")
        return c.problem(422, "This room has no start post", { field: "post" });
      // The start post is refused as a post is, under the post's own members.
      if (body.post !== undefined && (body.post.text ?? "").length > startPostLimit)
        return c.problem(422, "This value is not valid", {
          field: "text",
          detail: `Text must be at most ${String(startPostLimit)} characters.`,
        });
      const at = new Date().toISOString();
      const room: Room = {
        id: reserved ?? c.newId(),
        name: body.name.trim(),
        ...iconOf(body.icon),
        description: body.description ?? "",
        hashtags: tagsOf(body.hashtags),
        visibility,
        speakers: body.speakers ?? "manual",
        host: c.viewer(),
        opened_at: at,
        max_participants: body.max_participants ?? c.max,
        participants_count: 0,
        participants: [],
        can_speak: false,
      };
      reserved = undefined;
      c.rooms.push(room);
      c.history.unshift({ id: room.id, name: room.name, opened_at: at });
      if (body.post !== undefined) startPosts.push({ room: room.id, text: body.post.text ?? "" });
      return { status: 201, body: enter(room, true) };
    },
    "PATCH /voice/rooms/{id}": ({ params, body }) => {
      const room = c.off ? undefined : roomOf(params.id);
      if (room === undefined) return c.notFound();
      if (room.host.id !== c.viewer().id) return notHost();
      const fault = settingsFault({ name: room.name, ...body });
      if (fault !== null) return fault;
      // A maximum raised past the room the media node has now is refused, and nothing is saved.
      if (c.noRoom && body.max_participants !== undefined && body.max_participants > room.max_participants)
        return c.problem(503, "The voice chat is unavailable", { type: "/api/v1/problems/voice-unavailable" });
      if (body.name !== undefined) room.name = body.name.trim();
      if (body.icon !== undefined) {
        delete room.icon;
        Object.assign(room, iconOf(body.icon));
      }
      if (body.description !== undefined) room.description = body.description;
      if (body.hashtags !== undefined) room.hashtags = tagsOf(body.hashtags);
      if (body.visibility !== undefined) room.visibility = body.visibility;
      if (body.speakers !== undefined) room.speakers = body.speakers;
      // Lowered below the people in it, nobody leaves.
      if (body.max_participants !== undefined) room.max_participants = body.max_participants;
      const kept = c.history.find((h) => h.id === room.id);
      if (kept !== undefined) kept.name = room.name;
      return changed(room);
    },
    "DELETE /voice/rooms/{id}": ({ params }) => {
      const room = c.off ? undefined : roomOf(params.id);
      if (room === undefined) return c.notFound();
      if (room.host.id !== c.viewer().id) return notHost();
      endRoom(room);
      return { status: 204 };
    },
    "POST /voice/rooms/{id}/participants/{account_id}/speaking_permission": ({ params }) =>
      operate(params.id, params.account_id, false, (_, p) => {
        // A raised hand is granted; the participant becomes a speaker on their own.
        p.hand_raised = false;
      }),
    "DELETE /voice/rooms/{id}/participants/{account_id}/speaking_permission": ({ params }) =>
      operate(params.id, params.account_id, false, (_, p) => {
        p.role = "listener";
      }),
    "DELETE /voice/rooms/{id}/participants/{account_id}/hand": ({ params }) =>
      operate(params.id, params.account_id, false, (_, p) => {
        p.hand_raised = false;
      }),
    "POST /voice/rooms/{id}/participants/{account_id}/mute": ({ params }) => {
      const done = operate(params.id, params.account_id, false, () => undefined);
      if ("problem" in done) return done;
      write("mic", { participant: params.account_id, on: false });
      return { status: 204 };
    },
    "DELETE /voice/rooms/{id}/participants/{account_id}": ({ params }) =>
      operate(params.id, params.account_id, false, (room, p) => {
        // The host and the co-hosts do not remove each other.
        if (p.cohosting) return invalidParticipant();
        room.participants = room.participants.filter((q) => q !== p);
        room.participants_count -= 1;
        removed.add(`${room.id}>${p.account.id}`);
        return undefined;
      }),
    "POST /voice/rooms/{id}/participants/{account_id}/cohost": ({ params }) =>
      operate(params.id, params.account_id, true, (room, p) => {
        if (!p.cohosting && room.participants.filter((q) => q.cohosting).length >= cohostLimit)
          return c.problem(409, "The room has as many co-hosts as it can", {
            type: "/api/v1/problems/voice-too-many-cohosts",
          });
        p.cohosting = true;
        return undefined;
      }),
    "DELETE /voice/rooms/{id}/participants/{account_id}/cohost": ({ params }) =>
      operate(params.id, params.account_id, true, (_, p) => {
        p.cohosting = false;
      }),
    "POST /voice/rooms/{id}/host": ({ params, body }) =>
      operate(params.id, body.account_id, true, (room, p) => {
        if (!eligible(p))
          return c.problem(422, "This participant may not host rooms", {
            type: "/api/v1/problems/voice-invalid-next-host",
          });
        handOver(room, p);
        return undefined;
      }),
    "GET /voice/rooms/{id}/invitations": ({ params, query }) => {
      const room = c.off ? undefined : roomOf(params.id);
      if (room === undefined) return c.notFound();
      if (!moderates(room)) return notHostOrCohost();
      const items = invitations.filter((i) => i.room === room.id && !i.revoked).map((i) => i.invitation);
      return { status: 200, body: c.page(items, query).body };
    },
    "POST /voice/rooms/{id}/invitations": ({ params, body }) => {
      const room = c.off ? undefined : roomOf(params.id);
      if (room === undefined) return c.notFound();
      if (!moderates(room)) return notHostOrCohost();
      const account = c.accountById(body.account_id);
      // An account is invited to a room once, also after a revocation.
      if (invitations.some((i) => i.room === room.id && i.invitation.account.id === body.account_id))
        return c.problem(409, "This account was invited to the room before", {
          type: "/api/v1/problems/voice-already-invited",
        });
      if (
        account === undefined ||
        account.acct.includes("@") ||
        account.id === c.viewer().id ||
        account.id === room.host.id ||
        c.blocked(account.id) ||
        removed.has(`${room.id}>${account.id}`)
      )
        return c.problem(422, "This account cannot be invited to the room", {
          type: "/api/v1/problems/voice-invalid-invitee",
        });
      const invitation: Schema<"VoiceRoomInvitation"> = {
        id: c.newId(),
        account,
        role: body.role ?? "listener",
        invited_by_id: c.viewer().id,
        invited_at: new Date().toISOString(),
      };
      invitations.push({ room: room.id, invitation, revoked: false });
      return { status: 201, body: invitation };
    },
    "DELETE /voice/rooms/{id}/invitations/{account_id}": ({ params }) => {
      const room = c.off ? undefined : roomOf(params.id);
      if (room === undefined) return c.notFound();
      if (!moderates(room)) return notHostOrCohost();
      const kept = invitations.find(
        (i) => i.room === room.id && i.invitation.account.id === params.account_id && !i.revoked,
      );
      if (kept === undefined) return c.notFound();
      kept.revoked = true;
      return { status: 204 };
    },
    "POST /voice/rooms/{id}/participation": ({ params }) => {
      const room = c.off ? undefined : roomOf(params.id);
      if (room === undefined) return c.notFound();
      // A room that holds as many as its maximum, or more after it was lowered, takes no one new; a participant who
      // joins again from another device keeps their place.
      if (viewerIn(room) === undefined && room.participants.length >= room.max_participants)
        return c.problem(409, "The room is full", { type: "/api/v1/problems/voice-room-full" });
      // The host of a room joins it as its host, on the stage (a room opened for a scheduled time waits for them).
      return { status: 200, body: enter(room, room.host.id === c.viewer().id) };
    },
    "DELETE /voice/rooms/{id}/participation": ({ params, query }) => {
      if (c.off || joined?.room !== params.id) return c.notFound();
      // A session that no longer holds the reader's place leaves nothing.
      const session = query.get("session");
      if (session !== null && session !== joined.session) return { status: 204 };
      const room = roomOf(params.id);
      const self = room === undefined ? undefined : viewerIn(room);
      const others = room?.participants.filter((p) => p.account.id !== c.viewer().id) ?? [];
      if (room !== undefined && self?.hosting === true && others.length > 0) {
        // The host of a room with others in it names the next host, or ends it.
        if (query.get("end") === "true") {
          endRoom(room);
          return { status: 204 };
        }
        const nextId = query.get("next_host_id");
        if (nextId === null)
          return c.problem(409, "Choose the next host or end the room", {
            type: "/api/v1/problems/voice-host-must-choose",
          });
        const next = others.find((p) => p.account.id === nextId);
        if (next === undefined || !eligible(next))
          return c.problem(422, "The next host may not host rooms", {
            type: "/api/v1/problems/voice-invalid-next-host",
          });
        handOver(room, next);
      }
      end("removed", "left");
      if (room !== undefined && room.participants.length === 0) closeRoom(room, "empty");
      return { status: 204 };
    },
    "POST /voice/rooms/{id}/heartbeat": ({ params, body }) =>
      live(params.id, sessionOf(body)) ? { status: 204 } : c.notFound(),
    "GET /voice/rooms/{id}/stream": ({ params, query, request }) => {
      const session = query.get("session") ?? undefined;
      const room = roomOf(params.id);
      if (room === undefined || !live(params.id, session)) return c.notFound();
      // A session holds one stream: opening it again ends the one before.
      closeStream();
      opened += 1;
      let own: ReadableStreamDefaultController<Uint8Array> | null = null;
      const raw = new ReadableStream<Uint8Array>({
        start(controller) {
          own = controller;
          stream = controller;
          controller.enqueue(encoder.encode(": connected\n\n"));
          write("state", streamed(room));
          request.signal.addEventListener(
            "abort",
            () => {
              if (stream === own) closeStream();
            },
            { once: true },
          );
        },
        cancel() {
          if (stream === own) stream = null;
        },
      });
      return { status: 200, type: "text/event-stream", raw };
    },
    "POST /voice/rooms/{id}/mic": ({ params, body }) => {
      if (!live(params.id, body.session)) return c.notFound();
      const room = roomOf(params.id);
      const self = room === undefined ? undefined : viewerIn(room);
      if (body.on && self?.role !== "speaker")
        return c.problem(403, "You cannot speak in this room", { type: "/api/v1/problems/voice-cannot-speak" });
      write("mic", { participant: c.viewer().id, on: body.on });
      return { status: 204 };
    },
    "POST /voice/rooms/{id}/grant": ({ params, body }) =>
      live(params.id, body.session) ? { status: 200, body: grant() } : c.notFound(),
    "POST /voice/rooms/{id}/signals": ({ params, body }) =>
      live(params.id, body.session) ? { status: 204 } : c.notFound(),
    "POST /voice/rooms/{id}/reactions": ({ params, body }) => {
      if (joined?.room !== params.id) return c.notFound();
      write("reaction", { participant: c.viewer().id, emoji: body.emoji });
      return { status: 204 };
    },
    "POST /voice/rooms/{id}/speaker": ({ params }) => changeSelf(params.id, { role: "speaker", hand_raised: false }),
    "DELETE /voice/rooms/{id}/speaker": ({ params }) => changeSelf(params.id, { role: "listener" }),
    "POST /voice/rooms/{id}/hand": ({ params }) => changeSelf(params.id, { hand_raised: true }),
    "DELETE /voice/rooms/{id}/hand": ({ params }) => changeSelf(params.id, { hand_raised: false }),
  };

  const voice: FakeVoice = {
    roomId: (index) => c.listed()[index]?.id ?? "none",
    joined: () => joined,
    send: (event) => {
      if (event.event === "removed" || event.event === "replaced" || event.event === "ended") {
        end(event.event, event.data.reason);
        return;
      }
      if (event.event === "state") {
        const room = joined === null ? undefined : roomOf(joined.room);
        if (room !== undefined) write("state", streamed(room));
        return;
      }
      write(event.event, event.data);
    },
    drop: closeStream,
    opened: () => opened,
    endedRoomId: () => c.ended.id,
    hiddenRoomId: () => c.hidden.id,
    startPosts: () => startPosts,
    nextRoomId: () => (reserved ??= c.newId()),
    hostSetsMax: (max) => {
      const room = joined === null ? undefined : roomOf(joined.room);
      if (room === undefined) return;
      room.max_participants = max;
      write("state", streamed(room));
    },
    participantLeaves: (accountId) => {
      const room = joined === null ? undefined : roomOf(joined.room);
      if (room === undefined) return;
      const before = room.participants.length;
      room.participants = room.participants.filter((p) => p.account.id !== accountId);
      room.participants_count -= before - room.participants.length;
      write("state", streamed(room));
    },
    grantReader: () => {
      const room = joined === null ? undefined : roomOf(joined.room);
      if (room === undefined) return;
      grants.add(room.id);
      const self = viewerIn(room);
      if (self !== undefined) self.hand_raised = false;
      write("state", streamed(room));
    },
    entryOf: (id) => {
      if (c.off) return undefined;
      for (const room of c.listed()) {
        const p = room.participants.find((q) => q.account.id === id && q.role === "speaker");
        if (p === undefined) continue;
        return {
          id: room.id,
          name: room.name,
          ...(room.icon === undefined ? {} : { icon: room.icon }),
          role: p.hosting ? ("host" as const) : ("speaker" as const),
          participants_count: room.participants_count,
        };
      }
      return undefined;
    },
  };
  return { handlers, voice };
}
