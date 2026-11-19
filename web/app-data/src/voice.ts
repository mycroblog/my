// SPDX-License-Identifier: AGPL-3.0-only
// The voice chat rooms (ADR-0100 §5, §6, §14; ADR-0101 §3, §5, §6): the open rooms, by tag, one room and the rooms the
// reader hosted, as queries; joining and leaving, the heartbeat of a session, a new grant, the signals of P2P, the
// microphone, reactions, speaking and the raised hand, and the operations of the host and the co-hosts, as requests;
// and the room's stream of a session (GET /voice/rooms/{id}/stream), read with fetch since EventSource cannot send the
// access token, and opened again on the same session whenever it drops, the first `state` bringing the room up to
// date (nothing is replayed). The call that holds a session is @my/app-features'; this module keeps nothing of it.
import { EventStreamParser, ProblemError, type components, type ServerSentEvent } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import type { VoiceRoom } from "./around";
import { pagedQuery, usePagedList, type PagedList } from "./paged";
import { useAppData } from "./provider";

type Schemas = components["schemas"];

/** A participant of a voice room. */
export type VoiceRoomParticipant = Schemas["VoiceRoomParticipant"];
/** What a session needs to connect to the audio of a room. */
export type VoiceGrant = Schemas["VoiceGrant"];
/** A joined session: the room, the new session, and its grant. */
export type VoiceJoin = Schemas["VoiceRoomJoin"];
/** A room the reader hosted. */
export type VoiceRoomHistoryEntry = Schemas["VoiceRoomHistoryEntry"];
/** Why a session left a room, or why the room ended, as the stream's `removed`, `replaced` and `ended` say. */
export type VoiceEndReason = Schemas["VoiceStreamEnd"]["reason"];
/** A signal of P2P to another session of the room. */
export type VoiceSignalOut = Omit<Schemas["VoiceSignal"], "session">;

/** An event of the room's stream, its data parsed; the stream ends after `removed`, `replaced` and `ended`. */
export type VoiceStreamEvent =
  | { readonly type: "state"; readonly room: VoiceRoom }
  | { readonly type: "mic"; readonly participant: string; readonly on: boolean }
  | { readonly type: "reaction"; readonly participant: string; readonly emoji: string }
  | { readonly type: "signal"; readonly signal: Schemas["VoiceStreamSignal"] }
  | { readonly type: "muted"; readonly by: string }
  | { readonly type: "grant"; readonly grant: VoiceGrant; readonly reconnect: boolean }
  | { readonly type: "removed" | "replaced" | "ended"; readonly reason: VoiceEndReason };

// ---- Queries ----------------------------------------------------------------------------------------------------------

/** The open voice rooms the viewer sees, in the server's order, a page at a time; with `tag`, those of the hashtag. */
export function voiceRoomsQuery({ api, queries }: AppData, tag?: string) {
  const query = tag === undefined ? {} : { tag };
  return pagedQuery<VoiceRoom>(
    queries.queryOptions("get", "/voice/rooms", { params: { query } }).queryKey,
    async (cursor, signal) =>
      (
        await api.GET("/voice/rooms", {
          params: { query: cursor === undefined ? query : { ...query, cursor } },
          signal,
        })
      ).data,
  );
}

/** One open room the viewer sees. */
export function voiceRoomQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/voice/rooms/{id}", { params: { path: { id } } });
}

/** The rooms the reader hosted, newest first, a page at a time. */
export function voiceHistoryQuery({ api, queries }: AppData) {
  return pagedQuery<VoiceRoomHistoryEntry>(
    queries.queryOptions("get", "/voice/history").queryKey,
    async (cursor, signal) =>
      (await api.GET("/voice/history", { params: { query: cursor === undefined ? {} : { cursor } }, signal })).data,
  );
}

/** The open rooms the viewer sees (of `tag`, with one), for their list; nothing is read while `enabled` is false. */
export function useVoiceRooms(tag?: string, enabled = true): PagedList<VoiceRoom> {
  return usePagedList(voiceRoomsQuery(useAppData(), tag), enabled);
}

/** The room of `id`; undefined until the server answered, and its failure (a 404 for a room that ended). */
export function useVoiceRoom(id: string, enabled = true): { room: VoiceRoom | undefined; error: Error | null } {
  const query = useQuery({ ...voiceRoomQuery(useAppData(), id), enabled });
  return enabled ? { room: query.data, error: query.error as Error | null } : { room: undefined, error: null };
}

/** The rooms the reader hosted, newest first. */
export function useVoiceHistory(enabled = true): PagedList<VoiceRoomHistoryEntry> {
  return usePagedList(voiceHistoryQuery(useAppData()), enabled);
}

