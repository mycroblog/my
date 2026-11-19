// SPDX-License-Identifier: AGPL-3.0-only

// The grid of an entry's pictures (media.tsx): one alone, two side by side, three with the first as tall as the two
// after it, four in two rows, in a box 16:9 of its width; and what covers a sensitive picture, its blurhash, drawn in
// the picture's place.

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

// One page opens the story with each count, a step each.
test("1 picture fills a box 16:9 of the column's width, and 2, 3 and 4 pictures share one", async ({ page, open }) => {
  // The pictures by their descriptions; the second has none, and is found by what it does, in English.
  const described = {
    dawn: page.getByRole("img", { name: "Low hills at dawn under a peach sky", exact: true }),
    forest: page.getByRole("img", { name: "Green hills under a pale sky", exact: true }),
    night: page.getByRole("img", { name: "A night sky over dark hills", exact: true }),
  };
  const sea = page.getByRole("button", { name: "Open image", exact: true });

  for (const count of [1, 2, 3, 4] as const)
    await test.step(`${String(count)} ${count === 1 ? "picture" : "pictures"}`, async () => {
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
});

/** The value of a string of blurhash's base 83. */
function base83(text: string): number {
  const digits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz#$%*+,-.:;=?@[]^_{|}~";
  return text.split("").reduce((value, digit) => value * 83 + digits.indexOf(digit), 0);
}

test("a sensitive picture is covered by its blurhash, drawn in the picture's box until it is shown", async ({
  page,
  open,
}) => {
  await open(story, { args: { count: 1, sensitive: true } });
  const show = page.getByRole("button", { name: "Show", exact: true });
  const picture = page.getByRole("img", { name: "Low hills at dawn under a peach sky", exact: true });
  await expect(show).toBeVisible();
  await expect(picture).toHaveCount(0);
  /**
   * The cover, a picture hidden from screen readers that only its pixels tell: its box, and the mean of its colours.
   */
  const cover = () =>
    page.evaluate(() => {
      const canvas = document.querySelector("canvas");
      const pixels = canvas?.getContext("2d")?.getImageData(0, 0, canvas.width, canvas.height).data;
      if (canvas === null || pixels === undefined) return undefined;
      const sum = [0, 0, 0];
      for (let i = 0; i < pixels.length; i += 4)
        for (let c = 0; c < 3; c += 1) sum[c] = (sum[c] ?? 0) + (pixels[i + c] ?? 0);
      const { width, height } = canvas.getBoundingClientRect();
      return { width, height, mean: sum.map((s) => Math.round(s / (pixels.length / 4))) };
    });
  // The story's picture of dawn has the blurhash IuPrad~U8_, whose third to sixth characters hold its mean colour.
  const mean = base83("Prad");
  const [red, green, blue] = [mean >> 16, (mean >> 8) & 255, mean & 255];
  await expect
    .poll(async () => {
      const drawn = (await cover())?.mean ?? [];
      return drawn.length === 3
        ? Math.max(...[red, green, blue].map((v, i) => Math.abs(v - (drawn[i] ?? 0))))
        : undefined;
    })
    .toBeLessThanOrEqual(16);
  // The cover has the box the picture will have: 16:9 of the column's width.
  await expect
    .poll(async () => {
      const box = await cover();
      return box === undefined ? undefined : Math.abs(box.width * (9 / 16) - box.height);
    })
    .toBeLessThan(1.5);
  const covered = await cover();
  await show.click();
  await expect(picture).toBeVisible();
  await expect.poll(async () => (await boxesAtRest({ picture })).picture.width).toBe(covered?.width);
});
