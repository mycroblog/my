// SPDX-License-Identifier: AGPL-3.0-only

// The generic layout parts (ADR-0171 §3): the parts that place what they are given and nothing else, from which every
// other part of the design system takes its layout. They know nothing of mycroblog: a prop names an arrangement
// (`align="center"`, `wrap`) or a token of the theme's spacing (`gap="entry-line"`), never a class, so that what only
// a browser can verify (what overflows, wraps, mirrors right-to-left, or lies over the page) has these parts as its
// only sources, each with its layout test, and no screen needs to be rendered to be trusted.
//
// The parts, by the arrangement of what they hold:
//
// - `Box`: one box and how it sits where it is put. It arranges nothing inside itself.
// - `Row`: its children side by side, in the reader's direction (a flex row); `inline` inside a line of text or a
//   control (an icon and its label).
// - `Column`: its children one under another, sharing its height (a flex column): a head that keeps its height over a
//   body that takes the rest and scrolls.
// - `Stack`: its children one under another, each as wide as the stack and never wider (a grid of one column).
// - `Grid`: its children in columns alike, row after row, or in the rows of a named template.
// - `Layers`: its children over one another, in one cell as large as the largest.
// - `Pin`: a box taken out of the flow and held against the edges of the nearest `anchor` (a badge on a corner, a
//   spinner over a label), or of the viewport.
// - `Sticky`: a box that scrolls with what holds it until it meets an edge, then stays.
// - `Spoken`: words for assistive technology alone, which take no room.
//
// Every part also says how its own box sits in the part that holds it (the props of `Placement`): whether it takes
// the room left or keeps its size, where it stands across the row, how far it reaches past an edge, what it does with
// what does not fit. A part that only fills a row is a `Box` with `grow`.
//
// What a part does unasked is what cannot push a page past the viewport, so that no composite overflows by leaving a
// prop out. Every child of a `Row`, a `Stack`, a `Grid` or `Layers` may be narrower than what it holds, whatever
// element it is but a picture, and a word longer than its line breaks inside its box (styles.css, for every text):
// the child that must keep its size says so (`rigid`), and that is for what has a size of its own (an avatar, a
// button, a tab), never for a text someone wrote; a short label of the interface that must keep its words whole
// beside a text that wraps says `words`. Beside a child that takes the room left (`grow`), not one as wide
// as the row (`full`), the others keep their width. A child that does not fill its `Column`, its `Stack` or its `Layers`, and a pinned
// box, are no wider than what holds them. Three things these defaults cannot hold, which their author must ask for:
// a text that may not wrap (`whitespace-nowrap`, a `rigid` child) is cut only by `lines`; a `Row` of more children
// than fit, or of rigid children wider together than the row, overflows unless it may `wrap` or `scroll`; and what a
// child lays out by itself, in classes of its own that are not these parts, keeps the browser's defaults.
//
// A part renders one element and adds none, so that composing these costs a function call and no node: `as` names its
// tag ("div" unless said), or is the element to lay out, for a component that renders its own (a React Aria
// Components' Button, a Motion element, an element that takes a ref): the part clones it with its classes after the
// element's own, which may be a function of the component's state, as React Aria's are; children that are such a
// function stay on that element. The caller's `className` is the part's paint (colour, type, radius, its own padding
// and fixed size from tokens): the lint (restrict/layout-parts) refuses layout in it. The other attributes reach the
// element in the order the caller wrote them. A block of text (a paragraph, a heading, a list, a figure) has no margin
// of its own anywhere in the design system (styles.css): where a box stands is its container's to say.
//
// A part takes its paint the same way (the props of `Paint`), so that a layer above the design system, which writes no
// class (ADR-0025 §4), composes these parts and the controls and nothing else. `Paint` says what a composite's own
// boxes are: the style of a text (its step, its tone, its weight) and the surface under a group (its colour, its
// corners, its padding), each by a token. It does not say three things, which are parts of the design system instead:
// what a control looks like in its states (hovered, pressed, focused, invalid, disabled), which is the control's; a
// rule that reaches into another part (`[&_a]:…`, `[&>svg]:…`), which is a prop of the part reached into; and a value
// written out (`rounded-[3px]`, `pt-[0.25rem]`), which is a token or nothing.
//
// What these props cannot say is not written beside them in a class: it becomes a prop here when it is an arrangement
// of its own (and a line of the story that shows it), or a layout part named in its own file when only one part of
// the design system has it (web/devtools/src/layout-parts.js lists both kinds and says what a layout part is).

import { cloneElement, type HTMLAttributes, type JSX, type ReactElement, type ReactNode } from "react";

