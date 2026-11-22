// SPDX-License-Identifier: AGPL-3.0-only

// The app's frame (ADR-0025 §4, ADR-0110 §10, §11): the regions of a screen by the viewport's width, the one layout
// that reads the viewport (its breakpoints are tokens, `tablet:`, `desktop:` and `wide:` of theme.ts). The app chooses
// what goes in each region; the frame lays them out, with no visible page header, since the navigation shows where the
// reader is:
//
// - under `tablet` (a phone): the main column at the full width, and the dock fixed at the bottom: the tab bar, its
//   places as glyphs alone, the call's bar right above it while the reader is in a call, the primary action floating
//   at the end above both, and the toasts above all, at the dock's width;
// - from `tablet`: a rail of glyphs at the start, with the primary action under the mark and the call's bar ending it,
//   beside the main column; the toasts at the bottom, in the middle;
// - from `desktop`: the rail grows into a sidebar with the labels;
// - from `wide`: a column beside the main one, for what is around the reader, in sections on the page.
//
// The notices of the server (its closure, the reader offline, the server down) stand at the top of the main column,
// each on the raised surface, the closure first. A count of what waits (unread notifications, reports to handle) is a
// badge knocked out of the text's color, with no hue, since the accent means the primary action and links only. Every
// other surface is the page's; the dock stands apart from what scrolls under it by a shadow, not a line. A link of the
// frame navigates through the app's router when the app places NavigationProvider.

import { motion } from "@my/design-tokens";
import {
  createContext,
  use,
  useEffect,
  useRef,
  useState,
  type ReactElement,
  type ReactNode,
  type RefObject,
} from "react";
import { Button as AriaButton, Link, Tooltip, TooltipTrigger } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { ButtonBase } from "./button";
import { Icon, type IconName } from "./icons";
import { Box, Column, Row, Stack, type Gap } from "./layout";
import { UnreadMark } from "./list";
import { useReader } from "./locale";
import { MenuGroup, MenuItem, MenuSurface } from "./menu";
import { HeadcountMark, type Headcount } from "./voice";

/**
 * Where a part of the frame sits: in the rail or sidebar, in the tab bar, floating over it, in the Menu (the one menu,
 * a sheet on a phone and a popover from `tablet`), or in the dock above the tab bar (the call's bar).
 */
type Place = "side" | "tabs" | "float" | "menu" | "dock";

const PlaceContext = createContext<Place>("side");

const frame = tv({
  slots: {
    // On a phone, `--shell-call` is the room the call's bar takes above the tab bar while there is one.
    // On a phone, a disclosure's toggle is its label's width and little more, wherever on the page it stands, what
    // lies over the frame too (`--disclosure-toggle-inline` on the page, which disclosure.tsx reads).
    // On a phone, the page's scroller keeps what it scrolls into view (the focus, a match of find in page) clear of the
    // dock: its scroll padding at the end is `--shell-dock`, the height the dock covers (useDockHeight). At every
    // width it keeps it clear of what is held at the viewport's bottom over the screen (`--shell-foot`, the height of
    // a `FrameFoot`), above the dock on a phone.
    root: [
      "box-border min-h-dvh bg-surface font-sans text-text [body:has(&)]:m-0 [html:has(&)]:bg-surface",
      "[--shell-call:0rem] has-[[data-frame-call]]:[--shell-call:var(--shell-call-bar)]",
      "[html:has(&)]:scroll-pb-[var(--shell-foot,0px)]",
      "max-tablet:[html:has(&)]:scroll-pb-[calc(var(--shell-dock,calc(var(--shell-bar)+var(--shell-call)))+var(--shell-foot,0px))]",
      "max-tablet:[html:has(&)]:[--disclosure-toggle-inline:var(--disclosure-padding)]",
      "tablet:flex tablet:justify-center tablet:gap-shell-gap tablet:px-shell-edge desktop:gap-shell-gap-wide",
    ],
    // The rail and the column beside the main one scroll with the page, with no scrollbar of their own (useAlong).
    side: [
      "hidden tablet:sticky tablet:box-border tablet:flex tablet:min-h-dvh tablet:w-shell-rail tablet:flex-none",
      "tablet:flex-col tablet:gap-shell-edge tablet:self-start tablet:pt-shell-inset tablet:pb-shell-edge",
      "desktop:w-shell-sidebar",
    ],
    // The main column is the screen a change of screen moves (motion.tsx); the fixed bars stay over it. On a phone, its
    // end clears all the dock covers, the floating action too, so that what ends the screen (a post's More) can scroll
    // out from under it. A screen that ends in a `FrameFoot` keeps no room after it: the foot rests where it is held,
    // on the dock or the viewport's bottom, with no band between the two once the end is reached.
    main: [
      "box-border min-w-0 px-shell-edge pt-shell-gap",
      "pb-[max(calc(var(--shell-bar)+var(--shell-call)+var(--shell-end)),var(--shell-dock,0px))]",
      "has-[[data-frame-foot]]:pb-[max(calc(var(--shell-bar)+var(--shell-call)),var(--shell-dock,0px))]",
      "[view-transition-name:screen]",
      "tablet:shrink tablet:grow-0 tablet:basis-[var(--shell-column)] tablet:px-0 tablet:pb-shell-end",
      "tablet:has-[[data-frame-foot]]:pb-0",
    ],
    heading: "sr-only",
    aside: [
      "hidden text-body",
      "wide:sticky wide:box-border wide:grid wide:w-shell-aside wide:flex-none",
      "wide:content-start wide:gap-shell-section wide:self-start wide:py-shell-gap",
    ],
    // The dock lets the page take a press anywhere it shows nothing.
    dock: "pointer-events-none fixed inset-x-0 bottom-0 z-3 flex flex-col",
    // The toasts take the dock's width on a phone, above the primary action, so that a sentence reads on one or two
    // lines; from `tablet`, the column's width at most, in the middle.
    toasts: [
      "flex flex-col items-stretch gap-shell-inset px-shell-edge pb-shell-edge empty:hidden",
      "tablet:items-center tablet:pb-shell-end tablet:[&>*]:max-w-[var(--shell-column)]",
    ],
    float: [
      "pointer-events-auto me-shell-edge mb-shell-edge self-end rounded-control shadow-overlay",
      "[view-transition-name:float] tablet:hidden",
    ],
    // Positioned, so that its shadow paints over the floating action's layer (a view transition's), not under it.
    bars: "pointer-events-auto relative bg-surface pb-[env(safe-area-inset-bottom)] shadow-bar tablet:hidden",
    tabs: "[view-transition-name:tab-bar]",
    mark: [
      "flex h-shell-item flex-none items-center justify-center rounded-control outline-hidden",
      "data-pressed:translate-y-press data-focus-visible:shadow-focus desktop:justify-start desktop:px-shell-item-inline",
    ],
    markImage: "h-shell-mark",
    // An instance's own logo: square, at the mark's height, in the rail and the sidebar alike.
    logo: "size-shell-mark rounded-control object-contain",
  },
});

