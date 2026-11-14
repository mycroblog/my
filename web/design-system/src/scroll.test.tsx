// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { anchoring, firstInView, positionOf, restorePosition } from "./scroll";

/** A stand-in for a page whose keyed elements have a height each, one under another from `at` in the content. */
function keyedPage(keys: string[], { at = 20, height = 100, scrollTop = 0 } = {}) {
  const scroller: { scrollTop: number; ownerDocument?: Document } = { scrollTop };
  const state = { keys };
  const element = (key: string) => ({
    getAttribute: () => key,
    getBoundingClientRect() {
      const top = at + state.keys.indexOf(key) * height - scroller.scrollTop;
      return { top, bottom: top + height };
    },
  });
  const doc = {
    scrollingElement: scroller,
    documentElement: scroller,
    defaultView: { getComputedStyle: () => ({ scrollPaddingTop: "0px" }) },
    querySelectorAll: () => state.keys.map(element),
  } as unknown as Document;
  scroller.ownerDocument = doc;
  return { doc, scroller, state };
}

describe("saving and restoring a position", () => {
  test("it saves the element at the top of the view by its key, and its offset", () => {
    const { doc } = keyedPage(["c", "b", "a"], { scrollTop: 150 });
    expect(positionOf(doc)).toEqual({ top: 150, anchor: { key: "b", offset: -30 } });
  });

  test("it brings that element back to its offset, though entries went in above while away", () => {
    const { doc, scroller, state } = keyedPage(["c", "b", "a"], { scrollTop: 150 });
    const saved = positionOf(doc);
    state.keys = ["e", "d", "c", "b", "a"];
    scroller.scrollTop = 0;
    restorePosition(doc, saved);
    expect(scroller.scrollTop).toBe(350);
    expect(positionOf(doc).anchor).toEqual(saved.anchor);
  });

  test("without the element, it goes back to the plain offset", () => {
    const { doc, scroller, state } = keyedPage(["c", "b", "a"], { scrollTop: 150 });
    const saved = positionOf(doc);
    state.keys = ["z"];
    scroller.scrollTop = 0;
    restorePosition(doc, saved);
    expect(scroller.scrollTop).toBe(150);
  });
});

/**
 * A stand-in for a scroller and the items of a list in it: each item has a height and sits under the one before, the
 * list starting at `listAt` in the content, and the viewport's coordinates are the content's less the scroll.
 */
function page({ heights, listAt = 20, scrollTop = 0 }: { heights: number[]; listAt?: number; scrollTop?: number }) {
  const scroller = { scrollTop, getBoundingClientRect: () => ({ top: 0, bottom: 800 }) };
  const state = { heights, listAt };
  type Item = { index: number; isConnected: boolean; getBoundingClientRect(): { top: number; bottom: number } };
  const items: Item[] = [];
  const topOf = (i: number) => state.listAt + state.heights.slice(0, i).reduce((a, b) => a + b, 0);
  const item = (index: number): Item => ({
    index,
    isConnected: true,
    getBoundingClientRect() {
      const top = topOf(this.index) - scroller.scrollTop;
      return { top, bottom: top + (state.heights[this.index] ?? 0) };
    },
  });
  for (let i = 0; i < heights.length; i++) items.push(item(i));
  const list = { getBoundingClientRect: () => ({ top: state.listAt - scroller.scrollTop, bottom: 0 }) };
  /** Inserts items of `added` heights at the start of the list. */
  const insert = (...added: number[]) => {
    state.heights = [...added, ...state.heights];
    for (const it of items) it.index += added.length;
    items.unshift(...added.map((_, i) => item(i)));
  };
  const a = anchoring({ list, items: () => items, scroller, viewTop: () => 0 });
  return { scroller, items, insert, a, state };
}

describe("the first item in view", () => {
  test("is the first whose bottom is below the view's top, with its top's distance below it", () => {
    const { items } = page({ heights: [100, 100, 100], scrollTop: 150 });
    const found = firstInView(items, 0);
    expect(found?.element.index).toBe(1);
    // The second starts at 20 + 100 = 120 in the content, 30 above the view.
    expect(found?.offset).toBe(-30);
    expect(firstInView([], 0)).toBeUndefined();
  });
});

describe("anchoring", () => {
  test("away from the top, items inserted above leave what the reader sees in place", () => {
    const { scroller, items, insert, a } = page({ heights: [100, 100, 100], scrollTop: 150 });
    a.measure();
    expect(a.atTop()).toBe(false);
    const seen = items[1];
    const before = seen?.getBoundingClientRect().top;
    insert(80, 40);
    a.keep();
    expect(scroller.scrollTop).toBe(270);
    expect(seen?.getBoundingClientRect().top).toBe(before);
  });

  test("at the top, what is inserted shows", () => {
    const { scroller, insert, a } = page({ heights: [100, 100], scrollTop: 10 });
    a.measure();
    expect(a.atTop()).toBe(true);
    insert(80);
    a.keep();
    expect(scroller.scrollTop).toBe(10);
  });

  test("scrolling does not move the anchor, so that it never undoes the reader's scroll", () => {
    const { scroller, a } = page({ heights: [100, 100, 100], scrollTop: 150 });
    a.measure();
    // The reader (or a glide to a focused entry) scrolls, and a render comes before the scroll event.
    scroller.scrollTop = 180;
    a.keep();
    expect(scroller.scrollTop).toBe(180);
  });

  test("an item above that grows keeps the place too", () => {
    const { scroller, a, state } = page({ heights: [100, 100, 100], scrollTop: 250 });
    a.measure();
    state.heights = [160, 100, 100];
    a.keep();
    expect(scroller.scrollTop).toBe(310);
  });

  test("on the way to the top it keeps nothing, until the top or a release", () => {
    const { scroller, insert, a } = page({ heights: [100, 100, 100], scrollTop: 150 });
    a.measure();
    a.holdUntilTop();
    insert(50);
    a.keep();
    expect(scroller.scrollTop).toBe(150);
    a.release();
    insert(50);
    a.keep();
    expect(scroller.scrollTop).toBe(200);
    // Reaching the top ends the hold.
    a.holdUntilTop();
    scroller.scrollTop = 0;
    a.measure();
    scroller.scrollTop = 150;
    a.measure();
    insert(50);
    a.keep();
    expect(scroller.scrollTop).toBe(200);
  });

  test("a scroller that rounds to whole pixels does not drift over many insertions", () => {
    const { scroller, items, insert, a } = page({ heights: [100, 100, 100], scrollTop: 150 });
    let px = scroller.scrollTop;
    Object.defineProperty(scroller, "scrollTop", {
      get: () => px,
      set: (v: number) => {
        px = Math.round(v);
      },
    });
    a.measure();
    const seen = items[1];
    const before = seen?.getBoundingClientRect().top ?? 0;
    for (let i = 0; i < 10; i++) {
      insert(40.4);
      a.keep();
    }
    expect(Math.abs((seen?.getBoundingClientRect().top ?? 0) - before)).toBeLessThan(1);
  });

  test("an anchor taken away keeps nothing", () => {
    const { scroller, items, insert, a } = page({ heights: [100, 100, 100], scrollTop: 150 });
    a.measure();
    const gone = items[1];
    if (gone) gone.isConnected = false;
    insert(50);
    a.keep();
    expect(scroller.scrollTop).toBe(150);
  });
});
