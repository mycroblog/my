// SPDX-License-Identifier: AGPL-3.0-only

// How a line's text meets the action after it (a caption's Undo, a notice's Try again): a middle dot after words, held
// to the last word by a space that never breaks; none after a full sentence, whose stop already ends it. The stop
// stays with its sentence and the line may wrap after it, the action never broken inside: a Latin stop takes a space
// before the action, a full-width one brings its own room. Holding the stop with the action instead pushed the whole
// last phrase of a ja sentence to the next line.

import type { ReactNode } from "react";

const stops = ".!?。！？";
const wide = "。！？";

/** A middle dot held to the last word, so that no line starts with it. */
const actionDot = " · ";

/** The last character of the text of `node`, where the text is strings (an array of them, or of nodes ending in one). */
function lastChar(node: ReactNode): string | undefined {
  if (typeof node === "string") return node.trimEnd().at(-1);
  if (Array.isArray(node)) {
    for (let i = node.length - 1; i >= 0; i--) {
      const c = lastChar(node[i] as ReactNode);
      if (c !== undefined) return c;
    }
  }
  return undefined;
}

/** The joint between `text` and an action after it: the held dot after words; after a stop, a space or nothing. */
export function actionJoint(text: ReactNode): string {
  const c = lastChar(text);
  if (c === undefined || !stops.includes(c)) return actionDot;
  return wide.includes(c) ? "" : " ";
}
