// SPDX-License-Identifier: AGPL-3.0-only

// A person favourites and boosts a post, and the server keeps both: they show after a reload.

import { homeTimeline } from "./screens";
import { expect, test } from "./test";

test("a favourite and a boost stay after a reload", async ({ page, signIn, fresh }) => {
  const author = await fresh.person();
  const { text } = await fresh.post(author, { text: "The ending of the film we watched." });
  await signIn(await fresh.person({ follows: [author] }));
  const post = homeTimeline(page).post(author.displayName, text);
  await expect(post.favourited).toBeHidden();
  await expect(post.boosted).toBeHidden();
  // The boost goes without waiting for the favourite's answer: neither answer takes back the other mark. It starts
  // once the favourite button shows pressed, so that its menu does not open under the favourite's click.
  const favourite = post.favourite();
  await expect(post.favourited).toBeVisible();
  const boost = post.boost();
  await Promise.all([favourite, boost]);
  await expect(post.favourited).toHaveAccessibleName("Favourite, 1");
  await expect(post.boosted).toHaveAccessibleName("Boost, 1");
  await page.reload();
  await expect(post.favourited).toHaveAccessibleName("Favourite, 1");
  await expect(post.boosted).toHaveAccessibleName("Boost, 1");
});
