// SPDX-License-Identifier: AGPL-3.0-only

// The app's frame (ADR-0025 §4, ADR-0110 §10, §11): the regions of a screen by the viewport's width, the one layout
// that reads the viewport (its breakpoints are tokens, `tablet:`, `desktop:` and `wide:` of theme.ts). The app chooses
// what goes in each region; the frame lays them out, with no visible page header, since the navigation shows where the
// reader is:
//
// - under `tablet` (a phone): the main column at the full width, the navigation as a tab bar fixed at the bottom, and
//   the primary action floating over the bar's end;
// - from `tablet`: a rail of glyphs at the start, with the primary action under the mark, beside the main column;
// - from `desktop`: the rail grows into a sidebar with the labels;
// - from `wide`: a column beside the main one, for what is around the reader.
//
// Every surface is the page's; the tab bar stands apart from what scrolls under it by a shadow, not a line. A link of
// the frame navigates through the app's router when the app places NavigationProvider.

import { createContext, use, type ReactNode } from "react";
import { Link } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { ButtonBase } from "./button";
import { Icon, type IconName } from "./icons";

/** Where a part of the frame sits: in the rail or sidebar, in the tab bar, or floating over it. */
type Place = "side" | "tabs" | "float";

const PlaceContext = createContext<Place>("side");

const frame = tv({
  slots: {
    root: [
      "box-border min-h-dvh bg-surface font-sans text-text [body:has(&)]:m-0 [html:has(&)]:bg-surface",
      "tablet:flex tablet:justify-center tablet:gap-shell-gap tablet:px-shell-edge desktop:gap-shell-gap-wide",
    ],
    side: [
      "hidden tablet:sticky tablet:top-0 tablet:box-border tablet:flex tablet:h-dvh tablet:w-shell-rail tablet:flex-none",
      "tablet:flex-col tablet:gap-shell-edge tablet:self-start tablet:pt-shell-inset tablet:pb-shell-edge",
      "desktop:w-shell-sidebar",
    ],
    // The main column is the screen a change of screen moves (motion.tsx); the fixed bars stay over it.
    main: [
      "box-border min-w-0 px-shell-edge pt-shell-gap pb-[calc(var(--shell-bar)+var(--shell-end))]",
      "[view-transition-name:screen]",
      "tablet:shrink tablet:grow-0 tablet:basis-[var(--shell-column)] tablet:px-0 tablet:pb-shell-end",
    ],
    heading: "sr-only",
    aside: [
      "hidden text-body",
      "wide:box-border wide:grid wide:w-shell-aside wide:flex-none wide:content-start wide:gap-shell-section",
      "wide:self-start wide:py-shell-gap",
    ],
    section: "grid gap-shell-inset [&_p]:m-0",
    sectionHeading: "m-0 text-caption font-normal",
    sectionLink: [
      "inline-flex items-center gap-shell-inset rounded-control text-muted-text no-underline outline-hidden",
      "data-hovered:text-text data-hovered:underline data-hovered:underline-offset-[0.15em]",
      "data-focus-visible:shadow-focus",
    ],
    tabs: [
      "fixed inset-x-0 bottom-0 z-3 bg-surface pb-[env(safe-area-inset-bottom)] shadow-bar",
      "[view-transition-name:tab-bar] tablet:hidden",
    ],
    float: [
      "fixed end-shell-edge bottom-[calc(var(--shell-bar)+var(--shell-edge)+env(safe-area-inset-bottom))] z-3",
      "[view-transition-name:float]",
      "rounded-control shadow-overlay tablet:hidden",
    ],
    mark: [
      "flex h-shell-item items-center justify-center rounded-control outline-hidden data-focus-visible:shadow-focus",
      "desktop:justify-start desktop:px-shell-item-inline",
    ],
    markLight: "h-shell-mark [:root[data-theme=dark]_&]:hidden dark:[:root:not([data-theme=light])_&]:hidden",
    markDark: "hidden h-shell-mark [:root[data-theme=dark]_&]:block dark:[:root:not([data-theme=light])_&]:block",
  },
});

const nav = tv({
  slots: {
    list: "m-0 flex list-none p-0",
    item: [
      "flex items-center gap-shell-edge rounded-control text-muted-text no-underline outline-hidden",
      "data-focus-visible:shadow-focus-inset aria-[current=page]:font-strong aria-[current=page]:text-text",
    ],
    label: "truncate",
  },
  variants: {
    place: {
      side: {
        list: "flex-col gap-shell-item-gap",
        item: [
          "h-shell-item justify-center data-hovered:bg-surface-hover",
          "desktop:justify-start desktop:px-shell-item-inline desktop:text-[length:var(--shell-label)]",
        ],
        label: "hidden desktop:block",
      },
      tabs: { list: "justify-around", item: "size-shell-bar justify-center", label: "hidden" },
      float: { list: "", item: "", label: "hidden" },
    },
  },
});

