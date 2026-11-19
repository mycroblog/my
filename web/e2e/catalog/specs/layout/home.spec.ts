// SPDX-License-Identifier: AGPL-3.0-only

// The home timeline on the app's screen stories: the people suggested to a reader who follows no one stand over the
// reader's own posts, and a post's link card is drawn in the size the reader chose (#3199), large with its picture over
// its text, or small with the picture as a square beside it.

import type { Page } from "@playwright/test";
import { boxesAtRest, expectNoSidewaysScroll } from "../boxes";
import { expect, minimum, pc, test } from "../test";

test("the people suggested to a reader who follows no one stand over the reader's posts", async ({ page, open }) => {
  await open("screens-search--new-account-home");
  const main = page.getByRole("main");
  const suggested = main.getByRole("list", { name: "Suggested for you", exact: true });
  const post = main.getByRole("feed", { name: "Home timeline", exact: true }).getByRole("article", {
    name: "QuickDeals",
    exact: true,
  });
  await expect(suggested.getByRole("button", { name: "Follow", exact: true })).not.toHaveCount(0);
  const at = await boxesAtRest({ suggested, post });
  expect(at.suggested.bottom).toBeLessThanOrEqual(at.post.top);
  await expectNoSidewaysScroll(page);
});

/** The card of the link in Layla's post in the home timeline: the card itself, its picture and its title. */
function linkCard(page: Page) {
  const card = page
    .getByRole("main")
    .getByRole("feed", { name: "Home timeline", exact: true })
    .getByRole("link", {
      name: /^Two speech bubbles over a calm sea .*Why jokes are the hardest thing to translate.*$/,
    });
  return {
    card,
    picture: card.getByRole("img", { name: "Two speech bubbles over a calm sea", exact: true }),
    title: card.getByText("Why jokes are the hardest thing to translate", { exact: true }),
  };
}

// The edges of two boxes a layout rounded to device pixels may differ by less than a pixel.
const slack = 0.75;

/** The widths a card is read at, on one load of its screen: a phone's and a desktop's. */
const widths = [minimum, pc];

test("a reader who chose no size gets a link card with its picture over its text, as wide as the card, at 320 px and 1440 px", async ({
  page,
  open,
}) => {
  await open("screens-timeline--normal", { width: minimum });
  for (const width of widths)
    await test.step(`at ${String(width)} px`, async () => {
      await page.setViewportSize({ width, height: 640 });
      const at = await boxesAtRest(linkCard(page));

      expect(at.picture.bottom).toBeLessThanOrEqual(at.title.top + slack);
      expect(Math.abs(at.picture.width - at.card.width)).toBeLessThanOrEqual(slack);
      await expectNoSidewaysScroll(page);
    });
});

test("a reader who chose small cards gets the picture as a square beside the text, at the card's start, at 320 px and 1440 px", async ({
  page,
  open,
}) => {
  await open("screens-timeline--small-cards", { width: minimum });
  for (const width of widths)
    await test.step(`at ${String(width)} px`, async () => {
      await page.setViewportSize({ width, height: 640 });
      const at = await boxesAtRest(linkCard(page));

      // Beside: the text begins where the picture ends, and stands within the picture's height or beyond it, not
      // under.
      expect(at.picture.right).toBeLessThanOrEqual(at.title.left + slack);
      expect(at.title.top).toBeLessThan(at.picture.bottom);
      expect(Math.abs(at.picture.left - at.card.left)).toBeLessThanOrEqual(slack);
      expect(Math.abs(at.picture.width - at.picture.height)).toBeLessThanOrEqual(slack);
      // A thumbnail, not the picture of a large card laid on its side.
      expect(at.picture.width).toBeLessThan(at.card.width / 2);
      await expectNoSidewaysScroll(page);
    });
});
