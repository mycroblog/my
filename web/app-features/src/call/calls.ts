// SPDX-License-Identifier: AGPL-3.0-only
// The reader's call (ADR-0101 §7, FR-924): one at a time, owned here, at the module's level, outside React, so that no
// screen's mount or unmount, no render, no change of route or of layout starts or ends it. A screen joins a room
// (`joinCall`), reads the call with `useVoiceCall` / `useVoiceCallState`, and acts on it through the VoiceCall; the
// shell's bar reads the summary this module keeps in the call's store (store.ts).
//
// A call ends once, by the first of: leaving; the stream's `removed`, `replaced` or `ended`; the server dropping the
// session (a heartbeat or a stream answered 404, or no heartbeat through for 45 seconds); the audio failing; the
// session of the app ending (signed out here or in another tab); joining another room; the page going away
// (`pagehide`, which sends the leave with the page's last request). While a call lasts, the browser asks before the
// page unloads (a reload, a link outside the app), and the app does not load the page anew by itself (`inCall`).
import {
  joinVoiceRoom,
  leaveVoiceRoom,
  openVoiceRoom,
  type AppData,
  type ServerTime,
  type VoiceJoin,
  type VoiceRoomSettings,
} from "@my/app-data";
import { holdUnload, mediaDevices, watchOnline, watchPageHide } from "@my/platform";
import { browserEnvironment, createRoomSession, type RoomEnvironment } from "@my/rtc";
import { useSyncExternalStore } from "react";
import { endCall, enterCall, updateCall, type Call, type CallControls } from "./store";
import { systemClock, VoiceCall, type CallEnd, type VoiceCallRuntime, type VoiceCallState } from "./voice-call";

/** Where a call runs: its audio, the environment of the audio, its clock, and the page it outlives screens in. */
export type CallRuntime = VoiceCallRuntime & {
  /**
   * Makes the environment of the audio; called at once as the reader joins, inside their gesture, which lets the
   * audio play. The browser's by default.
   */
  readonly environment: () => RoomEnvironment;
  /** Calls its listener as the page goes away, until the function it returns is called. */
  readonly watchPageHide: (listener: () => void) => () => void;
  /** Has the browser ask before the page unloads, until the function it returns is called. */
  readonly holdUnload: () => () => void;
};

/** The browser's media devices were not given (outside HTTPS): no call can be joined. */
export class CallUnavailableError extends Error {
  override readonly name = "CallUnavailableError";
}

const browserRuntime: CallRuntime = {
  createSession: createRoomSession,
  clock: systemClock,
  environment: () => {
    const devices = mediaDevices();
    if (devices === undefined) throw new CallUnavailableError("app-features: the browser gives no media devices");
    return browserEnvironment({ devices, watchNetwork: (onChange) => watchOnline(onChange) });
  },
  watchNetwork: (onOnline) =>
    watchOnline((online) => {
      if (online) onOnline();
    }),
  watchPageHide,
  holdUnload,
};

let runtime: CallRuntime = browserRuntime;
let current: VoiceCall | null = null;
let currentState: VoiceCallState | null = null;
let lastEnd: CallEnd | null = null;
/** The join on its way, and its room: one at a time. */
let joining: { readonly roomId: string | null; readonly call: Promise<VoiceCall> } | null = null;
const listeners = new Set<() => void>();

const notify = () => {
  for (const listener of [...listeners]) listener();
};

const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};

/**
 * Runs the calls on `given` in place of what it names (a native RoomSession's factory, a test's fakes), until the
 * function it returns is called.
 */
export function runCallsOn(given: Partial<CallRuntime>): () => void {
  const before = runtime;
  runtime = { ...runtime, ...given };
  return () => {
    runtime = before;
  };
}

/** The summary of `state` the shell's bar shows. */
function summaryOf({ room, self, micWanted, phase, connection, playbackBlocked }: VoiceCallState): Call {
  return {
    roomId: room.id,
    room: room.name,
    host: room.host.display_name || room.host.username,
    participants: room.participants_count,
    muted: !micWanted,
    speaks: self?.role === "speaker",
    phase:
      phase === "connecting"
        ? "connecting"
        : phase === "reconnecting" || connection === "reconnecting" || connection === "lost"
          ? "reconnecting"
          : "live",
    playbackBlocked,
  };
}

/**
 * Joins the room `roomId` on the session of `data` and makes it the reader's call, which it resolves with; call it
 * from the reader's gesture (the join button), which lets the audio play. The call of that room, or the join on its
 * way to it, when there is one already, so that a join asked twice joins once; a call of another room ends once the
 * server took the join (`switched`: the server left that room), and stays when it refused it. It rejects with the
 * server's refusal (a room that is full, a host of another open room), or a CallUnavailableError.
 */
export function joinCall(data: AppData, roomId: string): Promise<VoiceCall> {
  if (current !== null && current.roomId === roomId && !current.ended) return Promise.resolve(current);
  if (joining !== null && joining.roomId === roomId) return joining.call;
  return begin(data, roomId, () => joinVoiceRoom(data, roomId));
}