// A place of the navigation where the frame puts it: its arrangement there, and what it looks like there.
const nav = tv({
  slots: {
    item: "relative flex items-center",
    glyph: "relative inline-flex flex-none",
    label: "",
    // The badge on the glyph's corner, where the label is not shown beside the glyph (the rail, the tab bar), and at
    // the line's end, where it is; each place sets which shows. The corner's badge is cut out of the glyph by a ring of the surface under it, so
    // that it reads over a filled glyph (the current place's).
    corner: "absolute -top-[0.375rem] start-[calc(100%-0.5rem)] items-center justify-center shadow-face-cut",
    // The neutral mark in the corner's place.
    dot: "absolute -top-[0.125rem] start-[calc(100%-0.25rem)]",
    end: "ms-auto items-center justify-center",
  },
  variants: {
    place: {
      side: {
        item: [
          // At the full width of the rail or the sidebar, so that the menu's popover opens beside the navigation.
          "box-border h-shell-item w-full flex-none justify-center gap-shell-edge data-hovered:bg-surface-hover",
          "data-hovered:[--face-cut-color:var(--color-surface-hover)]",
          "data-pressed:translate-y-press data-pressed:bg-surface-pressed",
          "data-pressed:[--face-cut-color:var(--color-surface-pressed)]",
          // The body's leading, not the font's `normal`, which the fallback fonts set to the size alone, so that the
          // label's truncation clips no descender.
          "desktop:justify-start desktop:px-shell-item-inline desktop:text-[length:var(--shell-label)]",
          "desktop:leading-[var(--text-body--line-height)]",
        ],
        // In the rail, the label names the place for assistive technology alone.
        label: "sr-only desktop:not-sr-only desktop:truncate",
        corner: "inline-flex desktop:hidden",
        end: "hidden desktop:inline-flex",
      },
      // In the tab bar a place is its glyph alone, as in the rail: a name too small to read is worse than none
      // (ADR-0110 §11), and the label names the place for assistive technology. The item is the bar's height and its
      // share of the bar's width, with no padding of a button's own, so that the whole of it takes a press.
      tabs: {
        item: [
          "min-h-shell-bar w-full justify-center p-0 data-pressed:translate-y-press",
          "pointer-coarse:data-pressed:bg-surface-pressed",
        ],
        label: "sr-only",
        corner: "inline-flex",
        end: "hidden",
      },
      float: { label: "hidden", corner: "hidden", end: "hidden" },
      // The Menu's items are the menu's own (MenuItem).
      menu: {},
      dock: { corner: "hidden", end: "hidden" },
    },
  },
});

// What a place looks like wherever it is.
const entry = tv({
  base: [
    "rounded-control text-muted-text no-underline outline-hidden",
    "data-focus-visible:shadow-focus-inset aria-[current=page]:font-strong aria-[current=page]:text-text",
  ],
});

const badge = tv({
  base: [
    "box-border h-shell-badge min-w-shell-badge rounded-[calc(var(--shell-badge)/2)]",
    "bg-text px-shell-badge-inline text-[0.6875rem] leading-none font-label text-surface tabular-nums",
  ],
});

/**
 * The label of an entry of the frame where the frame puts it: beside the glyph in the sidebar, for assistive
 * technology alone in the rail and in the tab bar; `plain` where it always shows as it is.
 */
function EntryLabel({
  place,
  plain = false,
  hidden,
  children,
}: {
  place: Place;
  plain?: boolean;
  /** It repeats a name its entry already has: hidden from assistive technology. */
  hidden?: boolean;
  children: ReactNode;
}) {
  return (
    <span className={plain ? "" : nav({ place }).label()} aria-hidden={hidden ? "true" : undefined}>
      {children}
    </span>
  );
}

/**
 * A place of the navigation as the frame lays it out where it is, as `as` (its link, its button): in the rail its
 * glyph in the rail's middle with the count on the glyph's corner, and so in the tab bar; in the sidebar the glyph,
 * the label and the count at the line's end.
 */
function NavEntry({
  place,
  as,
  glyph,
  corner,
  dot,
  label,
  said,
  end,
}: {
  place: Place;
  as: ReactElement<{ className?: unknown }>;
  glyph: ReactNode;
  /** The count on the glyph's corner, as its element. */
  corner?: ReactElement<{ className?: unknown }> | undefined;
  /** A mark in the corner's place, as its element. */
  dot?: ReactElement<{ className?: unknown }> | undefined;
  label: string;
  /** What assistive technology reads after the label. */
  said?: string | undefined;
  /** The count at the line's end, as its element. */
  end?: ReactElement<{ className?: unknown }> | undefined;
}) {
  const s = nav({ place });
  return (
    <Box as={as} className={s.item()}>
      <span className={s.glyph()}>
        {glyph}
        {corner !== undefined && <Box as={corner} className={s.corner()} />}
        {dot !== undefined && <Box as={dot} className={s.dot()} />}
      </span>
      <EntryLabel place={place}>{label}</EntryLabel>
      {said !== undefined && (
        <>
          {" "}
          <span className="sr-only">{said}</span>
        </>
      )}
      {end !== undefined && <Box as={end} className={s.end()} />}
    </Box>
  );
}

/** A count as its badge shows it: up to 99, then "99+", in the reader's numbers. */
function countText(count: number, lang: string): string {
  const format = new Intl.NumberFormat(lang);
  return count > 99 ? `${format.format(99)}+` : format.format(count);
}

/**
 * Keeps a column of the frame (the rail, the column beside the main one) moving with the page's scroll, as a sidebar
 * with no scrollbar of its own: one no taller than the viewport stays at its top; a taller one scrolls with the page
 * until its far edge meets the viewport's, then stays there, in either direction. It is sticky, its `top` moved by
 * as much as the page scrolled and held between the viewport's height less its own (its end at the viewport's end)
 * and 0 (its start at the viewport's start). A scroll reads only the scroll's position and writes `top`, in an
 * animation frame; its height comes from a ResizeObserver. The focus moving into the column scrolls the page, which
 * moves the column the way the focus needs.
 */
function useAlong(ref: RefObject<HTMLElement | null>): void {
  useEffect(() => (ref.current ? along(ref.current) : undefined), [ref]);
}

/** Moves `el` along with the page's scroll (useAlong) until the returned function stops it. */
function along(el: HTMLElement): () => void {
  let height = el.offsetHeight;
  let lastY = window.scrollY;
  let top = 0;
  let frame = 0;
  const place = (next: number) => {
    const held = Math.min(0, Math.max(window.innerHeight - height, next));
    if (held === top && el.style.top !== "") return;
    top = held;
    el.style.top = `${String(top)}px`;
  };
  const onScroll = () => {
    if (frame !== 0) return;
    frame = requestAnimationFrame(() => {
      frame = 0;
      const y = window.scrollY;
      place(top - (y - lastY));
      lastY = y;
    });
  };
  const resize = () => {
    height = el.offsetHeight;
    place(top);
  };
  const sizes = new ResizeObserver(resize);
  sizes.observe(el);
  place(top);
  window.addEventListener("scroll", onScroll, { passive: true });
  window.addEventListener("resize", resize);
  return () => {
    cancelAnimationFrame(frame);
    sizes.disconnect();
    window.removeEventListener("scroll", onScroll);
    window.removeEventListener("resize", resize);
    el.style.top = "";
  };
}