/** The frame of every screen of the app, which holds its regions. */
export function Frame({ children }: { children: ReactNode }) {
  return <div className={frame().root()}>{children}</div>;
}

/** The rail, from `tablet`, which grows into the sidebar from `desktop`: the mark, the primary action and the places. */
export function FrameSide({ label, children }: { label: string; children: ReactNode }) {
  return (
    <nav className={frame().side()} aria-label={label}>
      <PlaceContext value="side">{children}</PlaceContext>
    </nav>
  );
}

/** The tab bar, under `tablet`: the places, fixed at the bottom. */
export function FrameTabBar({ label, children }: { label: string; children: ReactNode }) {
  return (
    <nav className={frame().tabs()} aria-label={label}>
      <PlaceContext value="tabs">{children}</PlaceContext>
    </nav>
  );
}

/** What floats over the end of the tab bar, under `tablet`: the primary action. */
export function FrameFloat({ children }: { children: ReactNode }) {
  return (
    <div className={frame().float()}>
      <PlaceContext value="float">{children}</PlaceContext>
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

/** The column beside the main one, from `wide`: what is around the reader, in sections. */
export function FrameAside({ label, children }: { label: string; children: ReactNode }) {
  return (
    <aside className={frame().aside()} aria-label={label}>
      {children}
    </aside>
  );
}

/** A section of the column beside the main one, under a heading that links to its place, with that place's glyph. */
export function FrameSection({
  id,
  icon,
  href,
  heading,
  children,
}: {
  /** The heading's id, which names the section. */
  id: string;
  icon: IconName;
  href: string;
  heading: string;
  children: ReactNode;
}) {
  const s = frame();
  return (
    <section className={s.section()} aria-labelledby={id}>
      <h2 id={id} className={s.sectionHeading()}>
        <Link className={s.sectionLink()} href={href}>
          <Icon name={icon} place="caption" />
          {heading}
        </Link>
      </h2>
      {children}
    </section>
  );
}

/** The mark at the top of the rail, a link to the app's start; its image in the version for light and for dark. */
export function FrameMark({ href, label, light, dark }: { href: string; label: string; light: string; dark: string }) {
  const s = frame();
  return (
    <Link className={s.mark()} href={href} aria-label={label}>
      <img className={s.markLight()} src={light} alt="" />
      <img className={s.markDark()} src={dark} alt="" />
    </Link>
  );
}

/**
 * The screen's primary action in the frame (writing a post): under the mark in the rail, with its label in the
 * sidebar, or a glyph alone floating over the tab bar, named by its label.
 */
export function FrameAction({
  icon,
  label,
  onPress,
  "aria-keyshortcuts": keyshortcuts,
}: {
  icon: IconName;
  label: string;
  onPress: () => void;
  /** The keys that run the action (what `useAction` returns), for assistive technology. */
  "aria-keyshortcuts"?: string | undefined;
}) {
  const place = use(PlaceContext);
  const s = nav({ place });
  return (
    <ButtonBase
      kind="primary"
      size={place === "float" ? "float" : "side"}
      aria-label={label}
      aria-keyshortcuts={keyshortcuts}
      onPress={onPress}
    >
      <Icon name={icon} place="navigation" />
      <span className={s.label()} aria-hidden="true">
        {label}
      </span>
    </ButtonBase>
  );
}

/** The places of the app, in the rail, the sidebar or the tab bar. */
export function NavList({ children }: { children: ReactNode }) {
  return <ul className={nav({ place: use(PlaceContext) }).list()}>{children}</ul>;
}

/** A place: its glyph, on where the reader is, and its label, which the rail and the tab bar show as its name alone. */
export function NavItem({
  href,
  icon,
  label,
  current = false,
}: {
  href: string;
  icon: IconName;
  label: string;
  /** Whether the reader is at this place. */
  current?: boolean;
}) {
  const s = nav({ place: use(PlaceContext) });
  return (
    <li>
      <Link
        className={s.item()}
        href={href}
        {...(current ? { "aria-current": "page" as const } : {})}
        aria-label={label}
      >
        <Icon name={icon} place="navigation" on={current} />
        <span className={s.label()} aria-hidden="true">
          {label}
        </span>
      </Link>
    </li>
  );
}
