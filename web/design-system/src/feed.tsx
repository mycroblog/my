// SPDX-License-Identifier: AGPL-3.0-only

// A feed (ADR-0110 §10, the WAI-ARIA feed pattern): entries (entry.tsx) one under another, a space apart, on the page's
// one surface. It carries role="feed" and aria-busy, and each entry its aria-posinset and aria-setsize (FeedItem).
// Page Down and Page Up move the focus to the next and the previous entry. Whenever the keyboard brings the focus to
// an entry (those keys, Tab, or a shortcut through `moveFeedFocus`), the page scrolls that entry to the top of the
// view: a click does not. The scroll keeps the entry's scroll margin (--feed-scroll-margin) and the scroller's
// scroll-padding-top, which the shell sets to the height of a sticky header; with reduced motion it jumps instead of
// gliding. Near its end it asks for more (onNearEnd). It is the `ActionScope` of the focus actions declared inside
// it (the next and the previous post), which act while focus is on an entry or inside one.
//
// Its scroll (scroll.ts, ADR-0013 §4–§7): away from the top, entries inserted above leave what the reader sees where
// it was; at the top they show. It says whether its top is in view (a mark at its top, seen by an
// IntersectionObserver) and whether the reader is in it (focus or a selection inside), and floats the new entries'
// button (NewItems) over its top, which brings the reader to the top and the focus to the screen's heading. When to
// insert is its owner's to decide. Each entry carries its key as `data-scroll-key`, by which a screen restores its
// position, and by which the feed tells which entry went out above the view (onPassed) or was clicked (onEntryClick).