/**
 * Keeps `--shell-dock` on the document the height the dock covers at the bottom of the viewport on a phone, from the
 * top of the floating action (or of the bars, where no action floats) to the viewport's end, which the bars' tokens
 * alone do not tell where an action floats over them. It is unset while neither shows (from `tablet`).
 */
function useDockHeight(
  float: RefObject<HTMLElement | null>,
  bars: RefObject<HTMLElement | null>,
  floats: boolean,
): void {
  useEffect(() => {
    const root = document.documentElement;
    const measure = () => {
      const top = [float.current, bars.current]
        .filter((el): el is HTMLElement => el !== null && el.offsetHeight > 0)
        .map((el) => el.getBoundingClientRect().top);
      if (top.length === 0) root.style.removeProperty("--shell-dock");
      else root.style.setProperty("--shell-dock", `${String(Math.ceil(window.innerHeight - Math.min(...top)))}px`);
    };
    const sizes = new ResizeObserver(measure);
    for (const el of [float.current, bars.current]) if (el !== null) sizes.observe(el);
    measure();
    window.addEventListener("resize", measure);
    return () => {
      sizes.disconnect();
      window.removeEventListener("resize", measure);
      root.style.removeProperty("--shell-dock");
    };
  }, [float, bars, floats]);
}

/** The frame of every screen of the app, which holds its regions. */
export function Frame({ children }: { children: ReactNode }) {
  return <div className={frame().root()}>{children}</div>;
}

/**
 * The rail, from `tablet`, which grows into the sidebar from `desktop`: the mark, the primary action, the lists of
 * places (the reader's, then those of their rights, apart), and the call's bar at its end.
 */
export function FrameSide({ label, children }: { label: string; children: ReactNode }) {
  const ref = useRef<HTMLElement>(null);
  useAlong(ref);
  return (
    <nav ref={ref} className={frame().side()} aria-label={label}>
      <PlaceContext value="side">{children}</PlaceContext>
    </nav>
  );
}

/**
 * The dock at the bottom of the viewport. On a phone: the tab bar (`tabs`, a navigation named `label`), the call's bar
 * above it, the primary action floating at the end above both (`action`, on the screens that have one), and the toasts
 * above all of them. From `tablet`, the toasts alone, in the middle.
 */
export function FrameDock({
  label,
  tabs,
  call,
  action,
  toasts,
}: {
  label: string;
  tabs: ReactNode;
  call?: ReactNode;
  action?: ReactNode;
  toasts?: ReactNode;
}) {
  const s = frame();
  const floatRef = useRef<HTMLDivElement>(null);
  const barsRef = useRef<HTMLDivElement>(null);
  useDockHeight(floatRef, barsRef, action !== undefined);
  return (
    <div className={s.dock()}>
      <div className={s.toasts()}>{toasts}</div>
      {action !== undefined && (
        <div ref={floatRef} className={s.float()}>
          <PlaceContext value="float">{action}</PlaceContext>
        </div>
      )}
      <div ref={barsRef} className={s.bars()}>
        {call !== undefined && <PlaceContext value="dock">{call}</PlaceContext>}
        <nav className={s.tabs()} aria-label={label}>
          <PlaceContext value="tabs">{tabs}</PlaceContext>
        </nav>
      </div>
    </div>
  );
}

/** The main column: the screen's own content, at the full width on a phone and at the column's width beside the rail. */
export function FrameMain({ children }: { children: ReactNode }) {
  return <main className={frame().main()}>{children}</main>;
}

/**
 * A raised surface of the main column, as `as` (a section of a settings screen, a group of the settings' menu), its
 * parts one under another a `gap` apart: on a phone at the viewport's width, so that what it holds keeps the column's
 * width and its edges; from `tablet` inside the column, its corners rounded. Give `as` the mark of a raised surface
 * (`raisedSurface`), so that a control on it takes the page's surface.
 */
export function FramePanel({
  as,
  gap,
  children,
}: {
  as: ReactElement<{ className?: unknown }>;
  gap: Gap;
  children: ReactNode;
}) {
  return (
    <Stack
      as={as}
      gap={gap}
      className="-mx-shell-edge bg-surface-raised p-shell-edge tablet:mx-0 tablet:rounded-overlay"
    >
      {children}
    </Stack>
  );
}

/**
 * The head of a screen that the column beside the main one names from `wide` (a settings screen beside the menu that
 * marks its entry): its title and the way back, under `wide` alone.
 */
export function FrameHead({ children }: { children: ReactNode }) {
  return (
    <div className="wide:hidden" data-frame-head="">
      {children}
    </div>
  );
}

/**
 * Keeps `--shell-foot` on the document the height of what is held at the viewport's bottom over the screen (`el`),
 * which the frame adds to the page's scroll padding at the end: what takes the focus under it scrolls clear of it.
 */
function useFootHeight(el: RefObject<HTMLElement | null>): void {
  useEffect(() => {
    const foot = el.current;
    if (foot === null) return;
    const root = document.documentElement;
    const measure = () => {
      root.style.setProperty("--shell-foot", `${String(Math.ceil(foot.getBoundingClientRect().height))}px`);
    };
    const sizes = typeof ResizeObserver === "undefined" ? undefined : new ResizeObserver(measure);
    sizes?.observe(foot);
    measure();
    return () => {
      sizes?.disconnect();
      root.style.removeProperty("--shell-foot");
    };
  }, [el]);
}

/**
 * What ends a long screen and stays in reach (the submit bar of a form saved as a whole): at the screen's end, and
 * held at the bottom of the viewport, above the dock on a phone, while the end is further down; on the page's surface
 * at the column's width, the viewport's on a phone, over what scrolls under it. What takes the focus under it scrolls
 * clear of it (`--shell-foot`).
 */
export function FrameFoot({ children }: { children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null);
  useFootHeight(ref);
  return (
    <div
      ref={ref}
      data-frame-foot=""
      className={[
        "sticky bottom-0 z-1 -mx-shell-edge bg-surface px-shell-edge py-shell-inset",
        "max-tablet:bottom-[var(--shell-dock,calc(var(--shell-bar)+var(--shell-call)))] tablet:mx-0 tablet:px-0",
      ].join(" ")}
    >
      {children}
    </div>
  );
}

/**
 * The screen's heading, for assistive technology only, since the navigation shows where the reader is. It takes the
 * focus when the app moves it there after a change of screen (ADR-0110 §10).
 */
export function FrameHeading({ id, children }: { id?: string; children: ReactNode }) {
  return (
    <h1 id={id} className={frame().heading()} tabIndex={-1} data-frame-heading="">
      {children}
    </h1>
  );
}

