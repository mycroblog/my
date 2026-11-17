// SPDX-License-Identifier: AGPL-3.0-only

// AttachmentList and PollSettings (composer.tsx): the rows of a post's images, and a poll's settings.

import { expectNoComputedViolations } from "../axe";
import { boxesAtRest, expectInside, expectMirrored, expectNoSidewaysScroll } from "../boxes";
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
