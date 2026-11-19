// SPDX-License-Identifier: AGPL-3.0-only
// Why the reader's call in a room ended, by the room (ADR-0100 §5, §6): the screens join through `joinRoom`, which
// keeps the end of each call with its room, so that the room's page says why the reader is no longer in it (a host
// removed them, they joined on another device, the room ended) and the shell says it on every other screen. What the
// reader did themselves (leaving, joining another room, signing out, leaving the page) says nothing. Kept until the
// reader joins a room again or closes what said it; once the room's page said it, the shell says it no more.
import type { AppData } from "@my/app-data";
import { useSyncExternalStore } from "react";
import { joinCall } from "./calls";
import type { CallEnd, VoiceCall } from "./voice-call";

/**
 * The end of the reader's last call, with its room's ID and name, which the reader is to be told; `seen` once the
 * room's page told them.
 */
export type RoomEnd = { readonly roomId: string; readonly room: string; readonly end: CallEnd; readonly seen: boolean };

let last: RoomEnd | null = null;
const watched = new WeakSet<VoiceCall>();
const listeners = new Set<() => void>();

const changed = (now: RoomEnd | null) => {
  last = now;
  for (const listener of [...listeners]) listener();
};

const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};

/** Whether the reader is to be told of `end`: what ended the call was not their own doing. */
function toldOf(end: CallEnd): boolean {
  return !["left", "switched", "signed_out", "unloaded"].includes(end.kind);
}

/**
 * Joins the room `roomId` from the reader's gesture (`joinCall`), and keeps why that call ends with its room; what
 * the last call's end said goes as the join starts.
 */
export async function joinRoom(data: AppData, roomId: string): Promise<VoiceCall> {
  if (last !== null) changed(null);
  const call = await joinCall(data, roomId);
  if (!watched.has(call)) {
    watched.add(call);
    let name = call.getState().room.name;
    const unwatch = call.subscribe(() => {
      name = call.getState().room.name;
    });
    call.onEnd((end) => {
      unwatch();
      if (toldOf(end)) changed({ roomId: call.roomId, room: name, end, seen: false });
    });
  }
  return call;
}

/** The end the reader is to be told of, of the room `roomId` (of any room without one); null for none. */
export function useRoomEnd(roomId?: string): RoomEnd | null {
  const end = useSyncExternalStore(
    subscribe,
    () => last,
    () => null,
  );
  return end !== null && (roomId === undefined || end.roomId === roomId) ? end : null;
}

/** The room's page told the reader of the end of its call: the shell's toast goes, and does not come again. */
export function sawRoomEnd(roomId: string): void {
  if (last !== null && last.roomId === roomId && !last.seen) changed({ ...last, seen: true });
}

/** The reader closed what told them of the end. */
export function dismissRoomEnd(): void {
  if (last !== null) changed(null);
}