import {
  createContext,
  use,
  useEffect,
  useEffectEvent,
  useLayoutEffect,
  useRef,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import { tv } from "tailwind-variants/lite";
import { ButtonBase } from "./button";
import { anchoring, scrollerOf, scrollToTop, viewTopOf, type Anchoring } from "./scroll";
import { focusFrameHeading } from "./shell";
import { ActionScope } from "./shortcuts";

/** Where an entry stands in its feed, for aria-posinset and aria-setsize, and its key; undefined outside a feed. */
export type FeedPosition = { posinset: number; setsize: number; key: string };

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
function scrollEntryToTop(entry: Element, jump = false): void {
  const reduced = entry.ownerDocument.defaultView?.matchMedia("(prefers-reduced-motion: reduce)").matches ?? true;
  entry.scrollIntoView({ block: "start", inline: "nearest", behavior: reduced || jump ? "instant" : "smooth" });
}

// The feeds that moved in the frame being drawn: a feed moves once a frame, so that a held key advances steadily
// instead of piling up moves the screen never shows.
const movedThisFrame = new WeakSet<Element>();

/**
 * Moves the focus from `from` (an entry of a feed, or something inside one) to the entry `step` after it (1) or
 * before it (-1), and scrolls that entry to the top of the view: gliding for a single press, jumping while the key is
 * held (`repeat`), since each glide would cancel the one before and the screen would stand still. From inside the feed
 * but outside any entry, it moves to the first entry (1) or the last (-1). It moves at most once a frame; a press
 * within the same frame is taken without a move. It returns whether it took the press: not past either end, nor
 * outside a feed.
 */
export function moveFeedFocus(from: Element | null, step: 1 | -1, { repeat = false } = {}): boolean {
  const feed = from?.closest('[role="feed"]');
  if (!from || !feed) return false;
  const entries = entriesOf(feed);
  const current = from.closest<HTMLElement>(entrySelector);
  const i = current ? entries.indexOf(current) : -1;
  const to = i < 0 ? (step > 0 ? entries[0] : entries.at(-1)) : entries[i + step];
  if (!to) return false;
  if (movedThisFrame.has(feed)) return true;
  const view = feed.ownerDocument.defaultView;
  if (view?.requestAnimationFrame) {
    movedThisFrame.add(feed);
    view.requestAnimationFrame(() => {
      movedThisFrame.delete(feed);
    });
  }
  to.focus({ preventScroll: true });
  scrollEntryToTop(to, repeat);
  return true;
}

const keySteps: Readonly<Record<string, 1 | -1>> = { PageDown: 1, PageUp: -1 };

/** The feed's keys: Page Down and Page Up move between its entries, outside a field and without a modifier. */
export function onFeedKeyDown(
  e: Pick<
    KeyboardEvent,
    "key" | "altKey" | "ctrlKey" | "metaKey" | "shiftKey" | "target" | "preventDefault" | "defaultPrevented"
  > & { repeat?: boolean },
): void {
  const step = keySteps[e.key];
  const target = e.target as Element;
  if (step === undefined || e.defaultPrevented || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
  if (target.closest(fieldSelector)) return;
  if (moveFeedFocus(target, step, { repeat: e.repeat === true })) e.preventDefault();
}

/** Scrolls an entry the keyboard focused (Tab, a shortcut) to the top; an entry focused by a click stays put. */
export function onFeedFocus(e: { target: Element; currentTarget: Element }): void {
  const { target, currentTarget } = e;
  if (!target.matches(entrySelector) || target.parentElement?.closest('[role="feed"]') !== currentTarget) return;
  if (target.matches(":focus-visible")) scrollEntryToTop(target);
}

// The list, the mark at its top and the new entries' button share the first row of the grid, so that neither the
// mark nor the button takes room or a gap. The button keeps the overlay's shadow, so that it stands off whatever
// scrolls under it. The feed turns the browser's anchoring off, since it anchors itself.
const feed = tv({
  slots: {
    root: "grid grid-cols-[minmax(0,1fr)] gap-feed [overflow-anchor:none]",
    list: "col-start-1 row-start-1 grid grid-cols-[minmax(0,1fr)] gap-feed",
    top: "pointer-events-none col-start-1 row-start-1 h-px self-start",
    newItems: "sticky top-shell-gap z-2 col-start-1 row-start-1 flex h-0 justify-center self-start",
    newItemsButton: "rounded-control shadow-overlay",
  },
});

/** The new entries' button: what it says (how many), and what a press does before the feed goes to the top. */
export type NewItems = { label: string; onPress: () => void };

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
  /** Hears whether the feed's top is in view, when that changes. */
  onTopChange?: ((atTop: boolean) => void) | undefined;
  /** Hears whether the reader is in the feed (focus or a selection inside it), when that changes. */
  onReaderChange?: ((inside: boolean) => void) | undefined;
  /** The new entries' button, floating over the feed's top; left out while there are none to tell of. */
  newItems?: NewItems | undefined;
  /**
   * Hears the key of an entry once it went out above the view (its end above the view's top, below the scroller's
   * padding), and of each entry above the view when the feed shows it.
   */
  onPassed?: ((key: string) => void) | undefined;
  /** Hears the key of an entry the reader clicked or tapped, anywhere inside it. */
  onEntryClick?: ((key: string) => void) | undefined;
};

// What says that the reader scrolls by hand.
const takeOverEvents = ["wheel", "touchstart", "keydown"] as const;

/** Whether the document's selection, when there is one, starts or ends inside `el`. */
function selectsIn(el: Element): boolean {
  const selection = el.ownerDocument.getSelection();
  if (!selection || selection.isCollapsed) return false;
  return [selection.anchorNode, selection.focusNode].some((n) => n !== null && el.contains(n));
}

/**
 * Keeps the place of what the reader sees in `list` as entries go in above it (scroll.ts), and tells `onTopChange`
 * whether its top is in view, by the mark `top`. Returns its anchoring, once the list is there.
 */
function useAnchoring(
  list: { readonly current: HTMLElement | null },
  top: { readonly current: HTMLElement | null },
  onTopChange: ((atTop: boolean) => void) | undefined,
) {
  const anchor = useRef<{ scroller: Element; anchoring: Anchoring } | null>(null);
  const topChange = useEffectEvent((atTop: boolean) => {
    onTopChange?.(atTop);
  });
  useLayoutEffect(() => {
    const el = list.current;
    const mark = top.current;
    if (!el || !mark) return;
    const scroller = scrollerOf(el);
    const page = scroller === el.ownerDocument.scrollingElement;
    const a = anchoring({
      list: el,
      items: () => entriesOf(el),
      scroller,
      viewTop: () => viewTopOf(scroller),
    });
    anchor.current = { scroller, anchoring: a };
    a.measure();
    const target: EventTarget = page ? (el.ownerDocument.defaultView ?? scroller) : scroller;
    const onScroll = () => {
      a.measure();
    };
    target.addEventListener("scroll", onScroll, { passive: true });
    // A change of the entries' heights (an image that loads, a post that opens) keeps the place too.
    const resized =
      typeof ResizeObserver === "undefined"
        ? undefined
        : new ResizeObserver(() => {
            a.keep();
          });
    resized?.observe(el);
    // The top is in view when the mark is, below the scroller's padding (a sticky header).
    const seen =
      typeof IntersectionObserver === "undefined"
        ? undefined
        : new IntersectionObserver(
            (records) => {
              const last = records.at(-1);
              if (last) topChange(last.isIntersecting);
            },
            {
              root: page ? null : scroller,
              rootMargin: `${-(viewTopOf(scroller) - (page ? 0 : scroller.getBoundingClientRect().top))}px 0px 0px 0px`,
            },
          );
    seen?.observe(mark);
    return () => {
      target.removeEventListener("scroll", onScroll);
      resized?.disconnect();
      seen?.disconnect();
      anchor.current = null;
    };
  }, [list, top]);
  // Before the browser paints a change of the entries, the place it moved from is taken back.
  useLayoutEffect(() => {
    anchor.current?.anchoring.keep();
  });
  return anchor;
}

/** A feed of entries, one under another, a space apart. */
export function Feed({
  label,
  count,
  more,
  busy,
  onNearEnd,
  children,
  end,
  onTopChange,
  onReaderChange,
  newItems,
  onPassed,
  onEntryClick,
}: FeedProps) {
  const s = feed();
  const sentinel = useRef<HTMLDivElement>(null);
  const list = useRef<HTMLDivElement>(null);
  const top = useRef<HTMLDivElement>(null);
  const anchor = useAnchoring(list, top, onTopChange);
  // Whether focus is in the feed and whether a selection is, which onReaderChange hears together.
  const reader = useRef({ focus: false, selection: false, inside: false });
  const readerChange = useEffectEvent((part: "focus" | "selection", on: boolean) => {
    const r = reader.current;
    r[part] = on;
    const inside = r.focus || r.selection;
    if (inside === r.inside) return;
    r.inside = inside;
    onReaderChange?.(inside);
  });
  useEffect(() => {
    const el = list.current;
    if (!el) return;
    const doc = el.ownerDocument;
    const onSelection = () => {
      readerChange("selection", selectsIn(el));
    };
    doc.addEventListener("selectionchange", onSelection);
    return () => {
      doc.removeEventListener("selectionchange", onSelection);
    };
  }, []);
  const goToTop = () => {
    const el = list.current;
    newItems?.onPress();
    if (!el) return;
    const a = anchor.current;
    // What the press inserts goes in above without keeping the place: the view is on its way to the top.
    if (a) {
      a.anchoring.holdUntilTop();
      // The reader who takes the scroll over before the top is reached gets the anchoring back.
      const view = el.ownerDocument.defaultView;
      const takeOver = () => {
        a.anchoring.release();
        for (const type of takeOverEvents) view?.removeEventListener(type, takeOver);
      };
      for (const type of takeOverEvents) view?.addEventListener(type, takeOver, { passive: true });
    }
    scrollToTop(a?.scroller ?? scrollerOf(el));
    focusFrameHeading(el.ownerDocument);
  };
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
  const hears = onPassed !== undefined;
  const passed = useEffectEvent((key: string) => {
    onPassed?.(key);
  });
  useEffect(() => {
    const el = list.current;
    if (!el || !hears || typeof IntersectionObserver === "undefined") return;
    const scroller = scrollerOf(el);
    const page = scroller === el.ownerDocument.scrollingElement;
    // Observed anew whenever the entries change, so that a new entry is heard too.
    const observer = new IntersectionObserver(
      (records) => {
        for (const r of records) {
          const key = r.target.getAttribute("data-scroll-key");
          const top = r.rootBounds?.top ?? 0;
          if (key !== null && !r.isIntersecting && r.boundingClientRect.bottom <= top) passed(key);
        }
      },
      {
        root: page ? null : scroller,
        rootMargin: `${-(viewTopOf(scroller) - (page ? 0 : scroller.getBoundingClientRect().top))}px 0px 0px 0px`,
      },
    );
    for (const entry of entriesOf(el)) observer.observe(entry);
    return () => {
      observer.disconnect();
    };
  }, [hears, count]);
  return (
    <div className={s.root()}>
      {/* Always there, so that a change of the count is read out (ADR-0013 §6); the button never takes the focus. */}
      <div role="status" className={s.newItems()}>
        {newItems && (
          <span className={s.newItemsButton()}>
            <ButtonBase kind="primary" size="notice" onPress={goToTop}>
              {newItems.label}
            </ButtonBase>
          </span>
        )}
      </div>
      <div ref={top} aria-hidden="true" className={s.top()} />
      <FeedSize value={more ? -1 : count}>
        {/* A feed owns its articles: with none yet (loading, failed, empty), the list is no feed, and the notice of
            `end` tells what it holds. */}
        <div
          ref={list}
          role={count > 0 ? "feed" : undefined}
          aria-label={count > 0 ? label : undefined}
          aria-busy={busy}
          className={s.list()}
          onKeyDown={onFeedKeyDown}
          onClick={(e) => {
            const entry = (e.target as Element).closest(entrySelector);
            const key = entry?.getAttribute("data-scroll-key");
            if (entry?.parentElement?.closest('[role="feed"]') === e.currentTarget && key) onEntryClick?.(key);
          }}
          onFocus={(e) => {
            readerChange("focus", true);
            onFeedFocus({ target: e.target, currentTarget: e.currentTarget });
          }}
          onBlur={(e) => {
            if (!(e.relatedTarget instanceof Node && e.currentTarget.contains(e.relatedTarget))) {
              readerChange("focus", false);
            }
          }}
        >
          <ActionScope target={list}>{children}</ActionScope>
        </div>
      </FeedSize>
      {end}
      {asks && <div ref={sentinel} aria-hidden="true" />}
    </div>
  );
}

/**
 * One entry of a feed at `position` (from 1), which its Entry states with the feed's size, and its key: what it shows
 * (a post's id), by which a screen finds it again to restore its position.
 */
export function FeedItem({ id, position, children }: { id: string; position: number; children: ReactNode }) {
  const setsize = use(FeedSize);
  return <FeedPositionContext value={{ posinset: position, setsize, key: id }}>{children}</FeedPositionContext>;
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
