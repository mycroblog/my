// SPDX-License-Identifier: AGPL-3.0-only

// A person favourites and boosts a post, and the server keeps both: they show after a reload.

import { homeTimeline } from "./screens";
import { expect, person, test } from "./test";

test("a favourite and a boost stay after a reload", async ({ page, signIn }) => {
  const sam = person("sam");
  await signIn(person("layla"));
  const post = homeTimeline(page).post(sam.displayName, "The ending of the film we watched");
  await expect(post.favourited).toBeHidden();
  await expect(post.boosted).toBeHidden();
  await post.favourite();
  await expect(post.favourited).toHaveAccessibleName("Favourite, 1");
  await post.boost();
  await expect(post.boosted).toHaveAccessibleName("Boost, 1");
  await page.reload();
  await expect(post.favourited).toHaveAccessibleName("Favourite, 1");
  await expect(post.boosted).toHaveAccessibleName("Boost, 1");
});
