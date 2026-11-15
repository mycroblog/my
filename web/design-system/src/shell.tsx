// SPDX-License-Identifier: AGPL-3.0-only

// The app's frame (ADR-0025 §4, ADR-0110 §10, §11): the regions of a screen by the viewport's width, the one layout
// that reads the viewport (its breakpoints are tokens, `tablet:`, `desktop:` and `wide:` of theme.ts). The app chooses
// what goes in each region; the frame lays them out, with no visible page header, since the navigation shows where the
// reader is:
//
// - under `tablet` (a phone): the main column at the full width, and the dock fixed at the bottom: the tab bar, the
//   call's bar right above it while the reader is in a call, the primary action floating at the end above both, and
//   the toasts above all, at the dock's width;
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

import { createContext, use, useEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import {
  Button as AriaButton,
  Dialog,
  DialogTrigger,
  Heading,
  Link,
  Popover,
  ToggleButton,
  Tooltip,
  TooltipTrigger,
} from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { ButtonBase } from "./button";
import { IconButton } from "./icon-button";
import { Icon, type IconName } from "./icons";
import { UnreadMark } from "./list";
import { useReader } from "./locale";
import { ModalLayer, ModalPresence } from "./modal";

/**
 * Where a part of the frame sits: in the rail or sidebar, in the tab bar, floating over it, in the menu's sheet on a
 * phone or its popover from `tablet`, or in the dock above the tab bar (the call's bar).
 */
type Place = "side" | "tabs" | "float" | "sheet" | "popover" | "dock";

const PlaceContext = createContext<Place>("side");

/** What closes the menu a link sits in, which its press leaves for another screen; none outside a menu. */
const CloseContext = createContext<(() => void) | null>(null);

const frame = tv({
  slots: {
    // On a phone, `--shell-call` is the room the call's bar takes above the tab bar while there is one.
    root: [
      "box-border min-h-dvh bg-surface font-sans text-text [body:has(&)]:m-0 [html:has(&)]:bg-surface",
      "[--shell-call:0rem] has-[[data-frame-call]]:[--shell-call:var(--shell-call-bar)]",
      "tablet:flex tablet:justify-center tablet:gap-shell-gap tablet:px-shell-edge desktop:gap-shell-gap-wide",
    ],
    // The rail and the column beside the main one scroll with the page, with no scrollbar of their own (useAlong).
    side: [
      "hidden tablet:sticky tablet:box-border tablet:flex tablet:min-h-dvh tablet:w-shell-rail tablet:flex-none",
      "tablet:flex-col tablet:gap-shell-edge tablet:self-start tablet:pt-shell-inset tablet:pb-shell-edge",
      "desktop:w-shell-sidebar",
    ],
    // The main column is the screen a change of screen moves (motion.tsx); the fixed bars stay over it.
    main: [
      "box-border min-w-0 px-shell-edge pt-shell-gap pb-[calc(var(--shell-bar)+var(--shell-call)+var(--shell-end))]",
      "[view-transition-name:screen]",
      "tablet:shrink tablet:grow-0 tablet:basis-[var(--shell-column)] tablet:px-0 tablet:pb-shell-end",
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
      "tablet:items-center tablet:pb-shell-end",
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
      "data-focus-visible:shadow-focus desktop:justify-start desktop:px-shell-item-inline",
    ],
    markLight: "h-shell-mark [:root[data-theme=dark]_&]:hidden dark:[:root:not([data-theme=light])_&]:hidden",
    markDark: "hidden h-shell-mark [:root[data-theme=dark]_&]:block dark:[:root:not([data-theme=light])_&]:block",
    // An instance's own logo: square, at the mark's height, in the rail and the sidebar alike.
    logo: "size-shell-mark rounded-control object-contain",
  },
});

const nav = tv({
  slots: {
    list: "m-0 flex list-none p-0",
    li: "",
    item: [
      "relative flex items-center rounded-control text-muted-text no-underline outline-hidden",
      "data-focus-visible:shadow-focus-inset aria-[current=page]:font-strong aria-[current=page]:text-text",
    ],
    glyph: "relative inline-flex flex-none",
    label: "",
    // The badge on the glyph's corner, where the label is not shown beside the glyph, and at the line's end, where it
    // is; each place sets which shows.
    corner: "absolute -top-[0.375rem] start-[calc(100%-0.5rem)]",
    end: "ms-auto",
  },
  variants: {
    place: {
      side: {
        list: "flex-col gap-shell-item-gap",
        item: [
          // At the full width of the rail or the sidebar, so that the menu's popover opens beside the navigation.
          "box-border h-shell-item w-full flex-none justify-center gap-shell-edge data-hovered:bg-surface-hover",
          "desktop:justify-start desktop:px-shell-item-inline desktop:text-[length:var(--shell-label)]",
        ],
        // In the rail, the label names the place for assistive technology alone.
        label: "sr-only desktop:not-sr-only desktop:truncate",
        corner: "inline-flex desktop:hidden",
        end: "hidden desktop:inline-flex",
      },
      // Each item starts from its label's width and shares what is left; a label too long for 320 px wraps, inside a
      // word if it must, and is never shortened (ADR-0110 §11).
      tabs: {
        // The gap between items stays; a label wraps before it goes.
        list: "items-stretch gap-screen-tab-gap px-screen-tab-gap",
        li: "flex min-w-0 flex-[1_1_auto]",
        item: [
          "min-h-shell-bar w-full flex-col justify-center gap-[0.125rem] px-[0.125rem] py-shell-inset text-caption",
          "pointer-coarse:data-pressed:bg-surface-pressed",
        ],
        label: "text-center [overflow-wrap:anywhere]",
        corner: "inline-flex",
        end: "hidden",
      },
      float: { label: "hidden", corner: "hidden", end: "hidden" },
      // A row of the sheet, as a menu's: the glyph and the label at a form control's height.
      // Its glyphs line up with the sheet's title, the rows' surfaces reaching past them.
      sheet: {
        list: "-mx-control-timeline-inline flex-col",
        item: [
          "h-control-form gap-menu-icon-gap px-control-timeline-inline text-body text-text",
          "data-hovered:bg-surface-raised data-pressed:bg-surface-raised-hover",
        ],
        glyph: "text-muted-text",
        corner: "hidden",
        end: "inline-flex",
      },
      // A row of the menu's popover from `tablet`, as a menu's, inside its padding.
      popover: {
        list: "flex-col",
        item: [
          "h-control-form gap-menu-icon-gap px-control-timeline-inline text-body whitespace-nowrap text-text",
          "data-hovered:bg-surface-raised data-pressed:bg-surface-raised-hover",
        ],
        glyph: "text-muted-text",
        corner: "hidden",
        end: "inline-flex",
      },
      dock: { corner: "hidden", end: "hidden" },
    },
  },
});

const badge = tv({
  base: [
    "box-border h-shell-badge min-w-[var(--shell-badge)] items-center justify-center",
    "rounded-[calc(var(--shell-badge)/2)]",
    "bg-text px-shell-badge-inline text-[0.6875rem] leading-none font-label text-surface tabular-nums",
  ],
});

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
  return (
    <div className={s.dock()}>
      <div className={s.toasts()}>{toasts}</div>
      {action !== undefined && (
        <div className={s.float()}>
          <PlaceContext value="float">{action}</PlaceContext>
        </div>
      )}
      <div className={s.bars()}>
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
    root: "grid content-start gap-shell-inset [&_p]:m-0",
    // The heading names the section alone; the link to all of it ends the heading's line, outside the heading.
    head: "flex items-center justify-between gap-shell-inset text-caption",
    heading: "m-0 text-caption font-normal",
    link: [
      "inline-flex items-center gap-shell-inset rounded-control text-muted-text no-underline outline-hidden",
      "py-[calc((var(--target-minimum)-1lh)/2)] data-hovered:text-text data-hovered:underline",
      "data-hovered:underline-offset-[0.15em] data-focus-visible:shadow-focus",
    ],
    all: [
      "rounded-control py-[calc((var(--target-minimum)-1lh)/2)] text-accent-text no-underline outline-hidden",
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
    <section className={s.root()} aria-labelledby={id}>
      <div className={s.head()}>
        <h2 id={id} className={s.heading()}>
          <Link className={s.link()} href={href}>
            <Icon name={icon} place="caption" />
            {heading}
          </Link>
        </h2>
        {all !== undefined && (
          <Link className={s.all()} href={href} aria-label={all.name}>
            {all.text}
          </Link>
        )}
      </div>
      {children}
    </section>
  );
}

const fold = tv({
  slots: {
    // Compact and neutral, on the raised surface; its target reaches the minimum, and a finger's on a touch screen.
    root: [
      "mb-shell-gap box-border flex items-center gap-entry-caption rounded-overlay bg-surface-raised px-embed",
      "py-[calc((var(--target-minimum)-1lh)/2)] text-body text-text no-underline outline-hidden",
      "pointer-coarse:min-h-icon-button-touch data-hovered:bg-surface-raised-hover",
      "data-pressed:bg-surface-raised-hover data-focus-visible:shadow-focus wide:hidden",
    ],
    glyph: "inline-flex flex-none text-muted-text",
    text: "min-w-0 flex-1",
  },
});

/**
 * A section folded to one line above a feed under `wide`, where no column beside the main one holds it: its place's
 * glyph, what it holds in a few words ("2 unread announcements"), and the way onward; one link to its place. Hidden
 * from `wide`, where the column beside shows the section.
 */
export function FrameFold({ icon, href, children }: { icon: IconName; href: string; children: ReactNode }) {
  const s = fold();
  return (
    <Link className={s.root()} href={href}>
      <span className={s.glyph()}>
        <Icon name={icon} place="caption" />
      </span>
      <span className={s.text()}>{children}</span>
      <span className={s.glyph()}>
        <Icon name="onward" place="caption" />
      </span>
    </Link>
  );
}

const line = tv({
  slots: {
    list: "m-0 grid list-none gap-shell-inset p-0",
    item: "flex items-start gap-entry-caption",
    lines: "grid min-w-0 flex-1",
    title: [
      "line-clamp-2 rounded-control text-text no-underline outline-hidden data-hovered:underline",
      "data-hovered:underline-offset-[0.15em] data-focus-visible:shadow-focus",
    ],
    caption: "text-caption text-muted-text",
    mark: "inline-flex h-[1lh] flex-none items-center",
    glyph: "inline-flex h-[1lh] flex-none items-center text-muted-text",
    note: "m-0 flex flex-wrap items-baseline gap-x-shell-edge text-caption text-muted-text",
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
    <p className={line().note()}>
      <span>{children}</span>
      {action}
    </p>
  );
}

/** The lines of a section: what it lists, in order. */
export function SectionList({ children }: { children: ReactNode }) {
  return <ul className={line().list()}>{children}</ul>;
}

/**
 * A line of a section: its title, a link to it, on two lines at most, with a caption under it (a room's host and
 * listeners, an event's time), and the glyph of its kind before it when it has one (a notification's). `unread` marks
 * it with the unread mark at its end, in the text's color; `read` mutes it. `onPress` runs as the link is followed.
 */
export function SectionLine({
  href,
  title,
  caption,
  icon,
  unread,
  read = false,
  onPress,
}: {
  href: string;
  title: ReactNode;
  caption?: ReactNode;
  icon?: IconName | undefined;
  /** The unread mark's label while it is unread. */
  unread?: string | undefined;
  read?: boolean;
  onPress?: (() => void) | undefined;
}) {
  const s = line({ read });
  return (
    <li className={s.item()}>
      {icon !== undefined && (
        <span className={s.glyph()}>
          <Icon name={icon} place="caption" />
        </span>
      )}
      <span className={s.lines()}>
        <Link className={s.title()} href={href} dir="auto" {...(onPress === undefined ? {} : { onPress })}>
          {title}
        </Link>
        {caption !== undefined && <span className={s.caption()}>{caption}</span>}
      </span>
      {unread !== undefined && (
        <span className={s.mark()}>
          <UnreadMark label={unread} />
        </span>
      )}
    </li>
  );
}

const meter = tv({
  slots: {
    root: "grid gap-shell-inset",
    figure: "text-title tabular-nums",
    track: "h-shell-meter overflow-hidden rounded-[calc(var(--shell-meter)/2)] bg-surface-raised-hover",
    fill: "h-full rounded-[calc(var(--shell-meter)/2)] bg-text",
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
  const width = `${String(Math.round(Math.min(1, Math.max(0, value)) * 100))}%`;
  return (
    <div className={s.root()}>
      <span className={s.figure()} aria-hidden="true">
        {shown}
      </span>
      <div
        className={s.track()}
        role="meter"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(value * 100)}
        aria-valuetext={shown}
      >
        <div className={s.fill()} style={{ inlineSize: width }} />
      </div>
      <span className={s.caption()}>{caption}</span>
    </div>
  );
}

const footer = tv({
  slots: {
    root: "text-caption text-muted-text",
    // A link's target reaches the minimum around its text without moving the line (WCAG 2.5.8).
    list: "m-0 flex list-none flex-wrap gap-x-shell-edge p-0",
    link: [
      "inline-block rounded-control py-[calc((var(--target-minimum)-1lh)/2)] text-muted-text no-underline",
      "outline-hidden data-hovered:text-text data-hovered:underline data-focus-visible:shadow-focus",
    ],
  },
});

/** The server's pages (about, terms, privacy) at the end of the column beside the main one or of the menu's sheet. */
export function FrameFooter({ label, links }: { label: string; links: { href: string; label: string }[] }) {
  const s = footer();
  const close = use(CloseContext);
  return (
    <footer className={s.root()}>
      <nav aria-label={label}>
        <ul className={s.list()}>
          {links.map((l) => (
            <li key={l.href}>
              <Link className={s.link()} href={l.href} {...(close === null ? {} : { onPress: close })}>
                {l.label}
              </Link>
            </li>
          ))}
        </ul>
      </nav>
    </footer>
  );
}

const banner = tv({
  slots: {
    group: "mb-shell-gap grid gap-shell-inset",
    root: ["flex items-start gap-entry-avatar rounded-overlay px-embed py-shell-inset text-body [&_p]:m-0"],
    glyph: "inline-flex h-[1lh] flex-none items-center",
    body: "grid min-w-0 flex-1",
    links: "m-0 flex list-none flex-wrap gap-x-shell-edge p-0",
    link: [
      "inline-block rounded-control py-[calc((var(--target-minimum)-1lh)/2)] font-label underline",
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
    <div className={banner().group()} role="region" aria-label={label}>
      {children}
    </div>
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
    <div className={s.root()}>
      <span className={s.glyph()}>
        <Icon name={icon} place="action" />
      </span>
      <div className={s.body()}>
        <p dir="auto">{children}</p>
        {links.length > 0 && (
          <ul className={s.links()}>
            {links.map((l) => (
              <li key={l.href}>
                <Link
                  className={s.link()}
                  href={l.href}
                  {...(l.newTab === undefined ? {} : { target: "_blank", rel: "noopener noreferrer" })}
                >
                  {l.label}
                  {l.newTab !== undefined && <span className="sr-only"> {l.newTab}</span>}
                </Link>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

const toast = tv({
  slots: {
    root: [
      "pointer-events-auto box-border flex max-w-full items-center gap-entry-avatar rounded-overlay bg-text py-[0.25rem]",
      "ps-control-timeline-inline pe-[0.25rem] font-sans text-body text-surface shadow-overlay",
      "tablet:max-w-[var(--shell-column)]",
    ],
    text: "min-w-0 flex-1",
    action: [
      "h-control-timeline flex-none cursor-pointer rounded-control border-0 bg-transparent px-control-timeline-inline",
      "font-sans text-body font-label text-inherit outline-hidden hover:underline",
      "focus-visible:shadow-[inset_0_0_0_var(--focus-ring-width)_var(--color-surface)]",
    ],
    close: [
      "inline-flex size-control-timeline flex-none cursor-pointer items-center justify-center rounded-control border-0",
      "bg-transparent p-0 text-inherit outline-hidden hover:bg-[color-mix(in_srgb,var(--color-surface)_15%,transparent)]",
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
    <div className={s.root()} role="status">
      <span className={s.text()}>{children}</span>
      <button type="button" className={s.action()} onClick={onAction}>
        {action}
      </button>
      {onClose !== undefined && (
        <button type="button" className={s.close()} onClick={onClose} aria-label={closeLabel}>
          <Icon name="close" place="action" />
        </button>
      )}
    </div>
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
    <Link className={s.mark()} href={href} aria-label={label}>
      {logo !== undefined && logo !== "" ? (
        <img className={s.logo()} src={logo} alt="" />
      ) : (
        <>
          <img className={s.markLight()} src={light} alt="" />
          <img className={s.markDark()} src={dark} alt="" />
        </>
      )}
    </Link>
  );
}

/**
 * The screen's primary action in the frame (writing a post, or signing in): under the mark in the rail, with its label
 * in the sidebar, or floating over the tab bar: a glyph alone named by its label, or with its label when `labelled`
 * (signing in, which a glyph alone does not say).
 */
export function FrameAction({
  icon,
  label,
  onPress,
  labelled = false,
  "aria-keyshortcuts": keyshortcuts,
}: {
  icon: IconName;
  label: string;
  onPress: () => void;
  labelled?: boolean;
  /** The keys that run the action (what `useAction` returns), for assistive technology. */
  "aria-keyshortcuts"?: string | undefined;
}) {
  const place = use(PlaceContext);
  const s = nav({ place });
  const float = place === "float";
  return (
    <ButtonBase
      kind="primary"
      size={float ? (labelled ? "form" : "float") : "side"}
      aria-label={label}
      aria-keyshortcuts={keyshortcuts}
      onPress={onPress}
    >
      <span className="inline-flex items-center gap-shell-inset">
        <Icon name={icon} place="navigation" />
        <span className={float && labelled ? "" : s.label()} aria-hidden="true">
          {label}
        </span>
      </span>
    </ButtonBase>
  );
}

/** A list of places of the app, in the rail, the sidebar, the tab bar or the menu's sheet; two lists stand apart. */
export function NavList({ children }: { children: ReactNode }) {
  return <ul className={nav({ place: use(PlaceContext) }).list()}>{children}</ul>;
}

/** The glyph of a place with its count's badge on its corner, and the badge again at the line's end in the sidebar. */
function Counted({
  icon,
  on,
  count,
  label,
  countLabel,
}: {
  icon: IconName;
  on: boolean;
  count: number | undefined;
  label: string;
  countLabel: string | undefined;
}) {
  const place = use(PlaceContext);
  const s = nav({ place });
  const { lang } = useReader(undefined);
  const shown = count !== undefined && count > 0 ? countText(count, lang) : undefined;
  return (
    <>
      <span className={s.glyph()}>
        <Icon name={icon} place={place === "sheet" || place === "popover" ? "action" : "navigation"} on={on} />
        {shown !== undefined && (
          <span className={`${badge()} ${s.corner()}`} aria-hidden="true">
            {shown}
          </span>
        )}
      </span>
      <span className={s.label()}>{label}</span>
      {shown !== undefined && countLabel !== undefined && (
        <>
          {" "}
          <span className="sr-only">{countLabel}</span>
        </>
      )}
      {shown !== undefined && (
        <span className={`${badge()} ${s.end()}`} aria-hidden="true">
          {shown}
        </span>
      )}
    </>
  );
}

/**
 * A place: its glyph, on where the reader is, and its label, which the rail shows as its name alone and the tab bar
 * under the glyph. `count` puts a badge on it (unread notifications, reports to handle), which `countLabel` names.
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
  const s = nav({ place: use(PlaceContext) });
  const close = use(CloseContext);
  return (
    <li className={s.li()}>
      <Link
        className={s.item()}
        href={href}
        {...(current ? { "aria-current": "page" as const } : {})}
        {...(close === null ? {} : { onPress: close })}
      >
        <Counted icon={icon} on={current} count={count} label={label} countLabel={countLabel} />
      </Link>
    </li>
  );
}

const menu = tv({
  slots: {
    // The neutral mark on the menu's glyph while a place inside it has a count.
    dot: "absolute -top-[0.125rem] start-[calc(100%-0.25rem)] size-screen-unread rounded-[50%] bg-text",
    overlay: "fixed inset-0 z-50 box-border flex flex-col justify-end pt-shell-sheet-room",
    sheet: [
      "relative box-border grid max-h-full w-full content-start gap-dialog-gap overflow-y-auto rounded-t-overlay bg-surface",
      "px-dialog pt-shell-inset pb-[calc(var(--dialog-padding)+env(safe-area-inset-bottom))] font-sans text-body",
      "text-text shadow-overlay outline-hidden",
    ],
    dialog: "grid gap-dialog-gap outline-hidden",
    head: "flex items-center justify-between gap-dialog-gap",
    title: "m-0 text-title",
    groups: "grid gap-menu-group-gap",
    // From `tablet`: a menu's surface beside its entry, as wide as its rows, never wider than a menu may be.
    popover: [
      "box-border min-w-[var(--menu-min-width)] overflow-y-auto rounded-overlay",
      "bg-surface p-menu-padding font-sans text-body text-text shadow-overlay outline-hidden",
    ],
    popoverDialog: "grid gap-menu-group-gap outline-hidden",
    popoverFooter: "px-control-timeline-inline",
  },
});

/**
 * The menu: a place of the tab bar on a phone, and the last place of the rail and the sidebar, that opens what the
 * navigation does not hold (NavLists, apart, and the footer at the end). On a phone it opens a sheet from the bottom over
 * the dimmed page, titled, with its close button at the end of its head; from `tablet`, a popover beside its entry,
 * bounded as a menu is. Escape, a press outside and a place's press close either. `marked` puts a neutral mark on its
 * glyph while a place inside it has a count, which `markLabel` names for assistive technology.
 */
export function NavMenu({
  label,
  closeLabel,
  marked = false,
  markLabel,
  footer: end,
  children,
}: {
  label: string;
  closeLabel: string;
  marked?: boolean;
  markLabel?: string | undefined;
  footer?: ReactNode;
  children: ReactNode;
}) {
  const place = use(PlaceContext);
  const s = nav({ place });
  const m = menu();
  const { lang, direction } = useReader(undefined);
  const [open, setOpen] = useState(false);
  // A place's press (a link inside) closes the menu, as it leaves the screen.
  const close = () => {
    setOpen(false);
  };
  const entry = (
    <AriaButton
      className={`${s.item()} cursor-pointer border-0 bg-transparent font-sans`}
      {...(place === "tabs"
        ? {
            "aria-haspopup": "dialog" as const,
            "aria-expanded": open,
            onPress: () => {
              setOpen(true);
            },
          }
        : {})}
    >
      <span className={s.glyph()}>
        <Icon name="menu" place="navigation" />
        {marked && <span className={m.dot()} aria-hidden="true" />}
      </span>
      <span className={s.label()}>{label}</span>
      {marked && markLabel !== undefined && (
        <>
          {" "}
          <span className="sr-only">{markLabel}</span>
        </>
      )}
    </AriaButton>
  );
  if (place === "tabs")
    return (
      <li className={s.li()}>
        {entry}
        <ModalPresence>
          {open && (
            <ModalLayer key="menu" onDismiss={close} overlay={m.overlay()} panel={m.sheet()}>
              <Dialog className={m.dialog()}>
                <div className={m.dialog()} lang={lang} dir={direction}>
                  <div className={m.head()}>
                    <Heading slot="title" className={m.title()}>
                      {label}
                    </Heading>
                    <IconButton icon="close" label={closeLabel} slot="close" />
                  </div>
                  <CloseContext value={close}>
                    <div className={m.groups()}>
                      <PlaceContext value="sheet">{children}</PlaceContext>
                    </div>
                    {end}
                  </CloseContext>
                </div>
              </Dialog>
            </ModalLayer>
          )}
        </ModalPresence>
      </li>
    );
  return (
    <li className={s.li()}>
      <DialogTrigger isOpen={open} onOpenChange={setOpen}>
        {entry}
        {/* Toward the content, its end at the entry's, never flipped over the navigation; it mirrors from right to left. */}
        <Popover className={m.popover()} placement="end bottom" offset={8} shouldFlip={false}>
          <Dialog className={m.popoverDialog()} aria-label={label}>
            <CloseContext value={close}>
              <PlaceContext value="popover">{children}</PlaceContext>
              {end !== undefined && <div className={m.popoverFooter()}>{end}</div>}
            </CloseContext>
          </Dialog>
        </Popover>
      </DialogTrigger>
    </li>
  );
}

// Every control of the call's bar is a timeline control's size, and reaches the touch size on a touch screen.
const callTarget = "size-control-timeline pointer-coarse:size-icon-button-touch";

const call = tv({
  slots: {
    root: "flex items-center gap-entry-avatar",
    room: [
      "flex min-w-0 flex-1 items-center gap-entry-avatar rounded-control text-text no-underline outline-hidden",
      "data-focus-visible:shadow-focus",
    ],
    glyph: "inline-flex flex-none",
    lines: "grid min-w-0 flex-1 leading-entry",
    name: "truncate font-label",
    detail: "truncate text-caption text-muted-text",
    actions: "flex flex-none items-center gap-shell-inset",
    // The microphone muted is knocked out, as a toggle on is, with its crossed glyph: it reads at rest, without a hue.
    button: [
      "box-border inline-flex flex-none cursor-pointer items-center justify-center gap-icon-button-gap rounded-control",
      "border-0 p-0 font-sans text-caption font-label text-text outline-hidden data-focus-visible:shadow-focus",
      "data-selected:bg-text data-selected:text-surface data-selected:data-hovered:bg-text",
      "data-selected:data-pressed:bg-text",
      callTarget,
    ],
    buttonLabel: "sr-only",
    tooltip:
      "max-w-[var(--menu-min-width)] rounded-control bg-text px-shell-inset py-[0.25rem] text-caption text-surface",
  },
  variants: {
    place: {
      // Above the tab bar, at its full width, on the raised surface.
      dock: {
        root: "h-shell-call-bar bg-surface-raised ps-shell-edge pe-shell-inset",
        button: "bg-transparent data-hovered:bg-surface-raised-hover data-pressed:bg-surface-raised-pressed",
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
          "flex-none justify-center bg-surface data-hovered:bg-surface-hover",
          callTarget,
          "desktop:size-auto desktop:justify-start desktop:bg-transparent desktop:px-shell-inset desktop:pt-shell-inset",
          "desktop:data-hovered:bg-transparent desktop:data-hovered:[&_[dir]]:underline",
        ],
        // In the rail, the room's name and detail name its link for assistive technology, and its tooltip shows them.
        lines: "sr-only desktop:not-sr-only desktop:grid",
        actions: "flex-col desktop:flex-row",
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
      sheet: {},
      popover: {},
    },
  },
});

/**
 * The bar of the call the reader is in, kept while they move between screens: above the tab bar on a phone, ending the
 * navigation from `tablet`. The room's glyph, name and detail (who hosts it, how many listen) lead to its screen (in the
 * rail, its name in a tooltip); then the microphone, on while muted, and leaving last, each named, and labelled from
 * `desktop`.
 */
export function CallBar({
  href,
  room,
  detail,
  muted,
  muteLabel,
  onMute,
  leaveLabel,
  onLeave,
}: {
  href: string;
  room: string;
  detail: string;
  muted: boolean;
  /** The microphone's toggle, pressed while muted ("Mute"). */
  muteLabel: string;
  onMute: () => void;
  leaveLabel: string;
  onLeave: () => void;
}) {
  const place = use(PlaceContext);
  const s = call({ place });
  const link = (
    <Link className={s.room()} href={href}>
      <span className={s.glyph()}>
        <Icon name="live" place="action" />
      </span>
      <span className={s.lines()}>
        <span className={s.name()} dir="auto">
          {room}
        </span>{" "}
        <span className={s.detail()}>{detail}</span>
      </span>
    </Link>
  );
  return (
    <div className={s.root()} data-frame-call={place === "dock" ? "" : undefined} role="group" aria-label={room}>
      {place === "side" ? (
        <TooltipTrigger delay={300}>
          {link}
          <Tooltip className={s.tooltip()} placement="end" offset={8}>
            <span className="block font-label" dir="auto">
              {room}
            </span>
            {detail}
          </Tooltip>
        </TooltipTrigger>
      ) : (
        link
      )}
      <span className={s.actions()}>
        <ToggleButton className={s.button()} isSelected={muted} onChange={onMute}>
          <Icon name="microphone" place="action" on={muted} />
          <span className={s.buttonLabel()}>{muteLabel}</span>
        </ToggleButton>
        <AriaButton className={s.button()} onPress={onLeave}>
          <Icon name="leave" place="action" />
          <span className={s.buttonLabel()}>{leaveLabel}</span>
        </AriaButton>
      </span>
    </div>
  );
}
