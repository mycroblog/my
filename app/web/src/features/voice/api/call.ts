// SPDX-License-Identifier: AGPL-3.0-only
// The requests of a call (ADR-0101 §3, §6; ADR-0100 §5, §6, §10): the room stream of the user's session
// (GET /voice/rooms/{id}/stream), its heartbeats, a new grant, the signals of P2P, the microphone, reactions, leaving,
// speaking and listening, raising a hand, and the operations of the host and the co-hosts on a participant, with the
// invitations. The room stream is read here with fetch (EventSource cannot send the access token), like the app's
// stream (lib/sse): one per session, which the hooks open again with the same session when it ends.
import { EventStreamParser, type components } from "@mycroblog/mykit";
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError, NetworkError } from "../../../lib/errors";
import { type VoiceRoom, voiceRoomQueryOptions } from "./voice";

type Schemas = components["schemas"];

export type VoiceGrantData = Schemas["VoiceGrant"];
export type VoiceRole = Schemas["VoiceRole"];
export type VoiceSignalData = Omit<Schemas["VoiceSignal"], "session">;

/** An event of the room stream, its data parsed; the stream ends after `removed`, `replaced`, and `ended`. */
export type VoiceStreamEvent =
  | { type: "state"; room: VoiceRoom }
  | { type: "mic"; mic: Schemas["VoiceStreamMic"] }
  | { type: "reaction"; reaction: Schemas["VoiceStreamReaction"] }
  | { type: "signal"; signal: Schemas["VoiceStreamSignal"] }
  | { type: "muted"; muted: Schemas["VoiceStreamMuted"] }
  | { type: "grant"; grant: Schemas["VoiceStreamGrant"] }
  | { type: "removed" | "replaced" | "ended"; reason: string };

/** How long the room stream may stay silent: it sends a heartbeat every 15 seconds. */
const voiceStreamSilence = 45_000;

/** The room stream sent nothing for voiceStreamSilence. */
class VoiceStreamSilentError extends Error {
  override name = "VoiceStreamSilentError";
}

/** Types an event of the room stream; null for one that this client does not know or cannot read. */
function parseVoiceStreamEvent(event: string, data: string): VoiceStreamEvent | null {
  let value: unknown;
  try {
    value = JSON.parse(data);
  } catch {
    return null;
  }
  if (typeof value !== "object" || value === null) return null;
  switch (event) {
    case "state":
      return { type: "state", room: value as VoiceRoom };
    case "mic":
      return { type: "mic", mic: value as Schemas["VoiceStreamMic"] };
    case "reaction":
      return { type: "reaction", reaction: value as Schemas["VoiceStreamReaction"] };
    case "signal":
      return { type: "signal", signal: value as Schemas["VoiceStreamSignal"] };
    case "muted":
      return { type: "muted", muted: value as Schemas["VoiceStreamMuted"] };
    case "grant":
      return { type: "grant", grant: value as Schemas["VoiceStreamGrant"] };
    case "removed":
    case "replaced":
    case "ended":
      return { type: event, reason: (value as Schemas["VoiceStreamEnd"]).reason };
    default:
      return null;
  }
}

export type VoiceStreamOptions = {
  signal: AbortSignal;
  /** The stream opened. */
  onOpen: () => void;
  onEvent: (event: VoiceStreamEvent) => void;
};

/**
 * Reads the room stream of `session` until it ends: it resolves when the server ends it (or `signal` aborts) and
 * rejects with the ApiError of a refused stream (a 404 for a session that is no longer the user's), a network
 * failure, or a VoiceStreamSilentError.
 */