/** Moves the focus to the screen's heading (FrameHeading) in `doc`, without scrolling the page. */
export function focusFrameHeading(doc: Document): void {
  doc.querySelector<HTMLElement>("[data-frame-heading]")?.focus({ preventScroll: true });
}

/** The column beside the main one, from `wide`: what is around the reader, in sections, and the footer at its end. */
export function FrameAside({ label, children }: { label: string; children: ReactNode }) {
  const ref = useRef<HTMLElement>(null);
  useAlong(ref);
  return (
    <aside ref={ref} className={frame().aside()} aria-label={label}>
      {children}
    </aside>
  );
}

const section = tv({
  slots: {
    head: "text-caption",
    heading: "text-caption font-normal",
    link: [
      "rounded-control py-target-line text-muted-text no-underline outline-hidden",
      "data-hovered:text-text data-hovered:underline",
      "data-hovered:underline-offset-[0.15em] data-focus-visible:shadow-focus",
    ],
    all: [
      "rounded-control py-target-line text-accent-text no-underline outline-hidden",
      "data-hovered:underline data-hovered:underline-offset-[0.15em] data-focus-visible:shadow-focus",
    ],
  },
});

/**
 * A section of what is around the reader, in the column beside the main one, under a heading that links to its place,
 * with that place's glyph. `all` ends the heading's line with a link to all of it when the section shows only some:
 * its `text` shown, and its `name` for assistive technology, which starts with the text and names the place ("All
 * announcements").
 */
export function FrameSection({
  id,
  icon,
  href,
  heading,
  all,
  children,
}: {
  /** The heading's id, which names the section. */
  id: string;
  icon: IconName;
  href: string;
  heading: string;
  all?: { text: string; name: string } | undefined;
  children: ReactNode;
}) {
  const s = section();
  return (
    <Stack as="section" pack="start" gap="shell-inset" aria-labelledby={id}>
      {/* The heading names the section alone; the link to all of it ends the heading's line, outside the heading. */}
      <Row align="center" justify="between" gap="shell-inset" className={s.head()}>
        <h2 id={id} className={s.heading()}>
          <Row inline align="center" gap="shell-inset" as={<Link className={s.link()} href={href} />}>
            <Icon name={icon} place="caption" />
            {heading}
          </Row>
        </h2>
        {all !== undefined && (
          <Link className={s.all()} href={href} aria-label={all.name}>
            {all.text}
          </Link>
        )}
      </Row>
      {children}
    </Stack>
  );
}

const fold = tv({
  slots: {
    // Compact and neutral, on the raised surface; its target reaches the minimum, and a finger's on a touch screen.
    root: [
      "box-border rounded-overlay bg-surface-raised px-embed",
      "py-target-line text-body text-text no-underline outline-hidden",
      "pointer-coarse:min-h-icon-button-touch data-hovered:bg-surface-raised-hover",
      "data-pressed:translate-y-press data-pressed:bg-surface-raised-hover data-focus-visible:shadow-focus",
    ],
    glyph: "text-muted-text",
  },
});

/**
 * The line a section folds to above a feed, as `as`: its parts in its middle, the frame's gap under it; under `wide`
 * alone, where no column beside the main one holds the section.
 */
function FoldRow({ as, children }: { as: ReactElement<{ className?: unknown }>; children: ReactNode }) {
  return (
    <Row as={as} align="center" gap="entry-caption" spaceBelow="shell-gap" className="wide:hidden">
      {children}
    </Row>
  );
}

/**
 * A section folded to one line above a feed under `wide`, where no column beside the main one holds it: its place's
 * glyph, what it holds in a few words ("2 unread announcements"), and the way onward; one link to its place. Hidden
 * from `wide`, where the column beside shows the section.
 */
export function FrameFold({ icon, href, children }: { icon: IconName; href: string; children: ReactNode }) {
  const s = fold();
  return (
    <FoldRow as={<Link className={s.root()} href={href} />}>
      <Row as="span" inline rigid className={s.glyph()}>
        <Icon name={icon} place="caption" />
      </Row>
      <Box as="span" grow>
        {children}
      </Box>
      <Row as="span" inline rigid className={s.glyph()}>
        <Icon name="onward" place="caption" />
      </Row>
    </FoldRow>
  );
}

const line = tv({
  slots: {
    list: "list-none p-0",
    // A title that wraps breaks between phrases, never inside a word of ja (求めています, リアクション), as NoticeLine.
    title: [
      "rounded-control text-text no-underline outline-hidden [word-break:auto-phrase] data-hovered:underline",
      "data-hovered:underline-offset-[0.15em] data-focus-visible:shadow-focus",
    ],
    caption: "text-caption text-muted-text",
    glyph: "text-muted-text",
    note: "text-caption text-muted-text",
  },
  variants: {
    read: { true: { title: "text-muted-text" } },
  },
});

/**
 * What a section says in place of its lines, in a caption's size: they are loading, there are none, or they failed,
 * with `action` after it (a button to try again).
 */
export function SectionNote({ action, children }: { action?: ReactNode; children: string }) {
  return (
    <Row as="p" wrap align="baseline" gapX="shell-edge" className={line().note()}>
      <span>{children}</span>
      {action}
    </Row>
  );
}

/** The lines of a section: what it lists, in order. */
export function SectionList({ children }: { children: ReactNode }) {
  return (
    <Stack as="ul" gap="shell-inset" className={line().list()}>
      {children}
    </Stack>
  );
}

/**
 * A line of a section: its title, a link to it, on two lines at most, with a caption under it (a room's host and
 * listeners, an event's time), and the glyph of its kind before it when it has one (a notification's), or an emoji of
 * its own (a room's). `unread` marks it with the unread mark at its end, in the text's color; `read` mutes it.
 * `onPress` runs as the link is followed.
 */
export function SectionLine({
  href,
  title,
  caption,
  icon,
  glyph,
  unread,
  read = false,
  onPress,
}: {
  href: string;
  title: ReactNode;
  caption?: ReactNode;
  icon?: IconName | undefined;
  /** An emoji of its own in place of a kind's glyph (a room's icon emoji), hidden from assistive technology. */
  glyph?: ReactNode;
  /** The unread mark's label while it is unread. */
  unread?: string | undefined;
  read?: boolean;
  onPress?: (() => void) | undefined;
}) {
  const s = line({ read });
  return (
    <Row as="li" align="start" gap="entry-caption">
      {icon !== undefined && (
        <Row as="span" inline line rigid align="center" className={s.glyph()}>
          <Icon name={icon} place="caption" />
        </Row>
      )}
      {icon === undefined && glyph !== undefined && (
        <Row as="span" inline line rigid align="center" aria-hidden="true">
          {glyph}
        </Row>
      )}
      <Stack as="span" grow>
        <Box
          lines={2}
          as={<Link className={s.title()} href={href} dir="auto" {...(onPress === undefined ? {} : { onPress })} />}
        >
          {title}
        </Box>
        {caption !== undefined && <span className={s.caption()}>{caption}</span>}
      </Stack>
      {unread !== undefined && (
        <Row as="span" inline line rigid align="center">
          <UnreadMark label={unread} />
        </Row>
      )}
    </Row>
  );
}

