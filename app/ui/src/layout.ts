// SPDX-License-Identifier: AGPL-3.0-only
// The shared recipe of the layout primitives (Stack, Cluster, Grid): a screen lays itself out with typed props whose values
// are the spacing steps of web-client/16-look-states-and-wording.md §16.1 (G1) and the roles of app/ui/README.md, never with classes of its own.
import type { ComponentPropsWithRef, ElementType } from "react";
import { tv } from "tailwind-variants";

/** The spacing steps a gap or a padding may take (web-client/16-look-states-and-wording.md G1). */
export type Space = "0" | "0.5" | "1" | "2" | "3" | "4" | "6" | "8" | "10";

/** The elements a layout primitive may render, so that a screen keeps its landmarks, lists and forms without a wrapper. */
export type LayoutElement =
  | "div"
  | "section"
  | "article"
  | "aside"
  | "header"
  | "footer"
  | "nav"
  | "main"
  | "form"
  | "fieldset"
  | "ul"
  | "ol"
  | "li"
  | "dl"
  | "dd"
  | "p"
  | "span"
  | "label";

/**
 * The props of a component that renders the element `E` (`as`): its own props, then the element's attributes and ref
 * except `className` and `style`, so that the look stays in the kit.
 */
export type PolymorphicProps<E extends ElementType, Own> = Own & { as?: E } & Omit<
    ComponentPropsWithRef<E>,
    keyof Own | "as" | "className" | "style"
  >;

/** Props shared by `Stack` and `Cluster`. */
export type FlexLayoutProps = {
  /** Cross-axis alignment of the children (`items-*`). Stretch (the default) in a `Stack` makes every child as wide as the stack. */
  align?: "stretch" | "start" | "center" | "end" | "baseline";
  /** Main-axis distribution of the children (`justify-*`). */
  justify?: "start" | "center" | "end" | "between";
} & BoxProps;

/** Props shared by every layout primitive. */
export type BoxProps = {
  /** Makes the element a size container (`@container`), so the `@sm:` / `@md:` variants inside it answer to its width (web-client/README.md D2). */
  container?: boolean;
  /** Caps the inline size: `prose` (`max-w-prose`, running text), `sm` / `xs` (`w-full max-w-sm` / `max-w-xs`, a narrow form), `full` (`w-full`). */
  width?: "prose" | "sm" | "xs" | "full";
  /** Lets the element shrink below its content's width in a row (`min-w-0`), so long words wrap and names truncate. */
  shrink?: boolean;
  /** Takes the free space of a row (`flex-1`). */
  grow?: boolean;
  /** Takes a whole line of a wrapping row (`basis-full`). */
  fullRow?: boolean;
  /** Its own cross-axis alignment in its parent (`self-*`). */
  self?: "start" | "center" | "end";
  /** Padding on every side, in a spacing step. */
  padding?: Space;
  /** Inline padding, in a spacing step. A row of a list or a card is `paddingX="4" paddingY="3"` (README "Spacing"). */
  paddingX?: Space;
  /** Block padding, in a spacing step. */
  paddingY?: Space;
  /** Padding on the block start, in a spacing step. */
  paddingTop?: Space;
  /** Padding on the block end, in a spacing step. */
  paddingBottom?: Space;
  /** Margin on the block start, in a spacing step: sets a block apart from the content above it. */
  marginTop?: Space;
  /** Margin on the inline start, in a spacing step: indents a block under the control it describes. */
  marginStart?: Space;
  /**
   * A border: `md` / `lg` (a box with `rounded-md` / `rounded-lg`), `y` (the block edges, a list between other content), `top` (a rule above a region), `bottom` (a rule under a region),
   * `start` (a 2px rule on the inline start with `ps-3`, a quoted or nested block).
   */
  border?: "md" | "lg" | "y" | "top" | "bottom" | "start";
  /**
   * A background: `surface` (`bg-surface`, a row that stands on the page background) or `sunken` (`bg-surface-sunken`, a
   * well inside a raised surface, such as the quote of the post a form replies to).
   */
  background?: "surface" | "sunken";
  /** Moves the element to the inline end of its row (`ms-auto`), whether or not the row wraps. */
  pushEnd?: boolean;
  /** Text color for everything inside, unless a child sets its own. */
  tone?: "fg" | "muted" | "danger" | "warning";
  /** Text size for everything inside, unless a child sets its own. */
  textSize?: "xs" | "sm" | "base" | "lg";
};

/** `gap-*` per spacing step. */
export const gaps = {
  "0": "gap-0",
  "0.5": "gap-0.5",
  "1": "gap-1",
  "2": "gap-2",
  "3": "gap-3",
  "4": "gap-4",
  "6": "gap-6",
  "8": "gap-8",
  "10": "gap-10",
} as const satisfies Record<Space, string>;

