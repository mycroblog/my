// SPDX-License-Identifier: AGPL-3.0-only

// The live options of the hover and press looks of words (a text button, a text link, a quiet link) and of the press
// layers over pictures (#3369), which the stories offer the decider; none of them moves. Until the pick, no look is
// the components' own; the pick moves into them and this file goes.

import { createContext, useContext, type ReactNode } from "react";

/** How words answer: their underline thickens, their ink lightens, or a wash comes behind them. */
export type WordsLook = "underline" | "ink" | "wash";
/** How a picture answers: a shade over it, a wash of the page's surface over it, or an edge inside it. */
export type PicturesLook = "shade" | "wash" | "edge";

const WordsContext = createContext<WordsLook | undefined>(undefined);
const PicturesContext = createContext<PicturesLook | undefined>(undefined);

/** Shows the words and the pictures inside in a look of the options. */
export function PressOptions({
  words,
  pictures,
  children,
}: {
  words?: WordsLook;
  pictures?: PicturesLook;
  children: ReactNode;
}) {
  return (
    <WordsContext value={words}>
      <PicturesContext value={pictures}>{children}</PicturesContext>
    </WordsContext>
  );
}

// Underlined words thicken their line; a quiet link, which has none, takes one. Ink fades the words a step toward
// the page at hover and two at press. A wash puts the surface's hover and press steps behind the words' own line,
// not behind the room their target reaches past it.
const underlined: Record<WordsLook, string> = {
  underline: "data-hovered:decoration-2 data-pressed:decoration-[3px]",
  ink: "data-hovered:[color:color-mix(in_srgb,currentColor_75%,var(--color-surface))] data-pressed:[color:color-mix(in_srgb,currentColor_55%,var(--color-surface))]",
  wash: "bg-clip-content data-hovered:bg-surface-hover data-pressed:bg-surface-pressed",
};
const bare: Record<WordsLook, string> = {
  underline: "underline-offset-[0.15em] data-hovered:underline data-pressed:underline data-pressed:decoration-[3px]",
  ink: underlined.ink,
  wash: underlined.wash,
};

/** The classes of the words' look of the options: of underlined words, or of words with no underline. */
export function useWordsLook(kind: "underlined" | "bare"): string {
  const look = useContext(WordsContext);
  if (look === undefined) return "";
  return (kind === "underlined" ? underlined : bare)[look];
}

// A shade mixes the press tokens' shade over the picture, 10% at hover and 20% at press; a wash, the page's surface,
// 20% and 40%; an edge, a line of the muted text's color inside the picture, 2px and 4px.
const pictures: Record<PicturesLook, string> = {
  shade: [
    "data-hovered:bg-[color-mix(in_srgb,var(--press-shade)_var(--press-hover),transparent)]",
    "data-pressed:bg-[color-mix(in_srgb,var(--press-shade)_var(--press-pressed),transparent)]",
  ].join(" "),
  wash: [
    "data-hovered:bg-[color-mix(in_srgb,var(--color-surface)_20%,transparent)]",
    "data-pressed:bg-[color-mix(in_srgb,var(--color-surface)_40%,transparent)]",
  ].join(" "),
  edge: [
    "data-hovered:shadow-[inset_0_0_0_2px_var(--color-muted-text)]",
    "data-pressed:shadow-[inset_0_0_0_4px_var(--color-muted-text)]",
  ].join(" "),
};

/** The classes of the pictures' look of the options. */
export function usePicturesLook(): string {
  const look = useContext(PicturesContext);
  return look === undefined ? "" : pictures[look];
}
