// SPDX-License-Identifier: AGPL-3.0-only

// The events between the manager's notes (manager.tsx: the tool and the Notes panel beside the canvas) and the
// preview's overlay (overlay.tsx: picking, and the markers on the story), over Storybook's channel.

import type { NoteInput } from "./note";

export const events = {
  /** manager → preview: start (true) or stop (false) picking an element or an area. */
  pick: "my-devtools/pick",
  /** preview → manager: the note on what was picked, without its text; null when the decider gave up (Escape). */
  picked: "my-devtools/picked",
  /** manager → preview: the markers of the notes to show on the page; none hides them all. */
  markers: "my-devtools/markers",
  /** preview → manager: the <Options> on the page. */
  options: "my-devtools/options",
  /** manager → preview: show a variant of an <Options>: {id, variant}. */
  switch: "my-devtools/switch",
  /** manager → preview: pick the variant shown of an <Options>: {id, reason}. */
  pickVariant: "my-devtools/pick-variant",
  /** preview → manager: the note of that pick. */
  variantNote: "my-devtools/variant-note",
  /** preview → manager: the dev server says a batch changed (written or answered). */
  update: "my-devtools/update",
  /** preview → manager: the overlay is on a new page and wants the state again. */
  ready: "my-devtools/ready",
} as const;

/** A note's marker on the page: its number in the panel's list and its color (not sent, sent, answered). */
export interface Marker {
  key: string;
  label: string;
  color: string;
  note: NoteInput;
}

/** An <Options> on the page: its id, the variant shown, and every variant. */
export interface OptionSet {
  id: string;
  variant: string;
  variants: string[];
}