/** The spacing tokens a gap may be, and `hair`, the least space there is: a label and the quiet line under it. */
const GAP = {
  hair: "gap-[0.125rem]",
  "article-block": "gap-article-block",
  "chip-gap": "gap-chip-gap",
  "chip-spacing": "gap-chip-spacing",
  "content-paragraph": "gap-content-paragraph",
  "dialog-gap": "gap-dialog-gap",
  "dialog-row": "gap-dialog-row",
  "disclosure-gap": "gap-disclosure-gap",
  "embed-gap": "gap-embed-gap",
  "emoji-picker-gap": "gap-emoji-picker-gap",
  entry: "gap-entry",
  "entry-action": "gap-entry-action",
  "entry-avatar": "gap-entry-avatar",
  "entry-caption": "gap-entry-caption",
  "entry-line": "gap-entry-line",
  "entry-name": "gap-entry-name",
  "entry-room": "gap-entry-room",
  feed: "gap-feed",
  "feed-tight": "gap-feed-tight",
  "form-page-alert-gap": "gap-form-page-alert-gap",
  "form-page-field-gap": "gap-form-page-field-gap",
  "form-page-form-gap": "gap-form-page-form-gap",
  "form-page-gap": "gap-form-page-gap",
  "form-page-head-gap": "gap-form-page-head-gap",
  "form-page-intro-gap": "gap-form-page-intro-gap",
  "form-page-list-gap": "gap-form-page-list-gap",
  "form-page-mark-gap": "gap-form-page-mark-gap",
  "form-page-screen-gap": "gap-form-page-screen-gap",
  "icon-button-gap": "gap-icon-button-gap",
  "kbd-gap": "gap-kbd-gap",
  media: "gap-media",
  "menu-group-gap": "gap-menu-group-gap",
  "menu-icon-gap": "gap-menu-icon-gap",
  "screen-air": "gap-screen-air",
  "screen-field-gap": "gap-screen-field-gap",
  "screen-focus-gap": "gap-screen-focus-gap",
  "screen-head-room": "gap-screen-head-room",
  "screen-name-gap": "gap-screen-name-gap",
  "screen-profile-gap": "gap-screen-profile-gap",
  "screen-row-gap": "gap-screen-row-gap",
  "screen-tab-gap": "gap-screen-tab-gap",
  "screen-tabs-gap": "gap-screen-tabs-gap",
  "shell-edge": "gap-shell-edge",
  "shell-inset": "gap-shell-inset",
  "shell-item-gap": "gap-shell-item-gap",
  "viewer-inset": "gap-viewer-inset",
} as const;

/** A gap between the children of a line alone, where the lines of a row that wraps stand otherwise apart. */
const GAP_X = {
  "entry-avatar": "gap-x-entry-avatar",
  "entry-caption": "gap-x-entry-caption",
  "entry-name": "gap-x-entry-name",
  "form-page-gap": "gap-x-form-page-gap",
  "screen-meta-gap": "gap-x-screen-meta-gap",
  "shell-edge": "gap-x-shell-edge",
  "toggle-gap": "gap-x-toggle-gap",
} as const;

/** A gap between the lines of a row that wraps, or the rows of a grid, alone. */
const GAP_Y = {
  "0": "gap-y-0",
  "form-page-footer-rows": "gap-y-form-page-footer-rows",
  "screen-tab-gap": "gap-y-screen-tab-gap",
} as const;

export type Gap = keyof typeof GAP;

const ALIGN = {
  start: "items-start",
  center: "items-center",
  end: "items-end",
  baseline: "items-baseline",
  stretch: "items-stretch",
} as const;

const JUSTIFY = {
  start: "justify-start",
  center: "justify-center",
  end: "justify-end",
  between: "justify-between",
} as const;

const SELF = {
  start: "self-start",
  center: "self-center",
  end: "self-end",
  stretch: "self-stretch",
  baseline: "self-baseline",
} as const;

/** Where the room left beside the box goes: before it (`end` pushes it to the end), after it, around it. */
const PUSH = {
  start: "me-auto",
  end: "ms-auto",
  bottom: "mt-auto",
  center: "mx-auto",
  middle: "m-auto",
} as const;

/** The room left that a box takes: its share of it, or all of it past the least width it keeps before it wraps. */
const GROW = {
  share: "flex-1",
  // A name over a handle beside what the reader may do: under this width, what follows goes on a next line.
  names: "flex-[1_1_12rem]",
  // From the width of what it holds, with its share of what is left: a place of a bar whose label may be long.
  content: "flex-[1_1_auto]",
} as const;

const FULL = { inline: "w-full", block: "h-full", both: "size-full" } as const;

