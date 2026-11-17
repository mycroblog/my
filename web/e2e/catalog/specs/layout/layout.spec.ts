// SPDX-License-Identifier: AGPL-3.0-only

// The generic layout parts (layout.tsx): Row, Column, Stack, Grid, Layers and Pin given what they were not written
// for, with no prop that asks for anything: a word of 200 letters with no place to break, fifty children, a text four
// times its size. At the minimum width, in both directions, none makes its page wider than the viewport, and each
// child, and each text, stays inside what holds it.

import { atRest, boxesAtRest, expectNoSidewaysScroll, linesOf, reachingOut, textLines } from "../boxes";
import type { Page } from "@playwright/test";
import { expect, test } from "../test";

const long = /^A sentence long enough to run past the room its row leaves it.+$/;

/** What a story shows besides staying inside its page, by the story's name. */
const also: Record<string, (page: Page) => Promise<void>> = {
  "row-words": async (page) => {
    // The label gives way down to its longest word and no further: each of its lines is whole words, while the text
    // beside it wraps.
    const label = page.getByText("A value", { exact: true });
    await expect.poll(async () => (await textLines(label)).join(" ")).toBe("A value");
    await expect.poll(() => linesOf(page.getByText(long))).toBeGreaterThan(1);
  },
  "row-many-children": async (page) => {
    // The fifty wrap: the last stands on a line under the first.
    const at = await boxesAtRest({
      first: page.getByText("1", { exact: true }),
      last: page.getByText("50", { exact: true }),
    });
    expect(at.last.top).toBeGreaterThanOrEqual(at.first.bottom);
  },
  "row-many-children-scrolling": async (page) => {
    // The fifty keep their size on one line, and the row scrolls them inside itself.
    const row = page.getByRole("group", { name: "Numbers", exact: true });
    const at = await boxesAtRest({
      first: row.getByText("1", { exact: true }),
      last: row.getByText("50", { exact: true }),
    });
    expect(at.last.top).toBe(at.first.top);
    await expect.poll(() => row.evaluate((element) => element.scrollWidth - element.clientWidth)).toBeGreaterThan(0);
  },
};

// Defect: in a row inside a line of text, the glyph beside a word no line holds gives way below its own width (a box
// 2.5 px wide for a glyph of 12), and is painted over the first letters of the word.
const defects: Record<string, string> = {};

const stories = [
  "row-long-word",
  "row-bare-long-word",
  "row-large-text",
  "row-inline-long-word",
  "row-words",
  "row-many-children",
  "row-many-children-scrolling",
  "column-long-word",
  "stack-of-blocks",
  "stack-many-children",
  "stack-large-text",
  "grid-long-word",
  "grid-many-children",
  "layers-long-word",
  "pin-long-word",
];

for (const name of stories)
  test(`${name}: at the minimum width the part keeps its page's width and what it holds inside it, in both directions`, async ({
    page,
    open,
  }) => {
    test.fail(name in defects, defects[name]);
    for (const locale of ["en", "ar-XB"] as const) {
      await open(`design-system-layout--${name}`, { locale });
      await expectNoSidewaysScroll(page);
      const reaching = await atRest(() => reachingOut(page));
      expect({ locale, reaching }).toEqual({ locale, reaching: [] });
      await also[name]?.(page);
    }
  });