export async function readVoiceStream(
  api: Api,
  id: string,
  session: string,
  { signal, onOpen, onEvent }: VoiceStreamOptions,
): Promise<void> {
  const silence = new AbortController();
  let timer: ReturnType<typeof setTimeout> | undefined;
  const arm = () => {
    clearTimeout(timer);
    timer = setTimeout(() => {
      silence.abort(new VoiceStreamSilentError("voice: the room stream was silent"));
    }, voiceStreamSilence);
  };
  const both = AbortSignal.any([signal, silence.signal]);
  arm();
  try {
    const { data } = await api.fetch.GET("/voice/rooms/{id}/stream", {
      params: { path: { id }, query: { session } },
      parseAs: "stream",
      signal: both,
    });
    if (data === undefined || data === null) return;
    onOpen();
    const reader = data.getReader();
    const stop = () => void reader.cancel().catch(() => undefined);
    both.addEventListener("abort", stop, { once: true });
    const decoder = new TextDecoder("utf-8");
    const parser = new EventStreamParser();
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (signal.aborted) return;
        if (silence.signal.aborted) throw silence.signal.reason as Error;
        if (done) break;
        arm();
        for (const event of parser.push(decoder.decode(value, { stream: true }))) {
          const parsed = parseVoiceStreamEvent(event.event, event.data);
          if (parsed !== null) onEvent(parsed);
        }
      }
      for (const event of [...parser.push(decoder.decode()), ...parser.end()]) {
        const parsed = parseVoiceStreamEvent(event.event, event.data);
        if (parsed !== null) onEvent(parsed);
      }
    } finally {
      both.removeEventListener("abort", stop);
    }
  } catch (error) {
    if (signal.aborted) return;
    if (silence.signal.aborted) throw silence.signal.reason as Error;
    throw error;
  } finally {
    clearTimeout(timer);
  }
}

/** Whether `error` says that the session is no longer the user's in the room (or the room is gone): join again. */
export function isSessionGone(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404;
}

/**
 * How long to wait before sending a signal again after `error`, the failure of try `attempt` (from 1): the time
 * that a 429 names (a session sends at most 20 signals a second), or a growing wait after a network failure, three
 * times at most. Null for a failure that sending again does not cure (a 422 of a signal that the relay refuses).
 */
export function signalRetryDelay(error: unknown, attempt: number, now: number): number | null {
  if (attempt > 3) return null;
  if (error instanceof ApiError && error.status === 429) return Math.max((error.retryAt ?? now + 1_000) - now, 100);
  if (error instanceof NetworkError) return 500 * 2 ** (attempt - 1);
  return null;
}

/** Tells the server that the session is alive (every 15 seconds). */
export async function sendVoiceHeartbeat(api: Api, id: string, session: string): Promise<void> {
  await api.fetch.POST("/voice/rooms/{id}/heartbeat", { params: { path: { id } }, body: { session } });
}

/** A new grant for the session, 10 minutes before the one it has expires. */
export async function refreshVoiceGrant(api: Api, id: string, session: string): Promise<VoiceGrantData> {
  const { data, error } = await api.fetch.POST("/voice/rooms/{id}/grant", {
    params: { path: { id } },
    body: { session },
  });
  if (data === undefined) throw new ApiError(error);
  return data;
}

/** Sends a signal of P2P to another session of the room. */
export async function sendVoiceSignal(api: Api, id: string, session: string, signal: VoiceSignalData): Promise<void> {
  await api.fetch.POST("/voice/rooms/{id}/signals", { params: { path: { id } }, body: { session, ...signal } });
}

/** Tells the participants that the user's microphone is on or off. */
export async function setVoiceMic(api: Api, id: string, session: string, on: boolean): Promise<void> {
  await api.fetch.POST("/voice/rooms/{id}/mic", { params: { path: { id } }, body: { session, on } });
}

/** Sends a reaction (a Unicode emoji or a custom emoji's `:shortcode:`) to the participants. */
export async function reactInVoiceRoom(api: Api, id: string, emoji: string): Promise<void> {
  await api.fetch.POST("/voice/rooms/{id}/reactions", { params: { path: { id } }, body: { emoji } });
}

/** How the host leaves a room with other participants: handing it to the next host, or ending it. */
export type VoiceLeaveChoice = { nextHostId: string } | { end: true } | undefined;

/** Leaves the room; the host of a room with others in it chooses the next host or ends it. */
export async function leaveVoiceRoom(api: Api, id: string, choice: VoiceLeaveChoice): Promise<void> {
  const query = choice === undefined ? {} : "end" in choice ? { end: true } : { next_host_id: choice.nextHostId };
  await api.fetch.DELETE("/voice/rooms/{id}/participation", { params: { path: { id }, query } });
}

/**
 * Leaves the room as the page goes away (pagehide): the request outlives the page (keepalive). The host of a room with
 * others in it is refused (the server's 409) and stays the host until the grace after the heartbeats stop.
 */
export function leaveVoiceRoomOnUnload(api: Api, id: string): void {
  void api.fetch
    .DELETE("/voice/rooms/{id}/participation", { params: { path: { id } }, keepalive: true })
    .catch(() => undefined);
}

