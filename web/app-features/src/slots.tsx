// SPDX-License-Identifier: AGPL-3.0-only

// A message of the catalog with parts drawn by React in its arguments' places (an account's name, a relative time),
// so that the words around them keep the order of the reader's language.

import { Fragment, type ReactNode } from "react";
import type { stringsIn } from "./locale";
import type { MessageId } from "./messages.gen";

type T = ReturnType<typeof stringsIn>["t"];

// The first of the characters no text holds (the private use area), each standing for a part drawn by React inside a
// formatted string.
const firstSlot = 0xe000;
const slotPattern = /([\u{E000}-\u{F8FF}])/u;

/** The string `id` with `values`, and each of `parts` drawn by React in its argument's place. */
export function formatParts(
  t: T,
  id: MessageId,
  values: Readonly<Record<string, string | number>>,
  parts: Readonly<Record<string, ReactNode>>,
): ReactNode {
  const names = Object.keys(parts);
  const slots = Object.fromEntries(names.map((name, i) => [name, String.fromCodePoint(firstSlot + i)]));
  return t(id, { ...values, ...slots })
    .split(slotPattern)
    .map((piece, i) => {
      if (i % 2 === 0) return piece;
      const name = names[(piece.codePointAt(0) ?? firstSlot) - firstSlot] ?? "";
      return <Fragment key={i}>{parts[name]}</Fragment>;
    });
}
