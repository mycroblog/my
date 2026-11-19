// SPDX-License-Identifier: AGPL-3.0-only
// The host's Leave from the call's bar of another screen (ADR-0100 §6): a host with others in the room chooses who
// hosts next or ends it, which the room's page asks. The bar leads there (`askLeave`), and the page opens its leave
// dialog while the ask stands, until the dialog closes (`clearLeaveAsk`).
import { useSyncExternalStore } from "react";
import { currentCall } from "./calls";
import { hostsOthers } from "./host";

let asked: string | null = null;
const listeners = new Set<() => void>();

const changed = (now: string | null) => {
  asked = now;
  for (const listener of [...listeners]) listener();
};

const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};

/**
 * Whether the reader's Leave from the call's bar is to be asked on the room's page: they host the call's room with
 * others in it. The room's ID when it is, which the bar then leads to; null otherwise, and the bar leaves at once.
 */
export function leaveNeedsChoice(): string | null {
  const state = currentCall()?.getState();
  return state !== undefined && state.end === null && hostsOthers(state.room, state.self) ? state.roomId : null;
}

/** The bar asks the room `roomId`'s page to ask the host who hosts next, or to end the room. */
export function askLeave(roomId: string): void {
  changed(roomId);
}

/** The leave dialog closed: the ask is answered. */
export function clearLeaveAsk(): void {
  if (asked !== null) changed(null);
}

/** Whether the bar asked the page of the room `roomId` to ask who hosts next. */
export function useLeaveAsk(roomId: string): boolean {
  return (
    useSyncExternalStore(
      subscribe,
      () => asked,
      () => null,
    ) === roomId
  );
}