/** The user's own operations in a room, each answering the room after it. */
export type VoiceSelfOperation = "speak" | "listen" | "raise-hand" | "lower-hand";

export async function voiceSelfOperation(api: Api, id: string, operation: VoiceSelfOperation): Promise<VoiceRoom> {
  const params = { params: { path: { id } } };
  const { data, error } =
    operation === "speak"
      ? await api.fetch.POST("/voice/rooms/{id}/speaker", params)
      : operation === "listen"
        ? await api.fetch.DELETE("/voice/rooms/{id}/speaker", params)
        : operation === "raise-hand"
          ? await api.fetch.POST("/voice/rooms/{id}/hand", params)
          : await api.fetch.DELETE("/voice/rooms/{id}/hand", params);
  if (data === undefined) throw new ApiError(error);
  return data;
}

/**
 * An operation of the host or a co-host on a participant: let speak (`grant`), dismiss the raised hand, stop speaking
 * (`revoke`), mute, remove; and of the host only: make or stop being a co-host, and hand the room over.
 */
export type VoiceParticipantOperation =
  "grant" | "dismiss" | "revoke" | "mute" | "remove" | "cohost" | "uncohost" | "hand-over";

/** Runs `operation` on the participant `accountId`; the room after it, when the server answers one. */
export async function voiceParticipantOperation(
  api: Api,
  id: string,
  accountId: string,
  operation: VoiceParticipantOperation,
): Promise<VoiceRoom | undefined> {
  const params = { params: { path: { id, account_id: accountId } } };
  switch (operation) {
    case "grant": {
      const { data, error } = await api.fetch.POST("/voice/rooms/{id}/participants/{account_id}/grant", params);
      if (data === undefined) throw new ApiError(error);
      return data;
    }
    case "revoke": {
      const { data, error } = await api.fetch.DELETE("/voice/rooms/{id}/participants/{account_id}/grant", params);
      if (data === undefined) throw new ApiError(error);
      return data;
    }
    case "dismiss": {
      const { data, error } = await api.fetch.DELETE("/voice/rooms/{id}/participants/{account_id}/hand", params);
      if (data === undefined) throw new ApiError(error);
      return data;
    }
    case "cohost": {
      const { data, error } = await api.fetch.POST("/voice/rooms/{id}/participants/{account_id}/cohost", params);
      if (data === undefined) throw new ApiError(error);
      return data;
    }
    case "uncohost": {
      const { data, error } = await api.fetch.DELETE("/voice/rooms/{id}/participants/{account_id}/cohost", params);
      if (data === undefined) throw new ApiError(error);
      return data;
    }
    case "hand-over": {
      const { data, error } = await api.fetch.POST("/voice/rooms/{id}/host", {
        params: { path: { id } },
        body: { account_id: accountId },
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    }
    case "mute":
      await api.fetch.POST("/voice/rooms/{id}/participants/{account_id}/mute", params);
      return undefined;
    case "remove":
      await api.fetch.DELETE("/voice/rooms/{id}/participants/{account_id}", params);
      return undefined;
  }
}

/** Invites the local user `username` to the room as a listener or a speaker: finds the account, then invites it. */
export async function inviteToVoiceRoom(
  api: Api,
  id: string,
  username: string,
  role: VoiceRole,
): Promise<Schemas["VoiceRoomInvitation"]> {
  const found = await api.fetch.GET("/accounts/lookup", { params: { query: { acct: username.trim() } } });
  if (found.data === undefined) throw new ApiError(found.error);
  const { data, error } = await api.fetch.POST("/voice/rooms/{id}/invitations", {
    params: { path: { id } },
    body: { account_id: found.data.id, role },
  });
  if (data === undefined) throw new ApiError(error);
  return data;
}

/** Puts the room as the room stream or a write has it into its query (ADR-0045). */
export function cacheVoiceRoomState(api: Api, queryClient: QueryClient, room: VoiceRoom): void {
  queryClient.setQueryData(voiceRoomQueryOptions(api, room.id).queryKey, room);
}

/** Marks the room stale after the user left it, it ended, or the user was removed: the page reads it again. */
export async function refreshVoiceRoom(api: Api, queryClient: QueryClient, id: string): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: voiceRoomQueryOptions(api, id).queryKey }),
    queryClient.invalidateQueries({
      predicate: (query) => query.queryKey[0] === "get" && query.queryKey[1] === "/voice/rooms",
    }),
  ]);
}
