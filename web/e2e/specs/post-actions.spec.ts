// SPDX-License-Identifier: AGPL-3.0-only

// A person favourites, boosts and bookmarks a post, and the server keeps each: they show after a reload, the bookmarks
// in Bookmarks, the newest bookmark first, where one taken back stays gone.

import { bookmarks, Frame, homeTimeline } from "./screens";
import { expect, test } from "./test";

test("a favourite, a boost and a bookmark stay after a reload, and a bookmark taken back in Bookmarks stays gone", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const older = await fresh.post(author, { text: "Notes on the starter, to read again." });
  const newer = await fresh.post(author, { text: "The ending of the film we watched." });
  await signIn(await fresh.person({ follows: [author] }));
  const home = homeTimeline(page);
  const post = home.post(author.displayName, newer.text);
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
  // The newer post bookmarked first: the bookmarks go by when each was bookmarked, not by when it was posted.
  await post.bookmark();
  await home.post(author.displayName, older.text).bookmark();
  await page.reload();
  await expect(post.favourited).toHaveAccessibleName("Favourite, 1");
  await expect(post.boosted).toHaveAccessibleName("Boost, 1");

  await new Frame(page).goTo("Bookmarks");
  const saved = bookmarks(page);
  const first = saved.post(author.displayName, older.text);
  const second = saved.post(author.displayName, newer.text);
  await expect(first.article).toHaveAttribute("aria-posinset", "1");
  await expect(second.article).toHaveAttribute("aria-posinset", "2");

  await first.removeBookmark();
  await expect(first.article).toBeHidden();
  await expect(second.article).toHaveAttribute("aria-posinset", "1");
  await page.reload();
  await expect(second.article).toBeVisible();
  await expect(first.article).toBeHidden();
});
