// SPDX-License-Identifier: AGPL-3.0-only

// A feed (ADR-0110 §10, the WAI-ARIA feed pattern): entries (entry.tsx) one under another, a space apart, on the page's
// one surface. It carries role="feed" and aria-busy, and each entry its aria-posinset and aria-setsize (FeedItem).
// Page Down and Page Up move the focus to the next and the previous entry. Whenever the keyboard brings the focus to
// an entry (those keys, Tab, or a shortcut through `moveFeedFocus`), the page scrolls that entry to the top of the
// view: a click does not. The scroll keeps the entry's scroll margin (--feed-scroll-margin) and the scroller's
// scroll-padding-top, which the shell sets to the height of a sticky header; with reduced motion it jumps instead of
// gliding. Near its end it asks for more (onNearEnd).

import { createContext, use, useEffect, useEffectEvent, useRef, type KeyboardEvent, type ReactNode } from "react";
import { tv } from "tailwind-variants/lite";

/** Where an entry stands in its feed, for aria-posinset and aria-setsize; undefined outside a feed. */
export type FeedPosition = { posinset: number; setsize: number };

const FeedSize = createContext<number>(-1);
const FeedPositionContext = createContext<FeedPosition | undefined>(undefined);

/** The position of the entry inside the nearest FeedItem, which Entry gives its article. */
export const useFeedPosition = () => use(FeedPositionContext);

const entrySelector = "[data-entry]";
const fieldSelector = "input, textarea, select, [contenteditable]:not([contenteditable='false'])";
// How far before the end of a feed it asks for more: one more height of the view.
const nearEndMargin = "0px 0px 100% 0px";

/** The entries of `feed`, in order, leaving out those of a feed inside it. */
function entriesOf(feed: Element): HTMLElement[] {
  return Array.from(feed.querySelectorAll<HTMLElement>(entrySelector)).filter(
    (el) => el.parentElement?.closest('[role="feed"]') === feed,
  );
}

/**
 * Scrolls `entry` to the top of its scroller, below its scroll margin and the scroller's scroll padding (a sticky
 * header's), gliding unless the reader asked for reduced motion.
 */
function scrollEntryToTop(entry: Element): void {
  const reduced = entry.ownerDocument.defaultView?.matchMedia("(prefers-reduced-motion: reduce)").matches ?? true;
  entry.scrollIntoView({ block: "start", inline: "nearest", behavior: reduced ? "instant" : "smooth" });
}

/**
 * Moves the focus from `from` (an entry of a feed, or something inside one) to the entry `step` after it (1) or
 * before it (-1), and scrolls that entry to the top of the view. From inside the feed but outside any entry, it moves
 * to the first entry (1) or the last (-1). It returns whether it moved: not past either end, nor outside a feed.
 */
export function moveFeedFocus(from: Element | null, step: 1 | -1): boolean {
  const feed = from?.closest('[role="feed"]');
  if (!from || !feed) return false;
  const entries = entriesOf(feed);
  const current = from.closest<HTMLElement>(entrySelector);
  const i = current ? entries.indexOf(current) : -1;
  const to = i < 0 ? (step > 0 ? entries[0] : entries.at(-1)) : entries[i + step];
  if (!to) return false;
  to.focus({ preventScroll: true });
  scrollEntryToTop(to);
  return true;
}

const keySteps: Readonly<Record<string, 1 | -1>> = { PageDown: 1, PageUp: -1 };

/** The feed's keys: Page Down and Page Up move between its entries, outside a field and without a modifier. */
export function onFeedKeyDown(
  e: Pick<
    KeyboardEvent,
    "key" | "altKey" | "ctrlKey" | "metaKey" | "shiftKey" | "target" | "preventDefault" | "defaultPrevented"
  >,
): void {
  const step = keySteps[e.key];
  const target = e.target as Element;
  if (step === undefined || e.defaultPrevented || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
  if (target.closest(fieldSelector)) return;
  if (moveFeedFocus(target, step)) e.preventDefault();
}

/** Scrolls an entry the keyboard focused (Tab, a shortcut) to the top; an entry focused by a click stays put. */
export function onFeedFocus(e: { target: Element; currentTarget: Element }): void {
  const { target, currentTarget } = e;
  if (!target.matches(entrySelector) || target.parentElement?.closest('[role="feed"]') !== currentTarget) return;
  if (target.matches(":focus-visible")) scrollEntryToTop(target);
}

const feed = tv({
  slots: {
    root: "grid grid-cols-[minmax(0,1fr)] gap-feed",
    list: "grid grid-cols-[minmax(0,1fr)] gap-feed",
  },
});

export type FeedProps = {
  /** The feed's name, for assistive technology (the timeline's). */
  label: string;
  /** How many entries it holds now. */
  count: number;
  /** Whether more entries may follow those it holds: its size is then unknown. */
  more: boolean;
  /** Whether entries are being added or taken away (aria-busy). */
  busy: boolean;
  /** Asks for more entries when its end comes within a view's height; left out while none should be asked for. */
  onNearEnd?: (() => void) | undefined;
  /** The entries, each in a FeedItem. */
  children: ReactNode;
  /** What follows the entries, outside the feed: a FeedNotice of the loading, a failure, or no entries. */
  end?: ReactNode;
};

/** A feed of entries, one under another, a space apart. */
export function Feed({ label, count, more, busy, onNearEnd, children, end }: FeedProps) {
  const s = feed();
  const sentinel = useRef<HTMLDivElement>(null);
  const asks = onNearEnd !== undefined;
  const nearEnd = useEffectEvent(() => {
    onNearEnd?.();
  });
  useEffect(() => {
    const el = sentinel.current;
    if (!el || !asks || busy || typeof IntersectionObserver === "undefined") return;
    // Observed anew whenever the entries change, so that an end still in view after a page asks again.
    const observer = new IntersectionObserver(
      (records) => {
        if (records.some((r) => r.isIntersecting)) nearEnd();
      },
      { rootMargin: nearEndMargin },
    );
    observer.observe(el);
    return () => {
      observer.disconnect();
    };
  }, [asks, busy, count]);
  return (
    <div className={s.root()}>
      <FeedSize value={more ? -1 : count}>
        <div
          role="feed"
          aria-label={label}
          aria-busy={busy}
          className={s.list()}
          onKeyDown={onFeedKeyDown}
          onFocus={(e) => {
            onFeedFocus({ target: e.target, currentTarget: e.currentTarget });
          }}
        >
          {children}
        </div>
      </FeedSize>
      {end}
      {asks && <div ref={sentinel} aria-hidden="true" />}
    </div>
  );
}

/** One entry of a feed at `position` (from 1), which its Entry states with the feed's size. */
export function FeedItem({ position, children }: { position: number; children: ReactNode }) {
  const setsize = use(FeedSize);
  return <FeedPositionContext value={{ posinset: position, setsize }}>{children}</FeedPositionContext>;
}

const notice = tv({
  slots: {
    root: "m-0 text-center text-caption text-muted-text",
    action: "whitespace-nowrap",
  },
});

/**
 * A line after a feed's entries, centered and muted: that it loads, that it failed (`alert`), or that it has none,
 * with an action after it (a `TextButton`, such as trying again), apart from it by a middle dot.
 */
export function FeedNotice({
  alert = false,
  action,
  children,
}: {
  alert?: boolean | undefined;
  action?: ReactNode;
  children: ReactNode;
}) {
  const s = notice();
  return (
    <p role={alert ? "alert" : "status"} className={s.root()}>
      {children}
      {action !== undefined && (
        <>
          {" "}
          <span className={s.action()}>· {action}</span>
        </>
      )}
    </p>
  );
}
