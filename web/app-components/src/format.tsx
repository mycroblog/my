// SPDX-License-Identifier: AGPL-3.0-only

// A sentence of the catalog with parts drawn by React in its arguments' places: a name with its custom emoji and its
// isolation, a list of names.

import { Fragment, type ReactNode } from "react";
import type { stringsIn } from "./locale";
import type { MessageId } from "./messages.gen";

type T = ReturnType<typeof stringsIn>["t"];

// The first of the characters no text holds (the private use area), each standing for a part drawn by React inside a
// formatted string.
const firstSlot = 0xe000;
const slotPattern = /([-])/u;

/**
 * The string `id` with each of `parts` drawn by React in its argument's place, so that a name keeps its custom emoji
 * and its isolation inside a sentence, and no transform of the message (a pseudo-locale's) reaches it. The words
 * beside a part named in `held` keep to it, and `values` are the arguments the message reads as they are (a count).
 */
export function format(
  t: T,
  id: MessageId,
  parts: Readonly<Record<string, ReactNode>>,
  held: readonly string[] = [],
  values: Readonly<Record<string, string | number>> = {},
): ReactNode {
  const names = Object.keys(parts);
  const text = t(id, {
    ...values,
    ...Object.fromEntries(names.map((name, i) => [name, String.fromCodePoint(firstSlot + i)])),
  });
  const pieces = text.split(slotPattern);
  const nameAt = (slot: string | undefined) =>
    slot === undefined ? undefined : names[(slot.codePointAt(0) ?? firstSlot) - firstSlot];
  return pieces.map((piece, i) => {
    // The words beside a held part keep to it ("by LibreTranslate", "LibreTranslate が"): the spaces do not break.
    if (i % 2 === 0) {
      const before = held.includes(nameAt(pieces[i + 1]) ?? "") ? piece.replace(/ $/u, " ") : piece;
      return held.includes(nameAt(pieces[i - 1]) ?? "") ? before.replace(/^ /u, " ") : before;
    }
    const name = names[(piece.codePointAt(0) ?? firstSlot) - firstSlot] ?? "";
    return <Fragment key={i}>{parts[name]}</Fragment>;
  });
}
