// SPDX-License-Identifier: AGPL-3.0-only

// The body of a text people wrote (content.tsx): a word no line holds wraps anywhere, a mention and a hashtag never
// break inside, the hidden parts of a shortened address take no room, and quotes and lists indent on the start side.

import { expectNoComputedViolations } from "../axe";
import { boxesAtRest, boxOf, expectInside, expectMirrored, expectNoSidewaysScroll, linesOf } from "../boxes";
import { expect, minimum, test } from "../test";

const story = "design-system-layout-parts--written-text";

test("a written text at the minimum width: long words wrap, links stay whole, quotes and lists indent", async ({
  page,
  open,
}) => {
  const parts = {
    word: page.getByText(/^A word no line holds: .+$/),
    quote: page.getByRole("blockquote").getByRole("paragraph"),
    item: page.getByRole("listitem").filter({ hasText: /^A first item of a list\. .+$/ }),
    step: page.getByRole("listitem").filter({ hasText: /^A first step\. .+$/ }),
    code: page.getByRole("code"),
  };
  const mention = page.getByRole("link", { name: "@bartholomew_maximilian", exact: true });
  const hashtag = page.getByRole("link", { name: "#SlowMorningsByTheSea", exact: true });

  await open(story);
  await expectNoSidewaysScroll(page);
  const ltr = await boxesAtRest(parts);

  await test.step("a word no line holds wraps inside the body", async () => {
    await expect.poll(() => linesOf(parts.word)).toBeGreaterThan(2);
    await expectInside(parts.word, page);
    await expectInside(parts.code, page);
  });

  await test.step("quotes and lists indent on the start side, and end where the paragraphs do", () => {
    for (const block of [ltr.quote, ltr.item, ltr.step]) {
      expect(block.left).toBeGreaterThan(ltr.word.left);
      expect(block.right).toBe(ltr.word.right);
    }
  });

  await test.step("the hidden parts of a shortened address take no room, and the shown part ends in an ellipsis", async () => {
    const address = page.getByRole("link", { name: /^https:\/\/\s*example\.com\/a\/very\/long\/path\/to….*$/ });
    const shown = address.getByText("example.com/a/very/long/path/to", { exact: true });
    for (const hidden of ["https://", "/a/page/that/goes/on?and=on"])
      await expect
        .poll(async () => {
          const box = await boxOf(address.getByText(hidden, { exact: true }));
          return box.width * box.height;
        })
        .toBe(0);
    await expect.poll(() => linesOf(address)).toBe(1);
    // The link is as wide as what it shows and the ellipsis after it.
    await expect.poll(async () => (await boxOf(address)).width - (await boxOf(shown)).width).toBeLessThan(24);
    await expect.poll(() => shown.evaluate((part) => getComputedStyle(part, "::after").content)).toBe('"…"');
  });

  await test.step("a mention and a hashtag stay on one line wherever the line ends", async () => {
    // Each width moves the end of the line across the mention and the hashtag.
    for (let width = minimum; width <= minimum + 160; width += 16) {
      await page.setViewportSize({ width, height: 640 });
      await expect.poll(async () => [await linesOf(mention), await linesOf(hashtag)]).toEqual([1, 1]);
      await expectInside(mention, page);
    }
    await page.setViewportSize({ width: minimum, height: 640 });
  });

  await expectNoComputedViolations(page);

  await open(story, { locale: "ar-XB" });
  await expectNoSidewaysScroll(page);
  await expectMirrored(page, ltr, parts);
});
