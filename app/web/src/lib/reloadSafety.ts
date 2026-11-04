// SPDX-License-Identifier: AGPL-3.0-only
// Whether a reload the user did not ask for would lose nothing (ADR-0028 §13), in one place. What the page's DOM does not
// show registers here: the route and writes in flight (UpdatePrompt), files not yet uploaded (features/compose), and the
// call (features/voice), which the update's toast also waits for. The text of a draft is not a blocker: sessionStorage
// keeps it, and it would keep its author from ever updating.
import { useSyncExternalStore } from "react";

const blockers = new Set<() => boolean>();

/** Adds a check that answers true while a reload would lose something; returns its removal. */
export function registerReloadBlocker(blocks: () => boolean): () => void {
  blockers.add(blocks);
  return () => {
    blockers.delete(blocks);
  };
}

let callActive = false;
const callListeners = new Set<() => void>();

/** Tells whether the user is in a call; a reload would end it. */
export function reportCallActive(active: boolean): void {
  callActive = active;
  for (const listener of callListeners) listener();
}

/** Whether the user is in a call, for what waits until it ends. */
export function useCallActive(): boolean {
  return useSyncExternalStore(
    (listener) => {
      callListeners.add(listener);
      return () => callListeners.delete(listener);
    },
    () => callActive,
    () => false,
  );
}

const editable = 'input, textarea, select, [contenteditable]:not([contenteditable="false"])';
const openDialog = '[role="dialog"], [role="alertdialog"]';

/** Whether media is playing that a reload would stop. A muted loop (an animated GIF in a timeline) loses nothing. */
function isPlaying(media: HTMLMediaElement): boolean {
  return !media.paused && !media.ended && !(media.muted && media.loop);
}

/** Whether the page may reload now without the user asking. */
export function isSafeToReload(doc: Document = document): boolean {
  if (callActive) return false;
  for (const blocks of blockers) if (blocks()) return false;
  if (doc.querySelector(openDialog) !== null) return false;
  if (doc.activeElement?.matches(editable) === true) return false;
  return ![...doc.querySelectorAll<HTMLMediaElement>("video, audio")].some(isPlaying);
}