/** The most a box may be: its container's width or height, or a width of the theme. */
const MAX = {
  full: "max-w-full",
  "full-block": "max-h-full",
  dialog: "max-w-[var(--dialog-max-width)]",
  menu: "max-w-[var(--menu-min-width)]",
  // The quiet line under a menu item's label: it wraps within this, in a menu as wide as its longest label.
  "menu-about": "max-w-[16rem]",
  // A sheet from the viewport's bottom edge leaves the page in sight above it.
  sheet: "max-h-[85dvh]",
  // The suggestions for a word of the composer: eight options, then they scroll.
  suggestions: "max-h-[calc(8*var(--control-form-height)+2*var(--menu-padding))]",
  // A dialog in the middle of the viewport, a margin from its edges, its padding around what it holds.
  "viewport-dialog": "max-h-[calc(100dvh-2*var(--dialog-margin)-2*var(--dialog-padding))]",
  // A picture or a video is no larger than the room it is in.
  contain: "max-h-full max-w-full",
  // A description of the media viewer: a column of a reading width, a third of the screen at most.
  reading: "max-h-[33dvh] max-w-[40rem]",
  // An article's cover is never taller than this, whatever its width.
  "article-cover": "max-h-article-cover-max",
  // A link card's image is never taller than this, whatever its width.
  "media-card": "max-h-media-card-max",
} as const;

const CLIP = { both: "overflow-hidden", x: "overflow-x-hidden", y: "overflow-y-hidden" } as const;
const SCROLL = { both: "overflow-auto", x: "overflow-x-auto", y: "overflow-y-auto" } as const;
const LINES = { 1: "truncate", 2: "line-clamp-2", 3: "line-clamp-3" } as const;
const LAYER = { 1: "z-1", 2: "z-2", 3: "z-3", 10: "z-10", 20: "z-20", 50: "z-50", 60: "z-60" } as const;

/**
 * How far a box reaches past an edge of its container, so that what it holds, not its own padding, lines up with the
 * edge: by the inline padding of a control, or of a row.
 */
const BLEED_START = {
  "control-timeline-inline": "-ms-control-timeline-inline",
  "icon-button-inline": "-ms-icon-button-inline",
  "screen-row-inline": "-ms-screen-row-inline",
} as const;
const BLEED_END = {
  "control-timeline-inline": "-me-control-timeline-inline",
  "icon-button-inline": "-me-icon-button-inline",
} as const;
const BLEED_INLINE = {
  "emoji-picker-gap": "-mx-emoji-picker-gap",
  "icon-button-inline": "-mx-icon-button-inline",
  "screen-row-inline": "-mx-screen-row-inline",
  "shell-edge": "-mx-shell-edge",
  "entry-reach": "-mx-entry-reach",
} as const;
/** Past its line above and below: a control taller than the line it stands on leaves the line its height. */
const BLEED_BLOCK = {
  "composer-inset": "-my-composer-inset",
  "icon-button-inline": "-my-icon-button-inline",
  "control-timeline-line": "-my-control-timeline-line",
  "target-line": "-my-target-line",
  "entry-reach": "-my-entry-reach",
} as const;

/** The space a box keeps on one side, where its container sets no gap: the children of a line of text, a float. */
const SPACE_ABOVE = {
  "embed-gap": "mt-embed-gap",
  "form-page-gap": "mt-form-page-gap",
  "screen-air": "mt-screen-air",
  "screen-group-gap": "mt-screen-group-gap",
  "content-emoji-line": "mt-content-emoji-line",
} as const;
const SPACE_BELOW = {
  "menu-group-gap": "mb-menu-group-gap",
  "shell-edge": "mb-shell-edge",
  "shell-gap": "mb-shell-gap",
  "content-emoji-line": "mb-content-emoji-line",
} as const;
const SPACE_BEFORE = {
  "entry-caption": "ms-entry-caption",
  "form-page-list-gap": "ms-form-page-list-gap",
  "screen-avatar-inset": "ms-screen-avatar-inset",
  "content-emoji-side": "ms-content-emoji-side",
} as const;
const SPACE_AFTER = {
  "entry-caption": "me-entry-caption",
  "shell-edge": "me-shell-edge",
  "content-emoji-side": "me-content-emoji-side",
} as const;