/** Puts the room as the stream or a write has it into its query. */
export function receiveVoiceRoom(data: AppData, room: VoiceRoom): void {
  data.queryClient.setQueryData(voiceRoomQuery(data, room.id).queryKey, room);
}

/** The reader is no longer in the room `id` (left, removed, or it ended): the room and the lists are read again. */
export function voiceRoomLeft(data: AppData, id: string): void {
  void data.queryClient.invalidateQueries({ queryKey: voiceRoomQuery(data, id).queryKey });
  void data.queryClient.invalidateQueries({
    predicate: (query) => query.queryKey[0] === "get" && query.queryKey[1] === "/voice/rooms",
  });
}

/** The body of an answer, which the client sets whenever the request resolves. */
function answered<T>({ data }: { data?: T }, what: string): T {
  if (data === undefined) throw new Error(`app-data: ${what} answered without a body`);
  return data;
}

// ---- A session --------------------------------------------------------------------------------------------------------

/** Whether `error` says the server holds no such session of the reader (a 404): it dropped it, or the room ended. */
export function isVoiceSessionGone(error: unknown): boolean {
  return error instanceof ProblemError && error.status === 404;
}

/**
 * Joins the room `id` with a new session, which leaves the room the reader was in (a 409 for the host of an open
 * room elsewhere, and for a room that is full).
 */
export async function joinVoiceRoom({ api }: AppData, id: string): Promise<VoiceJoin> {
  return answered(await api.POST("/voice/rooms/{id}/participation", { params: { path: { id } } }), "the join");
}

/** How the host of a room with others in it leaves: handing it to the next host, or ending it. */
export type VoiceLeaveChoice = { readonly nextHostId: string } | { readonly end: true };

/**
 * Leaves the room `id`. The host of a room with others in it names a choice: without one the server refuses (a 409
 * `voice-host-must-choose`), and the reader stays in.
 */
export async function leaveVoiceRoom({ api }: AppData, id: string, choice?: VoiceLeaveChoice): Promise<void> {
  const query = choice === undefined ? {} : "end" in choice ? { end: true } : { next_host_id: choice.nextHostId };
  await api.DELETE("/voice/rooms/{id}/participation", { params: { path: { id }, query } });
}

/**
 * Whether the server refused a leave and the reader stays in: a host of a room with others in it who did not choose
 * (409), or a next host who may not take over (422).
 */
export function isVoiceLeaveRefused(error: unknown): boolean {
  return error instanceof ProblemError && (error.status === 409 || error.status === 422);
}

/**
 * Leaves the room `id` as the page goes away (pagehide): the request outlives the page (`keepalive`) and carries the
 * access token as every request does. A host of a room with others in it is refused, and stays the host until the
 * server's grace after the heartbeats stop.
 */
export function leaveVoiceRoomOnUnload({ api }: AppData, id: string): void {
  api.DELETE("/voice/rooms/{id}/participation", { params: { path: { id } }, keepalive: true }).catch(() => undefined);
}

/** Tells the server that `session` is alive (every 15 seconds; ADR-0100 §6). */
export async function sendVoiceHeartbeat({ api }: AppData, id: string, session: string): Promise<void> {
  await api.POST("/voice/rooms/{id}/heartbeat", { params: { path: { id } }, body: { session } });
}

/** A new grant for `session`, with new TURN credentials (ADR-0101 §5). */
export async function renewVoiceGrant({ api }: AppData, id: string, session: string): Promise<VoiceGrant> {
  return answered(
    await api.POST("/voice/rooms/{id}/grant", { params: { path: { id } }, body: { session } }),
    "the grant",
  );
}

/** Sends a signal of P2P from `session` to another session of the room. */
export async function sendVoiceSignal(
  { api }: AppData,
  id: string,
  session: string,
  signal: VoiceSignalOut,
): Promise<void> {
  await api.POST("/voice/rooms/{id}/signals", { params: { path: { id } }, body: { session, ...signal } });
}

/** Tells the participants that the microphone of `session` is on or off (a 403 for a session that may not speak). */
export async function setVoiceMic({ api }: AppData, id: string, session: string, on: boolean): Promise<void> {
  await api.POST("/voice/rooms/{id}/mic", { params: { path: { id } }, body: { session, on } });
}

/** Sends a reaction (a Unicode emoji or a custom emoji's `:shortcode:`) to the participants. */
export async function reactInVoiceRoom({ api }: AppData, id: string, emoji: string): Promise<void> {
  await api.POST("/voice/rooms/{id}/reactions", { params: { path: { id } }, body: { emoji } });
}

/** What the reader does of their own place in a room: speak, listen again, raise or lower the hand. */
export type VoiceSelfOperation = "speak" | "listen" | "raise-hand" | "lower-hand";

