// SPDX-License-Identifier: AGPL-3.0-only

// Scroll (ADR-0025 §4, ADR-0013 §4): the mechanism under a feed and a screen, with no policy of its own.
//
// A list away from the top of its view keeps what the reader sees in place when items are inserted above (or grow or
// shrink there) by an anchor: the first item whose bottom is below the top of the view, and where it stands in the
// scroller's content. After a change, the scroller moves by as much as the anchor moved, before the change is painted.
// At the top of the view nothing is kept, so that what is inserted shows. The browser's own anchoring
// (`overflow-anchor`) is not relied on, since Safari has none; a list that anchors itself turns it off, so that the two
// never correct the same change twice.
//
// A screen saves the page's position by a key as the reader scrolls, and restores it when it shows again: the element
// with a `data-scroll-key` at the top of the view and its offset, so that what was inserted above while the screen was
// away does not move it, else the plain offset. The app decides which screens restore and which open at the top.

import { useLayoutEffect } from "react";

/** The part of an element the mechanism reads: a fake stands in for it in a test. */
export type Box = { getBoundingClientRect(): { top: number; bottom: number }; readonly isConnected?: boolean };

/** The scroller of a view: the page's scrolling element, or an element with its own scrollbar. */
export type Scroller = Box & { scrollTop: number };

/** The scroller of `el`: its nearest ancestor that scrolls its overflow, else the page's scrolling element. */
export function scrollerOf(el: Element): Element {
  const doc = el.ownerDocument;
  const view = doc.defaultView;
  for (let p = el.parentElement; p && view; p = p.parentElement) {
    if (p === doc.body || p === doc.documentElement) break;
    const { overflowY } = view.getComputedStyle(p);
    if (overflowY === "auto" || overflowY === "scroll") return p;
  }
  return doc.scrollingElement ?? doc.documentElement;
}

/**
 * The top of the view of `scroller` in the viewport's coordinates, below its scroll padding (a sticky header's); the
 * page's scrolling element views from the top of the viewport. It does not move as the scroller scrolls.
 */
export function viewTopOf(scroller: Element): number {
  const doc = scroller.ownerDocument;
  const padding = Number.parseFloat(doc.defaultView?.getComputedStyle(scroller).scrollPaddingTop ?? "") || 0;
  const top = scroller === doc.scrollingElement ? 0 : scroller.getBoundingClientRect().top;
  return top + padding;
}

/**
 * The first of `items` (in their order down the view) whose bottom is below `viewTop`, and its top's distance below
 * `viewTop`; none when every item is above the view.
 */
export function firstInView<T extends Box>(
  items: Iterable<T>,
  viewTop: number,
): { element: T; offset: number } | undefined {
  for (const element of items) {
    const { top, bottom } = element.getBoundingClientRect();
    if (bottom > viewTop) return { element, offset: top - viewTop };
  }
  return undefined;
}

/** How a list keeps its place in its view. */
export type Anchoring = {
  /** Takes the anchor anew, and whether the list's top is in view: after the reader scrolls, and after a change. */
  measure(): void;
  /** Scrolls by as much as the anchor moved since it was measured, unless the list's top was in view; then measures. */
  keep(): void;
  /** Whether the list's top was in view when last measured. */
  atTop(): boolean;
  /** Keeps nothing until the view reaches the list's top, or until `release`: while it scrolls there. */
  holdUntilTop(): void;
  /** Keeps the place again after `holdUntilTop`. */
  release(): void;
};

// A pixel's leeway for a top that rounds to just below the view's.
const leeway = 1;

/**
 * The anchoring of `list` in `scroller`, whose view starts at `viewTop`, by its `items`. The anchor's place is taken in
 * the scroller's content, so that scrolling does not move it and only a change above it does.
 */