/** `gap-x-*` per spacing step. */
export const columnGaps = {
  "0": "gap-x-0",
  "0.5": "gap-x-0.5",
  "1": "gap-x-1",
  "2": "gap-x-2",
  "3": "gap-x-3",
  "4": "gap-x-4",
  "6": "gap-x-6",
  "8": "gap-x-8",
  "10": "gap-x-10",
} as const satisfies Record<Space, string>;

/** `gap-y-*` per spacing step. */
export const rowGaps = {
  "0": "gap-y-0",
  "0.5": "gap-y-0.5",
  "1": "gap-y-1",
  "2": "gap-y-2",
  "3": "gap-y-3",
  "4": "gap-y-4",
  "6": "gap-y-6",
  "8": "gap-y-8",
  "10": "gap-y-10",
} as const satisfies Record<Space, string>;

/** The box variants every layout primitive has. */
export const box = tv({
  variants: {
    container: { true: "@container" },
    width: { prose: "max-w-prose", sm: "w-full max-w-sm", xs: "w-full max-w-xs", full: "w-full" },
    shrink: { true: "min-w-0" },
    grow: { true: "flex-1" },
    fullRow: { true: "basis-full" },
    self: { start: "self-start", center: "self-center", end: "self-end" },
    padding: {
      "0": "p-0",
      "0.5": "p-0.5",
      "1": "p-1",
      "2": "p-2",
      "3": "p-3",
      "4": "p-4",
      "6": "p-6",
      "8": "p-8",
      "10": "p-10",
    },
    paddingX: {
      "0": "px-0",
      "0.5": "px-0.5",
      "1": "px-1",
      "2": "px-2",
      "3": "px-3",
      "4": "px-4",
      "6": "px-6",
      "8": "px-8",
      "10": "px-10",
    },
    paddingY: {
      "0": "py-0",
      "0.5": "py-0.5",
      "1": "py-1",
      "2": "py-2",
      "3": "py-3",
      "4": "py-4",
      "6": "py-6",
      "8": "py-8",
      "10": "py-10",
    },
    paddingTop: {
      "0": "pt-0",
      "0.5": "pt-0.5",
      "1": "pt-1",
      "2": "pt-2",
      "3": "pt-3",
      "4": "pt-4",
      "6": "pt-6",
      "8": "pt-8",
      "10": "pt-10",
    },
    paddingBottom: {
      "0": "pb-0",
      "0.5": "pb-0.5",
      "1": "pb-1",
      "2": "pb-2",
      "3": "pb-3",
      "4": "pb-4",
      "6": "pb-6",
      "8": "pb-8",
      "10": "pb-10",
    },
    marginTop: {
      "0": "mt-0",
      "0.5": "mt-0.5",
      "1": "mt-1",
      "2": "mt-2",
      "3": "mt-3",
      "4": "mt-4",
      "6": "mt-6",
      "8": "mt-8",
      "10": "mt-10",
    },
    marginStart: {
      "0": "ms-0",
      "0.5": "ms-0.5",
      "1": "ms-1",
      "2": "ms-2",
      "3": "ms-3",
      "4": "ms-4",
      "6": "ms-6",
      "8": "ms-8",
      "10": "ms-10",
    },
    border: {
      md: "rounded-md border border-border",
      lg: "rounded-lg border border-border",
      y: "border-y border-border",
      top: "border-t border-border",
      bottom: "border-b border-border",
      start: "border-s-2 border-border ps-3",
    },
    background: { surface: "bg-surface", sunken: "bg-surface-sunken" },
    pushEnd: { true: "ms-auto" },
    tone: { fg: "text-fg", muted: "text-fg-muted", danger: "text-danger", warning: "text-warning" },
    textSize: { xs: "text-xs", sm: "text-sm", base: "text-base", lg: "text-lg" },
  },
});

/** The flex variants of `Stack` and `Cluster`. */
export const flex = tv({
  extend: box,
  base: "flex",
  variants: {
    align: { stretch: "", start: "items-start", center: "items-center", end: "items-end", baseline: "items-baseline" },
    justify: { start: "", center: "justify-center", end: "justify-end", between: "justify-between" },
  },
});

/** Picks the box props out of a component's props, so the rest goes to the element. */
export function splitBox<P extends BoxProps>({
  container,
  width,
  shrink,
  grow,
  fullRow,
  self,
  padding,
  paddingX,
  paddingY,
  paddingTop,
  paddingBottom,
  marginTop,
  marginStart,
  border,
  background,
  pushEnd,
  tone,
  textSize,
  ...rest
}: P) {
  return [
    {
      container,
      width,
      shrink,
      grow,
      fullRow,
      self,
      padding,
      paddingX,
      paddingY,
      paddingTop,
      paddingBottom,
      marginTop,
      marginStart,
      border,
      background,
      pushEnd,
      tone,
      textSize,
    },
    rest,
  ] as const;
}
