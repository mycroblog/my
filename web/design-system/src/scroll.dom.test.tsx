// SPDX-License-Identifier: AGPL-3.0-only

// The page's position, saved and restored by the elements of the document that bear a key. The simulation lays
// nothing out, so each keyed element is told its box here, one under another, moving with the page's scroll: where a
// browser really puts them, and what its scroll padding and its own anchoring do, is a browser's to show.

import { afterEach, expect, test } from "bun:test";
import { render } from "@testing-library/react";
import { positionOf, restorePosition } from "./scroll";

/**
 * A page of keyed entries, each `height` tall, one under another from `at` in the content, scrolled to `scrollTop`;
 * `show` puts other entries in their place, as a list that changed while the reader was away.
 */
function keyedPage(keys: string[], { at = 20, height = 100, scrollTop = 0 } = {}) {
  const scroller = document.documentElement;
  const entries = (shown: string[]) => (
    <>
      {shown.map((key) => (
        <article
          key={key}
          data-scroll-key={key}
          ref={(entry) => {
            if (entry === null) return;
            entry.getBoundingClientRect = () => {
              const top = at + shown.indexOf(key) * height - scroller.scrollTop;
              return { top, bottom: top + height } as DOMRect;
            };
          }}
        />
      ))}
    </>
  );
  const { rerender } = render(entries(keys));
  scroller.scrollTop = scrollTop;
  return {
    scroller,
    show: (shown: string[]) => {
      rerender(entries(shown));
    },
  };
}

afterEach(() => {
  document.documentElement.scrollTop = 0;
});

test("it saves the element at the top of the view by its key, and its offset", () => {
  keyedPage(["c", "b", "a"], { scrollTop: 150 });

  expect(positionOf(document)).toEqual({ top: 150, anchor: { key: "b", offset: -30 } });
});

test("it brings that element back to its offset, though entries went in above while away", () => {
  const { scroller, show } = keyedPage(["c", "b", "a"], { scrollTop: 150 });
  const saved = positionOf(document);

  show(["e", "d", "c", "b", "a"]);
  scroller.scrollTop = 0;
  restorePosition(document, saved);

  expect(scroller.scrollTop).toBe(350);
  expect(positionOf(document).anchor).toEqual(saved.anchor);
});

test("without the element, it goes back to the plain offset", () => {
  const { scroller, show } = keyedPage(["c", "b", "a"], { scrollTop: 150 });
  const saved = positionOf(document);

  show(["z"]);
  scroller.scrollTop = 0;
  restorePosition(document, saved);

  expect(scroller.scrollTop).toBe(150);
});

test("a page with no keyed element is saved and restored by its plain offset", () => {
  const { scroller } = keyedPage([], { scrollTop: 240 });
  const saved = positionOf(document);
  expect(saved).toEqual({ top: 240 });

  scroller.scrollTop = 0;
  restorePosition(document, saved);

  expect(scroller.scrollTop).toBe(240);
});