/** How the box of a part sits in the part that holds it. */
type Placement = {
  /**
   * It takes the room left along its row or column, shared with the others that grow; in a row that wraps, a name of
   * `GROW` is the least width it keeps before what follows it wraps.
   */
  grow?: boolean | Exclude<keyof typeof GROW, "share"> | undefined;
  /**
   * It may be narrower than what it holds, as every child of a part that arranges children already may: for a part
   * in an element that is none of these parts. `block`: it may be shorter, so that the region inside it scrolls in
   * place of pushing its column past the container.
   */
  shrink?: boolean | "block" | undefined;
  /**
   * It keeps its size: it neither grows nor gives way. For what has a size of its own (an icon, an avatar, a
   * button), which would be drawn over its neighbours if it gave way; a text someone wrote wraps instead.
   */
  rigid?: boolean | undefined;
  /**
   * It gives way down to its longest word and no further, so that its words stay whole while the text beside it
   * wraps: for a short label of the interface (a setting's value). Never for a text someone wrote, whose one long
   * word would push the row past its container.
   */
  words?: boolean | undefined;
  /** Where it stands across its row or column, when not where the others do. */
  self?: keyof typeof SELF | undefined;
  /** Where it goes with the room left beside it: to the `end` of its row, to the `bottom` of its column. */
  push?: keyof typeof PUSH | undefined;
  /** As wide as its container (`block`: as tall, `both`). */
  full?: boolean | "block" | "both" | undefined;
  /** The most it may be. */
  max?: keyof typeof MAX | undefined;
  /** What a `Pin` inside it is held against; `isolate` also keeps the layers inside it from those outside. */
  anchor?: boolean | "isolate" | undefined;
  /** At the end of the first line of the text that follows it, which runs around it. */
  float?: "end" | undefined;
  /** Where it stands on its line of text, as an inline box. */
  baseline?: keyof typeof BASELINE | undefined;
  /** Its layer over what it overlaps, where the order of the document does not say. */
  layer?: keyof typeof LAYER | undefined;
  /** What does not fit is cut off. */
  clip?: boolean | "x" | "y" | undefined;
  /** What does not fit scrolls inside it. */
  scroll?: "x" | "y" | "both" | undefined;
  /** Its text keeps to so many lines, ending in an ellipsis. */
  lines?: keyof typeof LINES | undefined;
  /**
   * As tall as one line of its text, so that what it holds (a glyph, a mark) stands on the first line of the text
   * beside it when that text wraps.
   */
  line?: boolean | undefined;
  /**
   * How far it reaches past its container's edge at its start, at its end, or at both, so that what it holds lines up
   * with the edge.
   */
  bleedStart?: keyof typeof BLEED_START | undefined;
  bleedEnd?: keyof typeof BLEED_END | undefined;
  bleedInline?: keyof typeof BLEED_INLINE | undefined;
  /** How far it reaches past its line above and below, so that the line keeps its height. */
  bleedBlock?: keyof typeof BLEED_BLOCK | undefined;
  /** The space it keeps on one side, where its container sets no gap. */
  spaceAbove?: keyof typeof SPACE_ABOVE | undefined;
  spaceBelow?: keyof typeof SPACE_BELOW | undefined;
  spaceBefore?: keyof typeof SPACE_BEFORE | undefined;
  spaceAfter?: keyof typeof SPACE_AFTER | undefined;
};

/** How an inline box stands on its line: on the text's middle, or as a glyph does, a little under the baseline. */
const BASELINE = {
  top: "align-top",
  middle: "align-middle",
  bottom: "align-bottom",
  glyph: "align-[-0.125em]",
} as const;

type Tag = keyof JSX.IntrinsicElements;

/** The steps of the type scale a text is set in. */
const TEXT = { caption: "text-caption", body: "text-body", title: "text-title" } as const;

/** The colour of a text: the page's, muted, or the page's surface where it is knocked out of the text's colour. */
const TONE = { text: "text-text", muted: "text-muted-text", inverse: "text-surface" } as const;

const WEIGHT = { normal: "font-normal", label: "font-label", strong: "font-strong" } as const;

/** The height of a text's lines, where not its step's own: an entry's, for a name beside an avatar. */
const LEADING = { entry: "leading-entry" } as const;

/** What a text keeps of how it was typed: its line breaks, or its line breaks and its spaces. */
const KEEPS = { lines: "whitespace-pre-line", spaces: "whitespace-pre-wrap" } as const;

/** Where a text may break besides its spaces: anywhere a long word must, or between phrases alone (ja). */
const BREAKS = { anywhere: "[overflow-wrap:anywhere]", phrase: "[word-break:auto-phrase]" } as const;

/** The surface under a group: the page's own (inside a raised one), or raised from the page. */
const SURFACE = { page: "bg-surface", raised: "bg-surface-raised" } as const;

/** The corners of a surface: a control's, or those of an entry's surface. */
const RADIUS = { control: "rounded-control", entry: "rounded-entry" } as const;

/** The padding of a surface or of a row, by a token of the theme's spacing: on every side, or on the sides of a line. */
const PAD = { embed: "p-embed", "poll-padding": "p-poll-padding" } as const;
const PAD_X = { "poll-option-inline": "px-poll-option-inline" } as const;
const PAD_Y = { "poll-result-block": "py-poll-result-block" } as const;

/**
 * What a generic part takes of paint: the style of a text and the surface under a group, which are a composite's own
 * to say. What a control looks like in its states (hovered, pressed, focused, invalid) is not here: a control is a
 * part of the design system, with its states.
 */