/**
 * Opens a room with `settings` on the session of `data`, which the reader hosts and is joined to as a speaker, and
 * makes it the reader's call, which it resolves with; call it from the reader's gesture (the form's Open), as
 * `joinCall`. A call of another room ends once the server opened the room (it left that room), and stays when it
 * refused (a reader who may not open rooms, too many open, a host of another open room, settings it does not take).
 */
export function openCall(data: AppData, settings: VoiceRoomSettings): Promise<VoiceCall> {
  return begin(data, null, () => openVoiceRoom(data, settings));
}

/** Joins by `request` (a join of `roomId`, or an opening of a room whose ID the server gives), as `joinCall` says. */
function begin(
  data: AppData,
  roomId: string | null,
  request: () => Promise<VoiceJoin & ServerTime>,
): Promise<VoiceCall> {
  const before = joining?.call.catch(() => undefined);
  const given = runtime;
  let environment: RoomEnvironment;
  try {
    // Inside the reader's gesture: the audio's context starts there, or stays blocked until another.
    environment = given.environment();
  } catch (error) {
    return Promise.reject(error instanceof Error ? error : new Error(String(error)));
  }
  const call = (async () => {
    await before;
    const leaving = current;
    leaving?.switching(true);
    let join;
    try {
      join = await request();
    } catch (error) {
      leaving?.switching(false);
      environment.close?.();
      throw error;
    }
    let made: VoiceCall;
    try {
      made = new VoiceCall(data, join, environment, given);
    } catch (error) {
      // A grant this client cannot connect with: the session the server made goes again, and the room left stays left.
      leaving?.switched();
      environment.close?.();
      void leaveVoiceRoom(data, join.room.id, join.session).catch(() => undefined);
      throw error;
    }
    leaving?.switched();
    hold(made, given);
    made.start();
    return made;
  })();
  const entry = { roomId, call };
  joining = entry;
  const done = () => {
    if (joining === entry) joining = null;
  };
  call.then(done, done);
  return call;
}

/** Makes `call` the reader's call: the shell's bar shows it, the page's going away ends it, until it ends. */
function hold(call: VoiceCall, given: CallRuntime): void {
  current = call;
  currentState = call.getState();
  lastEnd = null;
  const unwatch = call.subscribe(() => {
    if (current !== call) return;
    const state = call.getState();
    currentState = state;
    if (state.end === null) updateCall(summaryOf(state));
    notify();
  });
  const release = given.holdUnload();
  const unhide = given.watchPageHide(() => {
    call.unload();
  });
  call.onEnd((end) => {
    unwatch();
    release();
    unhide();
    if (current !== call) return;
    current = null;
    currentState = null;
    lastEnd = end;
    endCall();
    notify();
  });
  enterCall(summaryOf(call.getState()), controlsOf(call));
}

/** What the shell's bar does to `call`. */
function controlsOf(call: VoiceCall): CallControls {
  return {
    setMuted: (muted: boolean) => {
      call.setMicrophone(!muted).catch(() => undefined);
    },
    leave: () => {
      call.leave().catch(() => {
        // The server kept the reader in (a host who must choose who hosts next): the bar comes back.
        if (current === call) enterCall(summaryOf(call.getState()), controlsOf(call));
      });
    },
    resumePlayback: () => {
      call.resumePlayback().catch(() => undefined);
    },
  };
}

/** The reader's call, or null. */
export function currentCall(): VoiceCall | null {
  return current;
}

/** Whether the reader is in a call: the app does not load the page anew by itself meanwhile. */
export function inCall(): boolean {
  return current !== null;
}

/** Why the reader's last call ended, until another starts; null while in one, or before any. */
export function lastCallEnd(): CallEnd | null {
  return lastEnd;
}

/**
 * Leaves the reader's call, if any, before the session of the app ends (signing out): the leave is sent while it
 * still can be, also for a join on its way, which is awaited first. A host whom the server keeps in (they did not
 * choose who hosts next) stays until the session's end ends the call.
 */
export async function leaveCallBeforeSignOut(): Promise<void> {
  const pending = joining?.call;
  if (pending !== undefined) await pending.catch(() => undefined);
  await current?.leave().catch(() => undefined);
}

/** The reader's call, or null; the same VoiceCall for the whole call. */
export function useVoiceCall(): VoiceCall | null {
  return useSyncExternalStore(subscribe, currentCall, () => null);
}

/** The state of the reader's call, or null while they are in none. */
export function useVoiceCallState(): VoiceCallState | null {
  return useSyncExternalStore(
    subscribe,
    () => currentState,
    () => null,
  );
}

/** Why the reader's last call ended, until another starts. */
export function useLastCallEnd(): CallEnd | null {
  return useSyncExternalStore(subscribe, lastCallEnd, () => null);
}