const meter = tv({
  slots: {
    figure: "text-title tabular-nums",
    track: "h-shell-meter rounded-[calc(var(--shell-meter)/2)] bg-surface-raised-hover",
    fill: "rounded-[calc(var(--shell-meter)/2)] bg-text",
    caption: "text-caption text-muted-text",
  },
});

/**
 * A meter of how much of a goal is met (the month's costs covered): the share as a figure, a bar in the text's color on
 * the raised surface, full at the goal and beyond, and a caption of what it measures.
 */
export function Meter({ value, label, caption }: { value: number; label: string; caption: ReactNode }) {
  const s = meter();
  const { lang } = useReader(undefined);
  const shown = new Intl.NumberFormat(lang, { style: "percent", maximumFractionDigits: 0 }).format(value);
  // The bar and the meter's value stay within its range, full at the goal and beyond; the figure says the share itself.
  const filled = Math.round(Math.min(1, Math.max(0, value)) * 100);
  return (
    <Stack gap="shell-inset">
      <span className={s.figure()} aria-hidden="true">
        {shown}
      </span>
      <Box
        clip
        className={s.track()}
        role="meter"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={filled}
        aria-valuetext={shown}
      >
        <Box full="block" share={filled / 100} className={s.fill()} />
      </Box>
      <span className={s.caption()}>{caption}</span>
    </Stack>
  );
}

const footer = tv({
  slots: {
    root: "text-caption text-muted-text",
    list: "list-none p-0",
    // A link's target reaches the minimum around its text without moving the line (WCAG 2.5.8).
    link: [
      "rounded-control py-target-line text-muted-text no-underline",
      "outline-hidden data-hovered:text-text data-hovered:underline data-focus-visible:shadow-focus",
    ],
  },
});

/**
 * The server's pages (about, terms, privacy) at the end of the column beside the main one, or of the Menu, where they
 * are its last group of items, each by its glyph.
 */
export function FrameFooter({
  label,
  links,
}: {
  label: string;
  links: { href: string; label: string; icon: IconName }[];
}) {
  const s = footer();
  if (use(PlaceContext) === "menu")
    return (
      <MenuGroup>
        {links.map((l) => (
          <MenuItem key={l.href} id={`footer:${l.href}`} icon={l.icon} href={l.href}>
            {l.label}
          </MenuItem>
        ))}
      </MenuGroup>
    );
  return (
    <footer className={s.root()}>
      <nav aria-label={label}>
        <Row as="ul" wrap gapX="shell-edge" className={s.list()}>
          {links.map((l) => (
            <li key={l.href}>
              <Box display="inline-block" as={<Link className={s.link()} href={l.href} />}>
                {l.label}
              </Box>
            </li>
          ))}
        </Row>
      </nav>
    </footer>
  );
}

const banner = tv({
  slots: {
    root: "rounded-overlay px-embed py-shell-inset text-body",
    links: "list-none p-0",
    link: [
      "rounded-control py-target-line font-label underline",
      "underline-offset-[0.15em] outline-hidden data-focus-visible:shadow-focus",
    ],
  },
  variants: {
    // What the reader must act on or cannot work around (the closure, the server down): knocked out of the text's
    // color, as a form's alert is, its links in the page's color; else on the raised surface, its links the accent's.
    strong: {
      true: { root: "bg-text text-surface", link: "text-surface" },
      false: { root: "bg-surface-raised", link: "text-accent-text" },
    },
  },
});

/**
 * The notices of the server at the top of the main column, on every screen, named `label` for assistive technology:
 * the closure first, then the reader offline or the server down.
 */
export function FrameBanners({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Stack gap="shell-inset" spaceBelow="shell-gap" role="region" aria-label={label}>
      {children}
    </Stack>
  );
}

/**
 * A notice of the server, which the reader cannot dismiss: its glyph, what it says, and the links of what to do (export
 * the data, move, the status page), compact, so that two of them leave the screen to the screen. `strong` knocks out
 * what the reader must act on or cannot work around (the closure, the server down); the reader offline is plain. A link
 * with `newTab` (the words that say so, for assistive technology) leaves the app in a new tab, as the status page does.
 */
export function FrameBanner({
  icon,
  strong = false,
  links = [],
  children,
}: {
  icon: IconName;
  strong?: boolean;
  links?: { href: string; label: string; newTab?: string | undefined }[];
  children: ReactNode;
}) {
  const s = banner({ strong });
  return (
    <Row align="start" gap="entry-avatar" className={s.root()}>
      <Row as="span" inline line rigid align="center">
        <Icon name={icon} place="action" />
      </Row>
      <Stack grow>
        <p dir="auto">{children}</p>
        {links.length > 0 && (
          <Row as="ul" wrap gapX="shell-edge" className={s.links()}>
            {links.map((l) => (
              <li key={l.href}>
                <Box
                  display="inline-block"
                  as={
                    <Link
                      className={s.link()}
                      href={l.href}
                      {...(l.newTab === undefined ? {} : { target: "_blank", rel: "noopener noreferrer" })}
                    />
                  }
                >
                  {l.label}
                  {l.newTab !== undefined && <span className="sr-only"> {l.newTab}</span>}
                </Box>
              </li>
            ))}
          </Row>
        )}
      </Stack>
    </Row>
  );
}

const toast = tv({
  slots: {
    // In the dock, which holds it to the column's width from `tablet`.
    root: [
      "pointer-events-auto box-border rounded-overlay bg-text py-shell-toast-inset",
      "ps-control-timeline-inline pe-shell-toast-inset font-sans text-body text-surface shadow-overlay",
    ],
    action: [
      "h-control-timeline cursor-pointer rounded-control border-0 bg-transparent px-control-timeline-inline",
      "font-sans text-body font-label text-inherit outline-hidden hover:underline active:translate-y-press",
      "focus-visible:shadow-[inset_0_0_0_var(--focus-ring-width)_var(--color-surface)]",
    ],
    close: [
      "size-control-timeline cursor-pointer rounded-control border-0",
      "bg-transparent p-0 text-inherit outline-hidden hover:bg-[color-mix(in_srgb,var(--color-surface)_15%,transparent)]",
      "active:translate-y-press",
      "focus-visible:shadow-[inset_0_0_0_var(--focus-ring-width)_var(--color-surface)]",
    ],
  },
});

/**
 * A toast in the dock (a newer version of the app, with Update): knocked out and neutral, as every short notice at the
 * bottom is. It stays until it is acted on or, when it has `onClose`, closed by its button at the end.
 */
