// SPDX-License-Identifier: AGPL-3.0-only

// The feed (feed.tsx): its entries one under another, the new entries' button floating over its top while the page
// scrolls, and the room it keeps above an entry it brings to the top.

import { expectNoComputedViolations } from "../axe";
import { boxesAtRest, boxOf, covered, expectInViewport, expectMirrored, expectNoSidewaysScroll } from "../boxes";
import { expect, test } from "../test";

const story = "design-system-layout-parts--long-feed";

test("a feed's entries stand one under another, and the new entries' button stays over its top as the page scrolls; a tight feed holds them closer together", async ({
  page,
  open,
}) => {
  const feed = page.getByRole("feed", { name: "Home", exact: true });
  const entry = (n: number) => feed.getByRole("article", { name: `Entry ${String(n)}`, exact: true });
  const button = page.getByRole("status").getByRole("button", { name: "12 new posts", exact: true });
  const parts = { feed, first: entry(1), second: entry(2), button };

  await open(story);
  await expectNoSidewaysScroll(page);
  const ltr = await boxesAtRest(parts);
  expect(ltr.second.top).toBeGreaterThanOrEqual(ltr.first.bottom);
  expect(ltr.second.left).toBe(ltr.first.left);
  expect(ltr.second.right).toBe(ltr.first.right);
  // The button is in the middle of the feed's width, over its first entry, and painted over it.
  expect(Math.abs((ltr.button.left + ltr.button.right) / 2 - (ltr.feed.left + ltr.feed.right) / 2)).toBeLessThan(1);
  expect(ltr.button.top).toBeGreaterThanOrEqual(ltr.feed.top);
  expect(ltr.button.bottom).toBeLessThan(ltr.first.bottom);
  await expect.poll(() => covered(button)).toEqual([]);
  await expectNoComputedViolations(page);

  await test.step("Page Down brings the next entry to the top, with the feed's room above it", async () => {
    await entry(1).focus();
    for (const n of [2, 3, 4]) {
      await page.keyboard.press("PageDown");
      await expect(entry(n)).toBeFocused();
      await expect.poll(async () => Math.round((await boxOf(entry(n))).top)).toBeGreaterThan(4);
      await expect.poll(async () => Math.round((await boxOf(entry(n))).top)).toBeLessThan(32);
    }
    // The page has scrolled, and the button is where it was, over what scrolls under it.
    await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(300);
    await expect.poll(async () => (await boxOf(button)).top).toBe(ltr.button.top);
    await expectInViewport(page, button);
    await expect.poll(() => covered(button)).toEqual([]);
  });

  await open(story, { locale: "ar-XB" });
  await expectNoSidewaysScroll(page);
  await expectMirrored(page, ltr, parts);

  await test.step("a tight feed holds its entries closer together", async () => {
    const pitch = async () => {
      const at = await boxesAtRest({ first: entry(1), second: entry(2) });
      return at.second.top - at.first.top;
    };
    await open(story, { args: { newItems: false } });
    await expect(page.getByRole("button", { name: "12 new posts", exact: true })).toHaveCount(0);
    const apart = await pitch();
    await open(story, { args: { newItems: false, tight: true } });
    await expect.poll(pitch).toBeLessThan(apart);
  });
});