type Paint = {
  /** The step of the type scale its text is set in. */
  text?: keyof typeof TEXT | undefined;
  /** The colour of its text. */
  tone?: keyof typeof TONE | undefined;
  weight?: keyof typeof WEIGHT | undefined;
  /** The height of its lines, where not its step's own. */
  leading?: keyof typeof LEADING | undefined;
  /** What its text keeps of how it was typed, for a text someone wrote that is shown as plain text. */
  keeps?: keyof typeof KEEPS | undefined;
  /** Its figures are as wide as one another, so that a column of them lines up. */
  numeric?: boolean | undefined;
  /** Its text keeps to one line: for a few words that belong together, never for a text someone wrote. */
  nowrap?: boolean | undefined;
  /** Where its text may break besides its spaces. */
  breaks?: keyof typeof BREAKS | undefined;
  /** The surface under it. */
  surface?: keyof typeof SURFACE | undefined;
  /** Its corners. */
  radius?: keyof typeof RADIUS | undefined;
  /** Its padding on every side. */
  pad?: keyof typeof PAD | undefined;
  /** Its padding at the start and the end of its lines. */
  padX?: keyof typeof PAD_X | undefined;
  /** Its padding above and below. */
  padY?: keyof typeof PAD_Y | undefined;
  /** A list without its markers and, unless it has a padding, their room. */
  plain?: boolean | undefined;
};

/** The props of `Paint`, which the element a part renders does not take. */
const PAINT: Record<keyof Paint, true> = {
  text: true,
  tone: true,
  weight: true,
  leading: true,
  keeps: true,
  numeric: true,
  nowrap: true,
  breaks: true,
  surface: true,
  radius: true,
  pad: true,
  padX: true,
  padY: true,
  plain: true,
};

/** The classes of a part's paint. */
function paintOf(props: Paint): string {
  let c = "";
  if (props.surface) c += ` ${SURFACE[props.surface]}`;
  if (props.radius) c += ` ${RADIUS[props.radius]}`;
  if (props.plain) c += props.pad ? " list-none" : " list-none p-0";
  // A padded box counts its padding in its size.
  if (props.pad) c += ` box-border ${PAD[props.pad]}`;
  if (props.padX) c += ` box-border ${PAD_X[props.padX]}`;
  if (props.padY) c += ` ${PAD_Y[props.padY]}`;
  if (props.text) c += ` ${TEXT[props.text]}`;
  if (props.tone) c += ` ${TONE[props.tone]}`;
  if (props.weight) c += ` ${WEIGHT[props.weight]}`;
  if (props.leading) c += ` ${LEADING[props.leading]}`;
  if (props.keeps) c += ` ${KEEPS[props.keeps]}`;
  if (props.numeric) c += " tabular-nums";
  if (props.nowrap) c += " whitespace-nowrap";
  if (props.breaks) c += ` ${BREAKS[props.breaks]}`;
  return c;
}

/** Words for assistive technology alone, where what is shown says them by a shape or a place. */
export function Spoken({ children }: { children: ReactNode }) {
  return <span className="sr-only">{children}</span>;
}

/** What every layout part takes beside its arrangement. */
export type PartProps = Placement &
  Paint &
  Omit<HTMLAttributes<HTMLElement>, "className" | "style" | "children"> & {
    /** The tag it renders, or the element it lays out (a component that renders its own element). */
    as?: Tag | ReactElement<{ className?: unknown }> | undefined;
    /** Its paint, inside the design system, where a part writes classes; a layer above gives the props of `Paint`. */
    className?: string | undefined;
    children?: ReactNode;
  };

/** The props of a part that say where its box sits, which the element it renders does not take. */
const PLACEMENT: Record<keyof Placement | "as", true> = {
  as: true,
  grow: true,
  shrink: true,
  rigid: true,
  words: true,
  self: true,
  push: true,
  full: true,
  max: true,
  anchor: true,
  float: true,
  baseline: true,
  layer: true,
  clip: true,
  scroll: true,
  lines: true,
  line: true,
  bleedStart: true,
  bleedEnd: true,
  bleedInline: true,
  bleedBlock: true,
  spaceAbove: true,
  spaceBelow: true,
  spaceBefore: true,
  spaceAfter: true,
};

/**
 * The element of a part: `as` with the caller's props in the order the caller wrote them, the part's classes after
 * the caller's own.
 */
