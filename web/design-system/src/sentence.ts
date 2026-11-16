// SPDX-License-Identifier: AGPL-3.0-only

// How a line's text meets the action after it (a caption's Undo, a notice's Try again): a middle dot after words, held
// to the last word by a space that never breaks; none after a full sentence, whose stop already ends it. Then the stop
// goes on the line of the action, inside its unbreakable run, so that the action never starts a line of its own (a
// word joiner alone does not hold it where a ja line wraps by phrase): after a Latin stop with a space that never
// breaks, after a full-width one with none, since it brings its own room.

import { createElement, Fragment, type ReactNode } from "react";

const stops = ".!?。！？";
const wide = "。！？";

/** A middle dot held to the last word, so that no line starts with it. */
export const actionDot = " · ";

/**
 * `text` with its final stop taken off and handed to the action's run: `[the text before it, the stop and its space]`;
 * undefined when the text is no string ending a sentence (an array ending in one counts, its last string split).
 */
export function splitStop(text: ReactNode): [ReactNode, string] | undefined {
  if (typeof text === "string") {
    const trimmed = text.trimEnd();
    const stop = trimmed.at(-1);
    if (stop === undefined || !stops.includes(stop)) return undefined;
    return [trimmed.slice(0, -1), wide.includes(stop) ? stop : `${stop} `];
  }
  if (Array.isArray(text) && text.length > 0) {
    const last = splitStop(text.at(-1) as ReactNode);
    if (last === undefined) return undefined;
    // Spread as the fragment's children, as they were written, so that they need no keys.
    return [createElement(Fragment, null, ...(text.slice(0, -1) as ReactNode[]), last[0]), last[1]];
  }
  return undefined;
}
