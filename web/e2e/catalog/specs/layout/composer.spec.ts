// SPDX-License-Identifier: AGPL-3.0-only

// AttachmentList and PollSettings (composer.tsx): the rows of a post's images, and a poll's settings; and the emoji
// button at the end of the text's first line.

import { expectNoComputedViolations } from "../axe";
import { boxesAtRest, boxOf, expectInside, expectMirrored, expectNoSidewaysScroll } from "../boxes";
import { expect, test } from "../test";

const story = "design-system-layout-parts--composer-parts";

test("the rows of images stand one under another with remove at their end, and a poll's settings stay in the page", async ({
  page,
  open,
}) => {
  const images = page.getByRole("list");
  const parts = {
    images,
    failed: images.getByRole("listitem").filter({ has: page.getByRole("status") }),
    described: images.getByRole("listitem").filter({ has: page.getByRole("textbox") }),
    description: images.getByRole("textbox", { name: "Description of image 2", exact: true }),
    retry: images.getByRole("button", { name: "Try again", exact: true }),
    removeFirst: images.getByRole("button", { name: "Remove image 1", exact: true }),
    removeSecond: images.getByRole("button", { name: "Remove image 2", exact: true }),
    duration: page.getByRole("button", { name: /^(Duration.*|1 day)$/ }),
    several: page.getByText("Allow several choices, as many as the voter likes", { exact: true }),
  };

  await open(story);
  await expectNoSidewaysScroll(page);
  const ltr = await boxesAtRest(parts);
  expect(ltr.described.top).toBeGreaterThanOrEqual(ltr.failed.bottom);
  expect(ltr.described.left).toBe(ltr.failed.left);
  expect(ltr.described.right).toBe(ltr.failed.right);
  // Remove ends each row, after what the row says of its image.
  expect(ltr.removeFirst.left).toBeGreaterThanOrEqual(ltr.retry.right);
  expect(ltr.removeSecond.left).toBeGreaterThanOrEqual(ltr.description.right);
  expect(ltr.removeSecond.right).toBe(ltr.removeFirst.right);
  await expectInside(parts.removeSecond, page);
  // The settings are under the rows, inside the page, and do not cover each other.
  expect(ltr.duration.top).toBeGreaterThanOrEqual(ltr.images.bottom);
  await expectInside(parts.duration, page);
  await expectInside(parts.several, page);
  expect(ltr.several.top >= ltr.duration.bottom || ltr.several.left >= ltr.duration.right).toBe(true);
  await expectNoComputedViolations(page);

  await open(story, { locale: "ar-XB" });
  await expectNoSidewaysScroll(page);
  await expectMirrored(page, ltr, parts);
});

for (const [id, locale, page_, text] of [
  ["design-system-composer--ended-left-to-right-text", "ar-XB", "right to left", "English"],
  ["design-system-composer--ended-right-to-left-text", "en", "left to right", "Arabic"],
] as const)
  test(`on a ${page_} page the emoji button at its end covers no character of the first line of a text in ${text}`, async ({
    page,
    open,
  }) => {
    await open(id, { locale });
    const box = page.getByRole("textbox");
    const button = page.getByRole("button", { name: "Add emoji", exact: true });
    await expect(box).toBeVisible();
    await expect
      .poll(async () => {
        const at = await boxOf(button);
        return box.evaluate((el, over) => {
          // The glyphs of its first text, not the boxes of the paragraphs around them.
          const first = el.ownerDocument.createTreeWalker(el, NodeFilter.SHOW_TEXT).nextNode();
          if (first === null) return "the text box holds no text";
          const range = el.ownerDocument.createRange();
          range.selectNodeContents(first);
          const rects = Array.from(range.getClientRects()).filter((r) => r.width > 0 && r.height > 0);
          if (rects.length === 0) return "the text has no first line";
          const top = Math.min(...rects.map((r) => r.top));
          const covered = rects
            .filter((r) => r.top < top + 1)
            .some((r) => r.left < over.right && over.left < r.right && r.top < over.bottom && over.top < r.bottom);
          return covered ? "the emoji button covers the text's first line" : "clear";
        }, at);
      })
      .toBe("clear");
  });