export function anchoring<T extends Box>({
  list,
  items,
  scroller,
  viewTop,
}: {
  list: Box;
  items: () => Iterable<T>;
  scroller: Scroller;
  viewTop: () => number;
}): Anchoring {
  let anchor: { element: T; at: number } | undefined;
  let top = true;
  let holding = false;
  // A scroller may round what it is set to (Safari to whole pixels): what a correction fell short by is carried to
  // the next one, so that many insertions do not add up to a visible drift. It lasts until the reader scrolls.
  let short = 0;
  let corrected: number | undefined;
  const placeOf = (el: Box, from: number) => el.getBoundingClientRect().top - from + scroller.scrollTop;
  const measure = () => {
    if (corrected !== scroller.scrollTop) {
      short = 0;
      corrected = undefined;
    }
    const from = viewTop();
    top = list.getBoundingClientRect().top >= from - leeway;
    if (top) holding = false;
    const first = firstInView(items(), from);
    anchor = first && { element: first.element, at: placeOf(first.element, from) };
  };
  return {
    measure,
    keep() {
      // An anchor taken away keeps nothing: what stood under it moves up.
      if (anchor && anchor.element.isConnected !== false && !top && !holding) {
        const moved = placeOf(anchor.element, viewTop()) - anchor.at;
        if (moved !== 0) {
          const to = scroller.scrollTop + moved + short;
          scroller.scrollTop = to;
          short = to - scroller.scrollTop;
          corrected = scroller.scrollTop;
        }
      }
      measure();
    },
    atTop: () => top,
    holdUntilTop() {
      holding = true;
    },
    release() {
      holding = false;
    },
  };
}

/** Scrolls `scroller` to its top, gliding unless the reader asked for reduced motion. */
export function scrollToTop(scroller: Element): void {
  const reduced = scroller.ownerDocument.defaultView?.matchMedia("(prefers-reduced-motion: reduce)").matches ?? true;
  scroller.scrollTo({ top: 0, behavior: reduced ? "instant" : "smooth" });
}

/** A saved position: the key of the element at the top of the view and its offset, and the plain offset. */
type SavedPosition = { top: number; anchor?: { key: string; offset: number } };

const keyAttribute = "data-scroll-key";

/** The position of the page's view now, by the elements with a `data-scroll-key`. */
export function positionOf(doc: Document): SavedPosition {
  const scroller = doc.scrollingElement ?? doc.documentElement;
  const first = firstInView(doc.querySelectorAll(`[${keyAttribute}]`), viewTopOf(scroller));
  const key = first?.element.getAttribute(keyAttribute);
  return { top: scroller.scrollTop, ...(first && key ? { anchor: { key, offset: first.offset } } : {}) };
}

/**
 * Brings the page's view to `saved`: the element of its key back at its offset, else the plain offset when that
 * element is gone.
 */
export function restorePosition(doc: Document, saved: SavedPosition): void {
  const scroller = doc.scrollingElement ?? doc.documentElement;
  const key = saved.anchor?.key;
  const element = Array.from(doc.querySelectorAll(`[${keyAttribute}]`)).find(
    (el) => el.getAttribute(keyAttribute) === key,
  );
  if (saved.anchor && element) {
    scroller.scrollTop += element.getBoundingClientRect().top - viewTopOf(scroller) - saved.anchor.offset;
  } else {
    scroller.scrollTop = saved.top;
  }
}

const saved = new Map<string, SavedPosition>();

/** What a screen does with the page's position when it shows: restore the one it was left at, or open at the top. */
export type ScrollMode = "restore" | "top";

/**
 * The page's position for the screen of `key`: while it shows, it is saved as the reader scrolls; when it shows,
 * `restore` brings back the position it was left at (the top the first time), and `top` opens it at the top. The
 * positions last as long as the page.
 */
export function useScrollRestoration(key: string, mode: ScrollMode): void {
  useLayoutEffect(() => {
    const was = mode === "restore" ? saved.get(key) : undefined;
    if (was) restorePosition(document, was);
    else window.scrollTo({ top: 0, behavior: "instant" });
    // Saved at once, not a frame later, when the next screen may have replaced this one. A browser fires scroll once a
    // frame at most.
    const onScroll = () => {
      saved.set(key, positionOf(document));
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      window.removeEventListener("scroll", onScroll);
    };
  }, [key, mode]);
}
