// SPDX-License-Identifier: AGPL-3.0-only

// The fake's voice chat rooms (ADR-0100 §5, §6, §14; ADR-0101 §3, §5, §6): the open rooms, one room and the rooms the
// reader hosted (none), joining with a new session and leaving, the session's heartbeat, its stream, the microphone, a
// new grant, the signals of P2P (taken, and carried nowhere), reactions, speaking and the raised hand. The grant is
// P2P's with no ICE servers, so that a call in a story connects only within the machine, and the fake's rooms have no
// other live session to connect to. The state is the fake's own, in memory; a test makes the stream say what the
// server would (`FakeVoice.send`), or drops it (`FakeVoice.drop`).

import type { Handlers, Refusal, Schema } from "./spec";

type Room = Schema<"VoiceRoom">;
type Participant = Schema<"VoiceRoomParticipant">;

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
};

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

  const roomOf = (id: string) => c.rooms.find((r) => r.id === id);
  const grant = (): Schema<"VoiceGrant"> => ({
    kind: "p2p",
    options: {},
    ice_servers: [],
    relay_only: false,
    expires_at: new Date(Date.now() + grantLifetime).toISOString(),
  });
  /** The room without the sessions, as the operations other than the stream show it. */
  const shown = (room: Room): Room => ({
    ...room,
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
      if (event === "ended") c.rooms.splice(c.rooms.indexOf(room), 1);
      else takeOut(room);
    }
    joined = null;
  };
  /** Changes the reader's participant in `id`, sends the room on the stream, and answers it. */
  const changeSelf = (id: string, change: Partial<Participant>) => {
    const room = roomOf(id);
    const self = room === undefined ? undefined : viewerIn(room);
    if (room === undefined || self === undefined) return c.notFound();
    Object.assign(self, change);
    if (joined?.room === id) write("state", room);
    return { status: 200 as const, body: shown(room) };
  };
  const sessionOf = (body: unknown) => (body as { session?: string } | undefined)?.session;

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
    "GET /voice/history": () => (c.off ? c.notFound() : { status: 200, body: { items: [] } }),
    "POST /voice/rooms/{id}/participation": ({ params }) => {
      const room = c.off ? undefined : roomOf(params.id);
      if (room === undefined) return c.notFound();
      // Joining leaves the room the reader was in, and replaces the session of a reader in this one already.
      if (joined !== null)
        end(joined.room === room.id ? "replaced" : "removed", joined.room === room.id ? "replaced" : "moved");
      sessions += 1;
      const session = `fake-session-${String(sessions)}`;
      room.participants.push({
        account: c.viewer(),
        role: "listener",
        hosting: false,
        cohosting: false,
        hand_raised: false,
        blocked: false,
        session,
      });
      room.participants_count += 1;
      joined = { room: room.id, session };
      opened = 0;
      return { status: 200, body: { room: shown(room), session, grant: grant() } };
    },
    "DELETE /voice/rooms/{id}/participation": ({ params, query }) => {
      if (c.off || joined?.room !== params.id) return c.notFound();
      // A session that no longer holds the reader's place leaves nothing.
      const session = query.get("session");
      if (session !== null && session !== joined.session) return { status: 204 };
      end("removed", "left");
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
          write("state", room);
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
        if (room !== undefined) write("state", room);
        return;
      }
      write(event.event, event.data);
    },
    drop: closeStream,
    opened: () => opened,
  };
  return { handlers, voice };
}