function part(layoutClassName: string, props: PartProps, style?: Record<string, string>) {
  const as = props.as ?? "div";
  let c = layoutClassName;
  if (props.grow) c += ` ${GROW[props.grow === true ? "share" : props.grow]}`;
  if (props.shrink) c += props.shrink === "block" ? " min-h-0" : " min-w-0";
  if (props.rigid) c += " flex-none";
  if (props.words) c += " min-w-auto";
  if (props.self) c += ` ${SELF[props.self]}`;
  if (props.push) c += ` ${PUSH[props.push]}`;
  if (props.full) c += ` ${FULL[props.full === true ? "inline" : props.full]}`;
  if (props.max) c += ` ${MAX[props.max]}`;
  if (props.anchor) c += props.anchor === "isolate" ? " relative isolate" : " relative";
  if (props.float) c += " float-end";
  if (props.baseline) c += ` ${BASELINE[props.baseline]}`;
  if (props.layer) c += ` ${LAYER[props.layer]}`;
  if (props.clip) c += ` ${CLIP[props.clip === true ? "both" : props.clip]}`;
  if (props.scroll) c += ` ${SCROLL[props.scroll]}`;
  if (props.lines) c += ` ${LINES[props.lines]}`;
  if (props.line) c += " h-[1lh]";
  if (props.bleedStart) c += ` ${BLEED_START[props.bleedStart]}`;
  if (props.bleedEnd) c += ` ${BLEED_END[props.bleedEnd]}`;
  if (props.bleedInline) c += ` ${BLEED_INLINE[props.bleedInline]}`;
  if (props.bleedBlock) c += ` ${BLEED_BLOCK[props.bleedBlock]}`;
  if (props.spaceAbove) c += ` ${SPACE_ABOVE[props.spaceAbove]}`;
  if (props.spaceBelow) c += ` ${SPACE_BELOW[props.spaceBelow]}`;
  if (props.spaceBefore) c += ` ${SPACE_BEFORE[props.spaceBefore]}`;
  if (props.spaceAfter) c += ` ${SPACE_AFTER[props.spaceAfter]}`;
  c = c.trim();
  const paint = `${props.className ?? ""}${paintOf(props)}`.trim();
  const own = paint === "" ? c : c === "" ? paint : `${paint} ${c}`;

  // The element's props: the caller's, with the classes where the caller wrote its own, else last.
  const given: Record<string, unknown> = props;
  const rest: Record<string, unknown> = {};
  for (const key in given) {
    if (key in PLACEMENT || key in PAINT) continue;
    if (key !== "className") rest[key] = given[key];
    else if (own !== "") rest["className"] = own;
  }
  if (!("className" in rest) && own !== "") rest["className"] = own;
  if (style !== undefined) rest["style"] = style;

  if (typeof as === "string") {
    const Element = as as "div";
    return <Element {...rest} />;
  }
  // The element's own classes first, as a function of its component's state where they are one.
  const its = as.props.className;
  if (typeof its === "function")
    rest["className"] = (...state: unknown[]) =>
      `${String((its as (...s: unknown[]) => unknown)(...state))} ${own}`.trim();
  else if (typeof its === "string" && its !== "") rest["className"] = `${its} ${own}`.trim();
  return cloneElement(as as ReactElement<Record<string, unknown>>, rest);
}

const DISPLAY = { block: "block", inline: "inline", "inline-block": "inline-block" } as const;

/** A block shown in one of the two colour schemes alone: the reader's, or the one the page's theme sets. */
const ONLY = {
  light: "block [:root[data-theme=dark]_&]:hidden dark:[:root:not([data-theme=light])_&]:hidden",
  dark: "hidden [:root[data-theme=dark]_&]:block dark:[:root:not([data-theme=light])_&]:block",
} as const;

export type BoxProps = PartProps & {
  /** In the flow as a block, or inside a line of text. */
  display?: keyof typeof DISPLAY | undefined;
  /** A block that shows in the light colour scheme alone, or in the dark one (a mark drawn for each). */
  only?: keyof typeof ONLY | undefined;
  /**
   * The paragraphs inside it run on as one text, in place of standing one under another (an excerpt of a post);
   * `all`: its blocks too.
   */
  run?: boolean | "all" | undefined;
  /** As wide as this share of its container (0 to 1): the fill of a meter. */
  share?: number | undefined;
  /** As wide as it is tall times this (width over height): a picture's box, whatever its width. */
  ratio?: number | undefined;
  /** Its press reaches over the whole of the nearest `anchor` (a line that is one link). */
  reach?: boolean | undefined;
};

/** One box, and how it sits where it is put: it arranges nothing inside itself. */
export function Box({ display, only, run, share, ratio, reach, ...rest }: BoxProps) {
  let c = display ? DISPLAY[display] : only ? ONLY[only] : "";
  if (run) c += run === "all" ? " [&_p]:inline [&_div]:inline" : " [&_p]:inline";
  if (reach) c += " after:absolute after:inset-0 after:content-['']";
  const style: Record<string, string> = {};
  if (share !== undefined) style["inlineSize"] = `${String(Math.round(share * 1000) / 10)}%`;
  if (ratio !== undefined) style["aspectRatio"] = String(ratio);
  return part(c, rest, share === undefined && ratio === undefined ? undefined : style);
}

type Flow = {
  /** Where the children stand across the row (a column: along its width). */
  align?: keyof typeof ALIGN | undefined;
  /** Where the children stand along the row when room is left (a column: along its height). */
  justify?: keyof typeof JUSTIFY | undefined;
  /** The space between the children. */
  gap?: Gap | undefined;
};

function flow(className: string, { align, justify, gap }: Flow) {
  let out = className;
  if (align) out += ` ${ALIGN[align]}`;
  if (justify) out += ` ${JUSTIFY[justify]}`;
  if (gap) out += ` ${GAP[gap]}`;
  return out;
}