export function Toast({
  action,
  onAction,
  closeLabel,
  onClose,
  children,
}: {
  action: string;
  onAction: () => void;
  closeLabel?: string | undefined;
  onClose?: (() => void) | undefined;
  children: ReactNode;
}) {
  const s = toast();
  return (
    <Row align="center" gap="entry-avatar" max="full" className={s.root()} role="status">
      <Box as="span" grow>
        {children}
      </Box>
      <Box rigid as={<button type="button" className={s.action()} onClick={onAction} />}>
        {action}
      </Box>
      {onClose !== undefined && (
        <Row
          inline
          rigid
          align="center"
          justify="center"
          as={<button type="button" className={s.close()} onClick={onClose} aria-label={closeLabel} />}
        >
          <Icon name="close" place="action" />
        </Row>
      )}
    </Row>
  );
}

/**
 * The mark's place at the top of the rail, as `as`: in the rail's middle, and from `desktop` at the sidebar's start,
 * on the column of the places' glyphs.
 */
function MarkPlace({ as, children }: { as: ReactElement<{ className?: unknown }>; children: ReactNode }) {
  return (
    <Box as={as} className={frame().mark()}>
      {children}
    </Box>
  );
}

/**
 * The mark at the top of the rail, a link to the app's start: the wordmark in its version for light and for dark, or
 * the instance's own logo in its place.
 */
export function FrameMark({
  href,
  label,
  light,
  dark,
  logo,
}: {
  href: string;
  label: string;
  light: string;
  dark: string;
  /** The instance's own logo, which takes the mark's place (ADR-0059 §8). */
  logo?: string | undefined;
}) {
  const s = frame();
  return (
    <MarkPlace as={<Link href={href} aria-label={label} />}>
      {logo !== undefined && logo !== "" ? (
        <img className={s.logo()} src={logo} alt="" />
      ) : (
        <>
          <Box only="light" as={<img className={s.markImage()} src={light} alt="" />} />
          <Box only="dark" as={<img className={s.markImage()} src={dark} alt="" />} />
        </>
      )}
    </MarkPlace>
  );
}

/**
 * The screen's primary action in the frame (writing a post, or signing in): under the mark in the rail, with its label
 * in the sidebar, or floating over the tab bar: a glyph alone named by its label, or with its label when `labelled`
 * (signing in, which a glyph alone does not say). With `menu`, a long press, a right click or Alt+Down on it opens a
 * menu of what it can start (a new post, a new article), the one menu; a press still runs the action. `secondary`
 * is a second thing to start (a new article), tonal under the primary one in `FrameActions`.
 */
export function FrameAction({
  icon,
  label,
  onPress,
  labelled = false,
  secondary = false,
  menu,
  "aria-keyshortcuts": keyshortcuts,
  "data-action": action,
}: {
  icon: IconName;
  label: string;
  onPress: () => void;
  labelled?: boolean;
  /** On the raised surface, tonal: a second action under the primary one, which keeps the accent. */
  secondary?: boolean;
  /** The menu the action holds besides itself: its name and its items (`MenuItem`s). */
  menu?: { label: string; children: ReactNode } | undefined;
  /** The keys that run the action (what `useAction` returns), for assistive technology. */
  "aria-keyshortcuts"?: string | undefined;
  /** The ID of the action (what `useAction` returns), by which its press is timed. */
  "data-action"?: string | undefined;
}) {
  const place = use(PlaceContext);
  const float = place === "float";
  const [open, setOpen] = useState(false);
  const button = (
    <ButtonBase
      kind={secondary ? "secondary" : "primary"}
      size={float ? (labelled ? "form" : "float") : "side"}
      aria-label={label}
      aria-keyshortcuts={keyshortcuts}
      data-action={action}
      onPress={onPress}
    >
      <Row as="span" inline align="center" gap="shell-inset">
        <Icon name={icon} place="navigation" />
        <EntryLabel place={place} plain={float && labelled} hidden>
          {label}
        </EntryLabel>
      </Row>
    </ButtonBase>
  );
  if (menu === undefined) return button;
  return (
    <div
      className="contents"
      onContextMenu={(e) => {
        e.preventDefault();
        setOpen(true);
      }}
    >
      <MenuSurface
        trigger={button}
        label={menu.label}
        longPress
        placement={float ? "top end" : "bottom start"}
        open={open}
        onOpenChange={setOpen}
      >
        {menu.children}
      </MenuSurface>
    </div>
  );
}

/**
 * The actions under the mark in the rail and the sidebar, close together as buttons of one row sit: the primary one
 * (New post), then a second one (New article), tonal.
 */
export function FrameActions({ children }: { children: ReactNode }) {
  return <Stack gap="shell-item-gap">{children}</Stack>;
}

/** A list of places of the app, in the rail, the sidebar, the tab bar or the Menu (a group); two lists stand apart. */
export function NavList({ children }: { children: ReactNode }) {
  const place = use(PlaceContext);
  if (place === "menu") return <MenuGroup>{children}</MenuGroup>;
  if (place === "tabs")
    return (
      <Row as="ul" align="stretch" gap="screen-tab-gap" className="list-none p-0 px-screen-tab-gap">
        {children}
      </Row>
    );
  return (
    <Column as="ul" gap="shell-item-gap" className="list-none p-0">
      {children}
    </Column>
  );
}

/**
 * A place's entry in the list of its bar: in the tab bar the places share the bar's width evenly, four of them as
 * six, none narrower than the least target a finger takes.
 */
function NavPlace({ children }: { children: ReactNode }) {
  if (use(PlaceContext) === "tabs")
    return (
      <Row as="li" grow className="min-w-icon-button-touch">
        {children}
      </Row>
    );
  return <li>{children}</li>;
}

/** A count's badge, hidden from assistive technology, which reads the count in words. */
const countBadge = (shown: string) => (
  <span className={badge()} aria-hidden="true">
    {shown}
  </span>
);

/**
 * A place: its glyph, on where the reader is, and its label, which the sidebar shows beside the glyph and which
 * names the place for assistive technology alone in the rail and in the tab bar. `count` puts a badge on it (unread notifications, reports to handle), which `countLabel` names.
 */
export function NavItem({
  href,
  icon,
  label,
  current = false,
  count,
  countLabel,
}: {
  href: string;
  icon: IconName;
  label: string;
  /** Whether the reader is at this place. */
  current?: boolean;
  /** How many wait there; none or zero shows no badge. */
  count?: number | undefined;
  /** What the count is, for assistive technology ("3 unread"). */
  countLabel?: string | undefined;
}) {
  const place = use(PlaceContext);
  const { lang } = useReader(undefined);
  const shown = count !== undefined && count > 0 ? countText(count, lang) : undefined;
  if (place === "menu") {
    return (
      <MenuItem
        id={href}
        icon={icon}
        href={href}
        current={current}
        badge={
          shown === undefined ? undefined : (
            <>
              <Row as="span" inline align="center" justify="center" className={badge()} aria-hidden="true">
                {shown}
              </Row>
              {countLabel !== undefined && <span className="sr-only">{countLabel}</span>}
            </>
          )
        }
      >
        {label}
      </MenuItem>
    );
  }
  return (
    <NavPlace>
      <NavEntry
        place={place}
        as={<Link className={entry()} href={href} {...(current ? { "aria-current": "page" as const } : {})} />}
        glyph={<Icon name={icon} place="navigation" on={current} />}
        corner={shown === undefined ? undefined : countBadge(shown)}
        label={label}
        said={shown === undefined ? undefined : countLabel}
        end={shown === undefined ? undefined : countBadge(shown)}
      />
    </NavPlace>
  );
}

