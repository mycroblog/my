// SPDX-License-Identifier: AGPL-3.0-only

// The grid of an entry's pictures (media.tsx): one alone, two side by side, three with the first as tall as the two
// after it, four in two rows, in a box 16:9 of its width.

import { expectNoComputedViolations } from "../axe";
import { boxesAtRest, expectMirrored, expectNoSidewaysScroll, type Box } from "../boxes";
import type { Locator } from "@playwright/test";
import { expect, test } from "../test";

const story = "design-system-media--default";

/** The box around every picture: the grid's. */
const around = (boxes: Box[]) => ({
  left: Math.min(...boxes.map((b) => b.left)),
  right: Math.max(...boxes.map((b) => b.right)),
  top: Math.min(...boxes.map((b) => b.top)),
  bottom: Math.max(...boxes.map((b) => b.bottom)),
});

for (const count of [1, 2, 3, 4] as const)
  test(`${String(count)} ${count === 1 ? "picture fills" : "pictures share"} a box 16:9 of the column's width`, async ({
    page,
    open,
  }) => {
    // The pictures by their descriptions; the second has none, and is found by what it does, in English.
    const described = {
      dawn: page.getByRole("img", { name: "Low hills at dawn under a peach sky", exact: true }),
      forest: page.getByRole("img", { name: "Green hills under a pale sky", exact: true }),
      night: page.getByRole("img", { name: "A night sky over dark hills", exact: true }),
    };
    const sea = page.getByRole("button", { name: "Open image", exact: true });
    const shown: Record<string, Locator> = { dawn: described.dawn };
    if (count > 2) shown["forest"] = described.forest;
    if (count > 3) shown["night"] = described.night;

    await open(story, { args: { count } });
    await expectNoSidewaysScroll(page);
    const ltr = await boxesAtRest(count > 1 ? { ...shown, sea } : shown);
    const { dawn: a, sea: b, forest: c, night: d } = ltr;
    if (a === undefined) throw new Error("the first picture is always there");
    const grid = around(Object.values(ltr));
    expect(Math.abs((grid.right - grid.left) * (9 / 16) - (grid.bottom - grid.top))).toBeLessThan(1.5);
    expect(grid.right - grid.left).toBeGreaterThan(280);
    if (b !== undefined) {
      // From two: the second beside the first, as wide.
      expect(b.top).toBe(a.top);
      expect(b.left).toBeGreaterThanOrEqual(a.right);
      expect(Math.abs(b.width - a.width)).toBeLessThan(1);
    }
    if (b !== undefined && c !== undefined && d === undefined) {
      // Three: the first as tall as the box, the third under the second.
      expect(a.height).toBeGreaterThan(b.height * 2);
      expect(c.top).toBeGreaterThanOrEqual(b.bottom);
      expect(c.left).toBe(b.left);
    }
    if (b !== undefined && c !== undefined && d !== undefined) {
      // Four: two rows of two.
      expect(c.top).toBeGreaterThanOrEqual(a.bottom);
      expect(c.left).toBe(a.left);
      expect(d.top).toBe(c.top);
      expect(d.left).toBe(b.left);
    }
    if (count === 4) await expectNoComputedViolations(page);
    if (count % 2 === 0) {
      await open(story, { locale: "ar-XB", args: { count } });
      await expectNoSidewaysScroll(page);
      await expectMirrored(page, ltr, shown);
    }
  });
