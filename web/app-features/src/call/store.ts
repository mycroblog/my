// SPDX-License-Identifier: AGPL-3.0-only
// The call the reader is in (ADR-0101 §7, ADR-0028 §18): state that outlives every screen, which the shell shows in its
// call slot while the reader moves between screens. The call controller (calls.ts) joins a room and carries its audio,
// and fills this store as it does; the shell reads it, mutes, resumes the audio and leaves through it. One call at a
// time (FR-924).
import { useSyncExternalStore } from "react";

/**
 * The call the reader is in: the room, who hosts it, how many are in it, whether the reader speaks in it (a speaker
 * when absent) and whether their microphone is off; whether its stream is still `connecting` or `reconnecting` (live
 * when absent), and whether the browser blocked its audio until a gesture.
 */
export type Call = {
  readonly roomId: string;
  readonly room: string;
  readonly host: string;
  readonly participants: number;
  readonly speaks?: boolean;
  readonly muted: boolean;
  readonly phase?: "connecting" | "live" | "reconnecting";
  readonly playbackBlocked?: boolean;
};

/** What the call does when the reader mutes, resumes the audio or leaves from the shell's bar. */
export type CallControls = {
  readonly setMuted?: (muted: boolean) => void;
  readonly leave?: () => void;
  readonly resumePlayback?: () => void;
};

let current: Call | null = null;
let controls: CallControls = {};
const listeners = new Set<() => void>();

const changed = (call: Call | null) => {
  current = call;
  for (const listener of listeners) listener();
};

const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};

/** The reader is in `call` now, which `callControls` mute and leave; it replaces any other. */
export function enterCall(call: Call, callControls: CallControls = {}): void {
  controls = callControls;
  changed(call);
}

/** The call changed (its participants, the microphone); nothing is told when nothing the bar shows changed. */
export function updateCall(change: Partial<Omit<Call, "roomId">>): void {
  if (current === null) return;
  const next = { ...current, ...change };
  const keys = Object.keys(next) as (keyof Call)[];
  if (keys.every((key) => next[key] === current?.[key])) return;
  changed(next);
}

/** Turns the reader's microphone off or on. */
export function muteCall(muted: boolean): void {
  controls.setMuted?.(muted);
  updateCall({ muted });
}

/** Plays the call's audio that the browser blocked, from the reader's gesture. */
export function resumeCallPlayback(): void {
  controls.resumePlayback?.();
}

/** The reader leaves the call. */
export function leaveCall(): void {
  const leave = controls.leave;
  controls = {};
  changed(null);
  leave?.();
}

/** The call ended by itself (the room ended, the reader was removed): the bar goes, and nothing is told. */
export function endCall(): void {
  controls = {};
  if (current !== null) changed(null);
}

/** The call the shell's bar shows now, or null; outside React. */
export function callShown(): Call | null {
  return current;
}

/** The call the reader is in, or null. */
export function useCall(): Call | null {
  return useSyncExternalStore(
    subscribe,
    () => current,
    () => null,
  );
}