/** Runs `operation` for the reader in the room `id`, and returns the room after it. */
export async function voiceSelfOperation(
  { api }: AppData,
  id: string,
  operation: VoiceSelfOperation,
): Promise<VoiceRoom> {
  const params = { params: { path: { id } } };
  switch (operation) {
    case "speak":
      return answered(await api.POST("/voice/rooms/{id}/speaker", params), operation);
    case "listen":
      return answered(await api.DELETE("/voice/rooms/{id}/speaker", params), operation);
    case "raise-hand":
      return answered(await api.POST("/voice/rooms/{id}/hand", params), operation);
    case "lower-hand":
      return answered(await api.DELETE("/voice/rooms/{id}/hand", params), operation);
  }
}

/**
 * What the host or a co-host does to a participant: let them speak (`grant`), dismiss their raised hand, stop their
 * speaking (`revoke`), mute them, remove them; and the host alone: make or unmake a co-host, and hand the room over.
 */
export type VoiceHostOperation =
  "grant" | "dismiss" | "revoke" | "mute" | "remove" | "cohost" | "uncohost" | "hand-over";

/** Runs `operation` on the participant `accountId` of the room `id`; the room after it, when the server answers one. */
export async function voiceHostOperation(
  { api }: AppData,
  id: string,
  accountId: string,
  operation: VoiceHostOperation,
): Promise<VoiceRoom | undefined> {
  const params = { params: { path: { id, account_id: accountId } } };
  switch (operation) {
    case "grant":
      return (await api.POST("/voice/rooms/{id}/participants/{account_id}/speaking_permission", params)).data;
    case "revoke":
      return (await api.DELETE("/voice/rooms/{id}/participants/{account_id}/speaking_permission", params)).data;
    case "dismiss":
      return (await api.DELETE("/voice/rooms/{id}/participants/{account_id}/hand", params)).data;
    case "cohost":
      return (await api.POST("/voice/rooms/{id}/participants/{account_id}/cohost", params)).data;
    case "uncohost":
      return (await api.DELETE("/voice/rooms/{id}/participants/{account_id}/cohost", params)).data;
    case "hand-over":
      return (await api.POST("/voice/rooms/{id}/host", { params: { path: { id } }, body: { account_id: accountId } }))
        .data;
    case "mute":
      await api.POST("/voice/rooms/{id}/participants/{account_id}/mute", params);
      return undefined;
    case "remove":
      return (await api.DELETE("/voice/rooms/{id}/participants/{account_id}", params)).data;
  }
}

// ---- The room's stream ------------------------------------------------------------------------------------------------

/** How long the room's stream may send nothing before it counts as dropped: three of its 15-second heartbeats. */
export const voiceStreamSilence = 45_000;

/** The waits before opening the stream again, by the failures in a row before it (the last for every later one). */
export const voiceStreamDelays: readonly number[] = [1_000, 2_000, 5_000, 10_000, 30_000];

/** The room's stream sent nothing for `voiceStreamSilence`. */
export class VoiceStreamSilentError extends Error {
  override readonly name = "VoiceStreamSilentError";
}

/** What `runVoiceStream` reads, and whom it tells. */
export type VoiceStreamOptions = {
  readonly room: string;
  readonly session: string;
  /** Ends the stream: `runVoiceStream` resolves once it aborts. */
  readonly signal: AbortSignal;
  /** The stream opened: the first time (`reconnected` false), or again after it dropped. */
  readonly onOpen?: (reconnected: boolean) => void;
  /** The stream dropped (it ended, failed or went silent), with why, and opens again after a wait. */
  readonly onDrop?: (error: unknown) => void;
  readonly onEvent: (event: VoiceStreamEvent) => void;
  /** Waits `ms`, or until `signal` aborts; a timer by default. */
  readonly sleep?: (ms: number, signal: AbortSignal) => Promise<void>;
  /** How long the stream may stay silent; `voiceStreamSilence` by default. */
  readonly silence?: number;
};

/**
 * Reads the room's stream of `session` as long as the session lasts, opening it again on the same session whenever
 * it drops (a network failure, a 5xx, a silent stream, the server closing it), after a wait that grows with the
 * failures in a row. It resolves once a `removed`, `replaced` or `ended` event was handed on, or `signal` aborted, and
 * rejects with the server's refusal of the stream (a ProblemError of a 4xx: a 404 when the server holds the session no
 * more), which opening it again would not change.
 */
export async function runVoiceStream(data: AppData, options: VoiceStreamOptions): Promise<void> {
  const { signal } = options;
  // A function, read anew after each await.
  const stopped = () => signal.aborted;
  const sleep = options.sleep ?? wait;
  let failures = 0;
  let opened = false;
  while (!stopped()) {
    let ended = false;
    try {
      ended = await readVoiceStream(data, options, () => {
        options.onOpen?.(opened);
        opened = true;
        failures = 0;
      });
      if (ended || stopped()) return;
      options.onDrop?.(null);
    } catch (error) {
      if (stopped()) return;
      if (refused(error)) throw error;
      failures++;
      options.onDrop?.(error);
    }
    const delay = voiceStreamDelays[Math.min(Math.max(failures - 1, 0), voiceStreamDelays.length - 1)] ?? 30_000;
    await sleep(delay, signal);
  }
}