/**
 * What a part that places children side by side gives each of them unasked: it may be narrower than what it holds.
 * A picture (an image, a glyph, a video, a canvas) is no box of text: it keeps the size it has, which squeezing would
 * distort. Without specificity, so that a child's own least width is heard.
 */
const SHRINKS = "[:where(&)>:where(:not(img,svg,video,canvas,picture))]:min-w-0";

/** A child that does not fill what holds it is no wider than it, its padding counted. */
const FITS = "[:where(&)>*]:box-border [:where(&)>*]:max-w-full";

/** How far each child of a row lies over the one before it: faces in a strip. */
const OVERLAP = {
  "screen-face-overlap": "[&>*+*]:-ms-screen-face-overlap",
  "screen-face-tuck": "[&>*+*]:-ms-screen-face-tuck",
} as const;

export type RowProps = PartProps &
  Flow & {
    /** Inside a line of text or a control, as wide as what it holds. */
    inline?: boolean | undefined;
    /** The children that do not fit go on a next line. */
    wrap?: boolean | undefined;
    /** The space between the children of a line alone. */
    gapX?: keyof typeof GAP_X | undefined;
    /** The space between the lines of a row that wraps alone. */
    gapY?: keyof typeof GAP_Y | undefined;
    /** Each child lies this far over the one before it. */
    overlap?: keyof typeof OVERLAP | undefined;
    /** Its children share its width alike, whatever each holds (a refusal beside its confirmation). */
    even?: boolean | undefined;
    /**
     * On a touch screen, its press target reaches the touch height above and below it, whatever its own height (an
     * icon button in a row of text); it is its own `anchor`.
     */
    touch?: boolean | undefined;
  };

/** Its children side by side, in the reader's direction. */
export function Row({ inline, wrap, align, justify, gap, gapX, gapY, overlap, even, touch, ...rest }: RowProps) {
  let c = flow(wrap ? (inline ? "inline-flex flex-wrap" : "flex flex-wrap") : inline ? "inline-flex" : "flex", {
    align,
    justify,
    gap,
  });
  // A box inside a line is as wide as what it holds: no wider than the line, its padding counted.
  c += inline ? ` box-border max-w-full ${SHRINKS}` : ` ${SHRINKS}`;
  if (gapX) c += ` ${GAP_X[gapX]}`;
  if (gapY) c += ` ${GAP_Y[gapY]}`;
  if (overlap) c += ` ${OVERLAP[overlap]}`;
  if (even) c += " [&>*]:flex-1";
  if (touch)
    c +=
      " pointer-coarse:before:absolute pointer-coarse:before:inset-x-0 pointer-coarse:before:top-1/2" +
      " pointer-coarse:before:h-icon-button-touch pointer-coarse:before:-translate-y-1/2";
  return part(c, rest);
}

export type ColumnProps = PartProps & Flow;

/**
 * Its children one under another, sharing its height: one that is `rigid` keeps its own, one that `grow`s takes the
 * rest (and scrolls what does not fit, with `shrink="block"` and `scroll`).
 */
export function Column({ align, justify, gap, ...rest }: ColumnProps) {
  // Across a column a child is as wide as the column, or, where it stands at one side of it, no wider.
  const c = flow("flex flex-col", { align, justify, gap });
  return part(align === undefined || align === "stretch" ? c : `${c} ${FITS}`, rest);
}

const CONTENT = { start: "content-start", center: "content-center" } as const;
const JUSTIFY_ITEMS = { start: "justify-items-start", center: "justify-items-center" } as const;

export type StackProps = PartProps & {
  /** The space between the children. */
  gap?: Gap | undefined;
  /** Where the children stand along the stack's width, when not across the whole of it. */
  justify?: keyof typeof JUSTIFY_ITEMS | undefined;
  /** Where the children stand in a stack taller than they are, when not spread over it. */
  pack?: keyof typeof CONTENT | undefined;
};

/** Its children one under another, each as wide as the stack and never wider. */
export function Stack({ gap, justify, pack, ...rest }: StackProps) {
  let c = "grid grid-cols-[minmax(0,1fr)]";
  if (gap) c += ` ${GAP[gap]}`;
  if (justify) c += ` ${JUSTIFY_ITEMS[justify]} ${FITS}`;
  if (pack) c += ` ${CONTENT[pack]}`;
  return part(c, rest);
}

/**
 * The templates of a grid's columns: so many alike, none with a least width. A column as wide as what it holds would
 * take the whole grid for one long word and leave the others none: a grid whose columns differ is its part's own
 * arrangement, named in its file.
 */
const COLUMNS = {
  1: "grid-cols-[minmax(0,1fr)]",
  2: "grid-cols-2",
} as const;