// The neutral mark on the menu's glyph while a place inside it has a count.
const dot = tv({ base: "size-screen-unread rounded-[50%] bg-text" });

/**
 * The Menu: a place of the tab bar on a phone, and the last place of the rail and the sidebar, that opens what the
 * navigation does not hold, the one menu (menu.tsx): its places a group per NavList, then the footer's pages, as items
 * that lead there; a sheet from the bottom over the dimmed page on a phone, a popover beside its entry from `tablet`,
 * toward the content and never flipped over the navigation. Escape, a press outside, close and a place's press close
 * it. `marked` puts a neutral mark on its glyph while a place inside it has a count, which `markLabel` names for
 * assistive technology.
 */
export function NavMenu({
  label,
  marked = false,
  markLabel,
  count,
  open,
  onOpenChange,
  footer: end,
  children,
}: {
  label: string;
  marked?: boolean;
  markLabel?: string | undefined;
  /**
   * How many wait in a place inside it that the reader reads (the unread direct messages): a badge in place of the
   * mark, which `markLabel` names.
   */
  count?: number | undefined;
  /** The menu is open, when its owner opens it too, with `onOpenChange`; else its entry alone does. */
  open?: boolean | undefined;
  onOpenChange?: ((open: boolean) => void) | undefined;
  footer?: ReactNode;
  children: ReactNode;
}) {
  const place = use(PlaceContext);
  const { lang } = useReader(undefined);
  const shown = count !== undefined && count > 0 ? countText(count, lang) : undefined;
  const trigger = (
    <NavEntry
      place={place}
      as={<AriaButton className={`${entry()} cursor-pointer border-0 bg-transparent font-sans`} />}
      glyph={<Icon name="menu" place="navigation" />}
      corner={shown === undefined ? undefined : countBadge(shown)}
      dot={shown === undefined && marked ? <span className={dot()} aria-hidden="true" /> : undefined}
      label={label}
      said={marked || shown !== undefined ? markLabel : undefined}
      end={shown === undefined ? undefined : countBadge(shown)}
    />
  );
  return (
    <NavPlace>
      {/* From `tablet`, toward the content, its end at the entry's; it mirrors from right to left. */}
      <MenuSurface
        trigger={trigger}
        label={label}
        placement="end bottom"
        offset={8}
        open={open}
        onOpenChange={onOpenChange}
      >
        <PlaceContext value="menu">
          {children}
          {end}
        </PlaceContext>
      </MenuSurface>
    </NavPlace>
  );
}

// Every control of the call's bar is a timeline control's size, and reaches the touch size on a touch screen.
const callTarget = "size-control-timeline pointer-coarse:size-icon-button-touch";

// The call's bar where the frame puts it: its arrangement there, and what changes of its look with it.
const call = tv({
  slots: {
    root: "flex items-center gap-entry-avatar",
    room: "flex min-w-0 flex-1 items-center gap-entry-avatar",
    glyph: "inline-flex flex-none",
    lines: "grid min-w-0 flex-1 leading-entry",
    // The name's line: the name, cut first, then how many are in the room, never cut.
    head: "flex min-w-0 items-baseline gap-entry-caption",
    name: "min-w-0 truncate",
    count: "flex-none",
    detail: "truncate",
    actions: "flex flex-none items-center gap-shell-inset",
    button: ["box-border inline-flex flex-none items-center justify-center gap-icon-button-gap", callTarget],
    buttonLabel: "sr-only",
    // Turning on the sound the browser held back: a button of its glyph and its words wherever there is room for them.
    hear: "box-border inline-flex flex-none items-center justify-center gap-icon-button-gap",
    hearLabel: "",
    tooltip: "max-w-[var(--menu-min-width)]",
  },
  variants: {
    place: {
      // Above the tab bar, at its full width, on the raised surface.
      dock: {
        root: "bg-surface-raised ps-shell-edge pe-shell-inset",
        button: "bg-transparent data-hovered:bg-surface-raised-hover data-pressed:bg-surface-raised-pressed",
        hear: "h-control-timeline px-control-timeline-inline pointer-coarse:h-icon-button-touch",
      },
      // At the rail's end: a raised tile of three controls on the page's surface, apart, each pressable at rest: the
      // room's link (its name in a tooltip), then its two buttons; from `desktop`, a raised card of the room's name and
      // detail, a link underlined on hover, over its two buttons, each with its label, side by side.
      side: {
        root: [
          "mt-auto flex-none flex-col items-center gap-shell-inset rounded-overlay bg-surface-raised py-shell-inset",
          "desktop:items-stretch desktop:p-shell-inset",
        ],
        room: [
          "flex-none justify-center bg-surface data-hovered:bg-surface-hover data-pressed:bg-surface-pressed",
          callTarget,
          "desktop:size-auto desktop:justify-start desktop:bg-transparent desktop:px-shell-inset desktop:pt-shell-inset",
          "desktop:data-hovered:bg-transparent desktop:data-pressed:bg-transparent",
          "desktop:data-hovered:[&_[dir]]:underline",
        ],
        // In the rail, the room's name and detail name its link for assistive technology, and its tooltip shows them.
        lines: "sr-only desktop:not-sr-only desktop:grid",
        actions: "flex-col desktop:flex-row desktop:flex-wrap",
        // In the rail, a tile of its glyph alone, named; from `desktop`, a line of its own over the two buttons.
        hear: [
          callTarget,
          "desktop:h-control-timeline desktop:w-auto desktop:basis-full desktop:px-control-timeline-inline",
          "desktop:pointer-coarse:h-icon-button-touch",
        ],
        hearLabel: "sr-only desktop:not-sr-only",
        button: [
          "bg-surface data-hovered:bg-surface-hover data-pressed:bg-surface-pressed",
          "desktop:h-control-timeline desktop:w-auto desktop:flex-1 desktop:px-control-timeline-inline",
          "desktop:pointer-coarse:h-icon-button-touch",
        ],
        buttonLabel: "desktop:not-sr-only",
        tooltip: "desktop:hidden",
      },
      tabs: {},
      float: {},
      menu: {},
    },
    // The browser holds the room's sound back: above the tab bar, hearing takes a line of its own over the room and
    // its two buttons, at the bar's width, so that the room's name and count keep their room.
    held: { true: {}, false: {} },
  },
  compoundVariants: [
    { place: "dock", held: false, class: { root: "h-shell-call-bar" } },
    {
      place: "dock",
      held: true,
      class: { root: "min-h-shell-call-bar flex-wrap py-shell-inset", hear: "basis-full" },
    },
  ],
});