/** Whether the server refused the stream for good: a 4xx but a timeout or a rate limit. */
function refused(error: unknown): boolean {
  return (
    error instanceof ProblemError && error.status >= 400 && error.status < 500 && ![408, 429].includes(error.status)
  );
}

/**
 * Reads one stream until it ends: true when it ended with `removed`, `replaced` or `ended`, false when the server
 * closed it otherwise or `signal` aborted; it rejects when it fails or stays silent.
 */
async function readVoiceStream(
  data: AppData,
  { room, session, signal, onEvent, silence = voiceStreamSilence }: VoiceStreamOptions,
  onOpen: () => void,
): Promise<boolean> {
  const quiet = new AbortController();
  let timer: ReturnType<typeof setTimeout> | undefined;
  const arm = () => {
    clearTimeout(timer);
    timer = setTimeout(() => {
      quiet.abort(new VoiceStreamSilentError(`app-data: the stream of room ${room} was silent`));
    }, silence);
  };
  const both = AbortSignal.any([signal, quiet.signal]);
  arm();
  try {
    const { data: body } = await data.api.GET("/voice/rooms/{id}/stream", {
      params: { path: { id: room }, query: { session } },
      parseAs: "stream",
      signal: both,
    });
    if (body === undefined || body === null) return false;
    onOpen();
    const reader = body.getReader();
    const stop = () => void reader.cancel().catch(() => undefined);
    both.addEventListener("abort", stop, { once: true });
    const decoder = new TextDecoder("utf-8");
    const parser = new EventStreamParser();
    const hand = (sents: readonly ServerSentEvent[]) => {
      for (const sent of sents) {
        const event = parseVoiceStreamEvent(sent.event, sent.data);
        if (event === null) continue;
        onEvent(event);
        if (event.type === "removed" || event.type === "replaced" || event.type === "ended") return true;
      }
      return false;
    };
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (signal.aborted) return false;
        if (quiet.signal.aborted) throw quiet.signal.reason as Error;
        if (done) break;
        arm();
        if (hand(parser.push(decoder.decode(value, { stream: true })))) return true;
      }
      return hand([...parser.push(decoder.decode()), ...parser.end()]);
    } finally {
      both.removeEventListener("abort", stop);
      stop();
    }
  } catch (error) {
    if (signal.aborted) return false;
    if (quiet.signal.aborted) throw quiet.signal.reason as Error;
    throw error;
  } finally {
    clearTimeout(timer);
  }
}

/** Types an event of the room's stream; null for one this client does not know or cannot read. */
export function parseVoiceStreamEvent(event: string, data: string): VoiceStreamEvent | null {
  let value: unknown;
  try {
    value = JSON.parse(data);
  } catch {
    return null;
  }
  if (typeof value !== "object" || value === null) return null;
  const v = value as Record<string, unknown>;
  switch (event) {
    case "state":
      return typeof v["id"] === "string" && Array.isArray(v["participants"])
        ? { type: "state", room: value as VoiceRoom }
        : null;
    case "mic":
      return typeof v["participant"] === "string" && typeof v["on"] === "boolean"
        ? { type: "mic", participant: v["participant"], on: v["on"] }
        : null;
    case "reaction":
      return typeof v["participant"] === "string" && typeof v["emoji"] === "string"
        ? { type: "reaction", participant: v["participant"], emoji: v["emoji"] }
        : null;
    case "signal":
      return typeof v["from"] === "string" && typeof v["kind"] === "string"
        ? { type: "signal", signal: value as Schemas["VoiceStreamSignal"] }
        : null;
    case "muted":
      return typeof v["by"] === "string" ? { type: "muted", by: v["by"] } : null;
    case "grant":
      return typeof v["grant"] === "object" && v["grant"] !== null && typeof v["reconnect"] === "boolean"
        ? { type: "grant", grant: v["grant"] as VoiceGrant, reconnect: v["reconnect"] }
        : null;
    case "removed":
    case "replaced":
    case "ended":
      return typeof v["reason"] === "string" ? { type: event, reason: v["reason"] as VoiceEndReason } : null;
    default:
      return null;
  }
}

/** Waits `ms`, or until `signal` aborts. */
function wait(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal.aborted) {
      resolve();
      return;
    }
    const timer = setTimeout(done, ms);
    signal.addEventListener("abort", done, { once: true });
    function done() {
      clearTimeout(timer);
      signal.removeEventListener("abort", done);
      resolve();
    }
  });
}
