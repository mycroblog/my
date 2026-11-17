// SPDX-License-Identifier: AGPL-3.0-only

// ThreadReply (thread.tsx): a reply indents by its depth, up to three steps, on the start side, and only where its
// column has the room.

import { boxesAtRest, expectMirrored, expectNoSidewaysScroll } from "../boxes";
import { expect, pc, test } from "../test";

const story = "design-system-layout-parts--thread-replies";

test("a reply indents a step per depth from the second, three at most, on the start side; none on a phone", async ({
  page,
  open,
}) => {
  const reply = (depth: number) => page.getByText(`A reply at depth ${String(depth)}.`, { exact: true });
  const parts = {
    post: page.getByText("The post the replies are under.", { exact: true }),
    first: reply(1),
    second: reply(2),
    third: reply(3),
    fourth: reply(4),
    ninth: reply(9),
  };

  await open(story, { width: pc });
  await expectNoSidewaysScroll(page);
  const ltr = await boxesAtRest(parts);
  const step = ltr.second.left - ltr.first.left;
  expect(ltr.first.left).toBe(ltr.post.left);
  expect(step).toBeGreaterThan(0);
  expect(ltr.third.left - ltr.first.left).toBe(2 * step);
  expect(ltr.fourth.left - ltr.first.left).toBe(3 * step);
  expect(ltr.ninth.left).toBe(ltr.fourth.left);
  // The indent takes from the start alone: every reply ends where the post does.
  expect(ltr.ninth.right).toBe(ltr.post.right);

  // On a phone the column has no room to give: every reply starts where the post does.
  await page.setViewportSize({ width: 320, height: 640 });
  await expectNoSidewaysScroll(page);
  await expect.poll(async () => (await boxesAtRest(parts)).ninth.left).toBe(ltr.post.left);

  await open(story, { width: pc, locale: "ar-XB" });
  await expectMirrored(page, ltr, parts);
});