/** The templates of a grid's rows: a `head` as tall as what it holds over a body that takes the rest. */
const ROWS = { head: "grid-rows-[auto_minmax(0,1fr)]" } as const;

export type GridProps = PartProps & {
  columns?: keyof typeof COLUMNS | undefined;
  rows?: keyof typeof ROWS | undefined;
  /** The space between the cells. */
  gap?: Gap | undefined;
  gapX?: keyof typeof GAP_X | undefined;
  gapY?: keyof typeof GAP_Y | undefined;
  /** Where each child stands in its cell, down the cell. */
  align?: keyof typeof ALIGN | undefined;
};

/** Its children in columns alike, row after row. */
export function Grid({ columns = 1, rows, gap, gapX, gapY, align, ...rest }: GridProps) {
  let c = `grid ${COLUMNS[columns]}`;
  if (rows) c += ` ${ROWS[rows]}`;
  if (gap) c += ` ${GAP[gap]}`;
  if (gapX) c += ` ${GAP_X[gapX]}`;
  if (gapY) c += ` ${GAP_Y[gapY]}`;
  if (align) c += ` ${ALIGN[align]}`;
  return part(c, rest);
}

export type LayersProps = PartProps & {
  /** Every child in the middle of the cell, in place of filling it. */
  center?: boolean | undefined;
};

/**
 * Its children over one another, the later over the earlier, in one cell as large as the largest: a picture under
 * its badge, a text under its placeholder.
 */
export function Layers({ center, ...rest }: LayersProps) {
  const cell = "grid grid-cols-[minmax(0,1fr)] [&>*]:[grid-area:1/1]";
  return part(center ? `${cell} place-items-center ${FITS}` : cell, rest);
}

/** How far from an edge a pinned or a sticky box is held: on it, or a token of the theme's spacing away. */
const TOP = {
  "0": "top-0",
  half: "top-1/2",
  "composer-field-block": "top-composer-field-block",
  "media-badge": "top-media-badge",
  "shell-gap": "top-shell-gap",
} as const;
const BOTTOM = { "0": "bottom-0", "media-badge": "bottom-media-badge" } as const;
const START = {
  "0": "start-0",
  "control-timeline-inline": "start-control-timeline-inline",
  "viewer-inset": "start-viewer-inset",
} as const;
const END = {
  "0": "end-0",
  "entry-line": "end-entry-line",
  "media-badge": "end-media-badge",
  "viewer-inset": "end-viewer-inset",
} as const;
const INSET_INLINE = {
  "0": "inset-x-0",
  "media-badge": "inset-x-media-badge",
  "shell-edge": "inset-x-shell-edge",
} as const;
const INSET_BLOCK = { "0": "inset-y-0" } as const;

type Edges = {
  top?: keyof typeof TOP | undefined;
  bottom?: keyof typeof BOTTOM | undefined;
  start?: keyof typeof START | undefined;
  end?: keyof typeof END | undefined;
  /** From both the start and the end. */
  insetInline?: keyof typeof INSET_INLINE | undefined;
  /** From both the top and the bottom. */
  insetBlock?: keyof typeof INSET_BLOCK | undefined;
};

function edges(className: string, { top, bottom, start, end, insetInline, insetBlock }: Edges) {
  let out = className;
  if (top) out += ` ${TOP[top]}`;
  if (bottom) out += ` ${BOTTOM[bottom]}`;
  if (start) out += ` ${START[start]}`;
  if (end) out += ` ${END[end]}`;
  if (insetInline) out += ` ${INSET_INLINE[insetInline]}`;
  if (insetBlock) out += ` ${INSET_BLOCK[insetBlock]}`;
  return out;
}

export type PinProps = PartProps &
  Edges & {
    /** Against the viewport's edges, in place of the nearest `anchor`'s. */
    viewport?: boolean | undefined;
    /** Over the whole of what it is held against. */
    cover?: boolean | undefined;
    /** In the middle of what it is held against, at its own size (which it must have). */
    center?: boolean | undefined;
  };

/**
 * A box taken out of the flow and held against the edges of the nearest `anchor`, or of the viewport: over the whole
 * of it, in its middle, or by the edges it names.
 */
export function Pin({ viewport, cover, center, top, bottom, start, end, insetInline, insetBlock, ...rest }: PinProps) {
  let c = viewport ? "fixed" : "absolute";
  if (cover || center) c += " inset-0";
  if (center) c += " m-auto";
  // As wide as what it holds, and no wider than what it is held against, its padding counted.
  else if (!cover) c += " box-border max-w-full";
  return part(edges(c, { top, bottom, start, end, insetInline, insetBlock }), rest);
}

export type StickyProps = PartProps & Pick<Edges, "top" | "bottom">;

/** A box that scrolls with what holds it until it meets the top or the bottom it names, then stays. */
export function Sticky({ top, bottom, ...rest }: StickyProps) {
  return part(edges("sticky", { top, bottom }), rest);
}