// What the parts of the call's bar look like wherever it is. The microphone muted is knocked out, with its crossed
// glyph: it reads at rest, without a hue, as the room's own microphone control does (voice.tsx).
const callLook = tv({
  slots: {
    room: [
      "rounded-control text-text no-underline outline-hidden data-pressed:translate-y-press",
      "data-focus-visible:shadow-focus",
    ],
    name: "font-label",
    detail: "text-caption text-muted-text",
    button: [
      "cursor-pointer rounded-control",
      "border-0 p-0 font-sans text-caption font-label text-text outline-hidden data-pressed:translate-y-press",
      "data-focus-visible:shadow-focus",
      "data-on:bg-text data-on:text-surface data-on:data-hovered:bg-text data-on:data-pressed:bg-text",
    ],
    // Knocked out of the text's color, as a toggle on is: it is what the reader must do to hear the room.
    hear: [
      "cursor-pointer rounded-control border-0 bg-text font-sans text-caption font-label text-surface",
      "outline-hidden data-pressed:translate-y-press data-focus-visible:shadow-focus",
    ],
    tooltip: "rounded-control bg-text px-shell-inset py-shell-toast-inset text-caption text-surface",
    // The room's glyph while the call connects again: a spinner of its size.
    spinner: [
      "box-border size-icon-action rounded-full border-spinner border-solid border-spinner-track border-t-current",
      "animate-spin",
      motion.spin.reduced === "still" ? "motion-reduce:animate-none" : "",
    ],
  },
});

/**
 * A part of the call's bar where the frame puts the bar, as `as`: above the tab bar a row at the dock's width; at the
 * rail's end a tile of its controls one under another, and from `desktop` a card of the room's name and detail over
 * its two buttons side by side, each with its label.
 */
function CallPart({
  part,
  as,
  held = false,
  children,
}: {
  part:
    | "root"
    | "room"
    | "glyph"
    | "lines"
    | "head"
    | "name"
    | "count"
    | "detail"
    | "actions"
    | "button"
    | "buttonLabel"
    | "hear"
    | "hearLabel"
    | "tooltip";
  as: ReactElement<{ className?: unknown }>;
  /** The browser holds the room's sound back, and hearing is a part of the bar. */
  held?: boolean | undefined;
  children?: ReactNode;
}) {
  const s = call({ place: use(PlaceContext), held });
  return (
    <Box as={as} className={s[part]()}>
      {children}
    </Box>
  );
}

/**
 * The bar of the call the reader is in, kept while they move between screens: above the tab bar on a phone, ending the
 * navigation from `tablet`. The room's glyph, name and detail (who hosts it, how many listen) lead to its screen (in the
 * rail, its name in a tooltip); then, for a speaker, the microphone, a button named by what a press does now (Mute,
 * Unmute while muted, knocked out and crossed out then, as the room's own control is), and leaving last, each named,
 * and labelled from `desktop`; a listener's bar has no microphone. How many are in the room follows its name, never cut. While the call connects or connects again, a
 * spinner stands in the room's glyph and the detail says so; while the browser holds the room's sound back until a
 * press, `hear` is the bar's strongest part, knocked out, with its words: above the tab bar a line of its own over the
 * room, at the rail's end a tile before the microphone, and from `desktop` a line of its own over the two buttons.
 */
export function CallBar({
  href,
  room,
  count,
  detail,
  busy = false,
  hear,
  muted,
  micLabel,
  onMic,
  leaveLabel,
  onLeave,
}: {
  href: string;
  room: string;
  /** How many are in the room. */
  count?: Headcount | undefined;
  /** Who hosts it, or what the call is going through ("Reconnecting…", "The host muted you"). */
  detail: string;
  /** The call connects, or connects again. */
  busy?: boolean | undefined;
  /** The browser holds the room's sound back until the reader presses this ("Tap to hear"). */
  hear?: { label: string; onPress: () => void } | undefined;
  muted?: boolean | undefined;
  /** The microphone's name: what a press does now ("Mute", and "Unmute" while muted). */
  micLabel?: string | undefined;
  /**
   * The microphone was pressed: the owner turns it off, or on while muted. Without it the bar has no microphone: the
   * reader listens and does not speak.
   */
  onMic?: (() => void) | undefined;
  leaveLabel: string;
  onLeave: () => void;
}) {
  const place = use(PlaceContext);
  const look = callLook();
  const held = hear !== undefined;
  const hearing = hear !== undefined && (
    <CallPart part="hear" held as={<AriaButton className={look.hear()} onPress={hear.onPress} />}>
      <Icon name="hear" place="action" />
      <CallPart part="hearLabel" as={<span />}>
        {hear.label}
      </CallPart>
    </CallPart>
  );
  const link = (
    <CallPart part="room" as={<Link className={look.room()} href={href} />}>
      <CallPart part="glyph" as={<span />}>
        {busy ? <span className={look.spinner()} aria-hidden="true" /> : <Icon name="live" place="action" />}
      </CallPart>
      <CallPart part="lines" as={<span />}>
        <CallPart part="head" as={<span />}>
          <CallPart part="name" as={<span className={look.name()} dir="auto" />}>
            {room}
          </CallPart>
          {count !== undefined && " "}
          {count !== undefined && (
            <CallPart part="count" as={<span className={look.detail()} />}>
              <HeadcountMark count={count} />
            </CallPart>
          )}
        </CallPart>{" "}
        <CallPart part="detail" as={<span className={look.detail()} />}>
          {detail}
        </CallPart>
      </CallPart>
    </CallPart>
  );
  return (
    <CallPart
      part="root"
      held={held}
      as={
        <div
          data-frame-call={place === "dock" ? "" : undefined}
          role="group"
          aria-label={room}
          aria-busy={busy || undefined}
        />
      }
    >
      {place === "dock" && hearing}
      {place === "side" ? (
        <TooltipTrigger delay={300}>
          {link}
          <CallPart part="tooltip" as={<Tooltip className={look.tooltip()} placement="end" offset={8} />}>
            <Box as="span" display="block" className="font-label" dir="auto">
              {room}
            </Box>
            {count === undefined ? detail : `${count.label} · ${detail}`}
          </CallPart>
        </TooltipTrigger>
      ) : (
        link
      )}
      <CallPart part="actions" as={<span />}>
        {place !== "dock" && hearing}
        {onMic !== undefined && (
          <CallPart
            part="button"
            as={<AriaButton className={look.button()} onPress={onMic} data-on={muted || undefined} />}
          >
            <Icon name="microphone" place="action" on={muted === true} />
            <CallPart part="buttonLabel" as={<span />}>
              {micLabel}
            </CallPart>
          </CallPart>
        )}
        <CallPart part="button" as={<AriaButton className={look.button()} onPress={onLeave} />}>
          <Icon name="leave" place="action" />
          <CallPart part="buttonLabel" as={<span />}>
            {leaveLabel}
          </CallPart>
        </CallPart>
      </CallPart>
    </CallPart>
  );
}
