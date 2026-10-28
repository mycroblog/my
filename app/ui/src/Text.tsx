// SPDX-License-Identifier: AGPL-3.0-only
// Text by its role (app/ui/README.md "Type roles", web-client/16-look-states-and-wording.md §16.2): `Text` for any run of text with typed size,
// tone, weight and wrapping; `Hint` for the secondary text of a region (G6); `InlineError` for an error in place.
import { createElement } from "react";
import { tv } from "tailwind-variants";
import type { PolymorphicProps } from "./layout";

/** The elements a run of text may render. */
export type TextElement =
  "p" | "span" | "div" | "li" | "dt" | "dd" | "code" | "time" | "strong" | "small" | "label" | "legend";

const text = tv({
  variants: {
    size: { xs: "text-xs", sm: "text-sm", base: "text-base", lg: "text-lg" },
    tone: {
      fg: "text-fg",
      muted: "text-fg-muted",
      danger: "text-danger",
      warning: "text-warning",
      success: "text-success",
      accent: "text-accent",
    },
    weight: { normal: "font-normal", medium: "font-medium", semibold: "font-semibold" },
    wrap: { anywhere: "wrap-anywhere", truncate: "truncate", pretty: "text-pretty" },
    whitespace: { preWrap: "whitespace-pre-wrap", preLine: "whitespace-pre-line" },
    numeric: { true: "tabular-nums" },
    mono: { true: "font-mono" },
    block: { true: "block" },
    align: { start: "text-start", center: "text-center", matchParent: "align-match-parent" },
    lineClamp: { 2: "line-clamp-2", 3: "line-clamp-3" },
    selectAll: { true: "select-all" },
    shrink: { true: "min-w-0" },
    inset: { true: "px-1" },
    eyebrow: { true: "text-xs font-extrabold tracking-widest text-accent uppercase" },
  },
});

type TextOwnProps = {
  /** The size of the role; inherited when unset. */
  size?: "xs" | "sm" | "base" | "lg";
  /** The color token; inherited when unset. `muted` is for secondary information only, never body text or an action (G6). */
  tone?: "fg" | "muted" | "danger" | "warning" | "success" | "accent";
  /** The weight; inherited when unset. */
  weight?: "normal" | "medium" | "semibold";
  /**
   * `anywhere` for text that may hold a long word (a URL, a handle, people's words); `truncate` for one line cut with an
   * ellipsis (a name in a row, inside a `shrink` parent); `pretty` to balance the last line of a short heading-like text (G7).
   */
  wrap?: "anywhere" | "truncate" | "pretty";
  /** Keeps the line breaks people wrote: `preWrap` keeps spaces too, `preLine` collapses them. */
  whitespace?: "preWrap" | "preLine";
  /** Digits of equal width, for counts and amounts (G8). */
  numeric?: boolean;
  /** The monospaced family, for codes and identifiers. */
  mono?: boolean;
  /** Renders an inline element (`span`) as a block. */
  block?: boolean;
  /**
   * `start` / `center` set the alignment; `matchParent` lines a `dir="auto"` block up with its parent's direction
   * instead of its own (G9).
   */
  align?: "start" | "center" | "matchParent";
  /** Cuts the text after this many lines. */
  lineClamp?: 2 | 3;
  /** Selects the whole text on a click, for a code to copy. */
  selectAll?: boolean;
  /** Lets the element shrink below its content's width in a row (`min-w-0`). */
  shrink?: boolean;
  /** A little space at both sides (`px-1`), for the `legend` that interrupts a bordered group's edge. */
  inset?: boolean;
  /** The small upper-case label above a heading (`text-xs`, wide letter spacing, `text-accent`); its text is the caller's. */
  eyebrow?: boolean;
};

export type TextProps<E extends TextElement = "span"> = PolymorphicProps<E, TextOwnProps>;

/** A run of text with typed size, tone, weight and wrapping. A `span` unless `as` says otherwise. */
export function Text<E extends TextElement = "span">({
  as,
  size,
  tone,
  weight,
  wrap,
  whitespace,
  numeric,
  mono,
  block,
  align,
  lineClamp,
  selectAll,
  shrink,
  inset,
  eyebrow,
  ...rest
}: TextProps<E>) {
  return createElement(as ?? "span", {
    ...rest,
    className: text({
      size,
      tone,
      weight,
      wrap,
      whitespace,
      numeric,
      mono,
      block,
      align,
      lineClamp,
      selectAll,
      shrink,
      inset,
      eyebrow,
    }),
  });
}

export type HintProps<E extends "p" | "span" | "div" = "p"> = Omit<TextProps<E>, "tone" | "size"> & {
  /** `sm` (the default) is the role of secondary text (G6); `inherit` keeps the size around it. */
  size?: "sm" | "inherit";
};

/** Secondary text: a description, a hint, a count or a time (`text-sm text-fg-muted`, G6). A paragraph unless `as` says otherwise. */
export function Hint<E extends "p" | "span" | "div" = "p">({ as, size = "sm", ...rest }: HintProps<E>) {
  // The generic props spread into Text's own generic props, which TypeScript cannot relate without the cast.
  const props = { ...rest, as: as ?? "p", size: size === "inherit" ? undefined : size, tone: "muted" } as TextProps;
  return <Text {...props} />;
}

export type InlineErrorProps<E extends "p" | "span" | "div" = "p"> = Omit<TextProps<E>, "tone" | "size" | "role"> & {
  /** `sm` (the default) under a control or in a dialog; `inherit` keeps the size around it. */
  size?: "sm" | "inherit";
  /**
   * Announces the error at once (`role="alert"`). Only for the result of the user's own action, such as a failed save;
   * an error that came with the page is found by reading (web-client/16-look-states-and-wording.md G16).
   */
  alert?: boolean;
};

/**
 * An error shown in place (`text-danger`): the failure of an action next to it. A field's own error belongs to the
 * field's `errorMessage`, and an error of a loaded region to `RegionError`.
 */
export function InlineError<E extends "p" | "span" | "div" = "p">({
  as,
  size = "sm",
  alert = false,
  ...rest
}: InlineErrorProps<E>) {
  const props = {
    ...rest,
    as: as ?? "p",
    size: size === "inherit" ? undefined : size,
    tone: "danger",
    role: alert ? "alert" : undefined,
  } as TextProps;
  return <Text {...props} />;
}
