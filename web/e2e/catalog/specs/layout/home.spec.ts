// SPDX-License-Identifier: AGPL-3.0-only

// The home of a reader who follows no one, on the app's screen story: the people it suggests stand over the reader's
// own posts.

import { boxesAtRest, expectNoSidewaysScroll } from "../boxes";
import { expect, test } from "../test";

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
