// SPDX-License-Identifier: AGPL-3.0-only
// The call the reader is in (ADR-0101 §7, ADR-0028 §18): state that outlives every screen, which the shell shows in its
// call slot while the reader moves between screens. The voice feature joins a room and carries its audio, and fills
// this store as it does; the shell reads it, mutes and leaves through it. One call at a time (FR-924).
import { useSyncExternalStore } from "react";

/** The call the reader is in: the room, who hosts it, how many are in it, and whether the reader's microphone is off. */
export type Call = {
  readonly roomId: string;
  readonly room: string;
  readonly host: string;
  readonly participants: number;
  readonly muted: boolean;
};

/** What the voice feature does when the reader mutes or leaves from the shell's bar. */
export type CallControls = {
  readonly setMuted?: (muted: boolean) => void;
  readonly leave?: () => void;
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

/** The call changed (its participants, the microphone). */
export function updateCall(change: Partial<Omit<Call, "roomId">>): void {
  if (current !== null) changed({ ...current, ...change });
}

/** Turns the reader's microphone off or on. */
export function muteCall(muted: boolean): void {
  controls.setMuted?.(muted);
  updateCall({ muted });
}

/** The reader leaves the call. */
export function leaveCall(): void {
  const leave = controls.leave;
  controls = {};
  changed(null);
  leave?.();
}

/** The call the reader is in, or null. */
export function useCall(): Call | null {
  return useSyncExternalStore(
    subscribe,
    () => current,
    () => null,
  );
}
