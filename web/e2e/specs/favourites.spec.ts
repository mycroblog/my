// SPDX-License-Identifier: AGPL-3.0-only

// A person favourites a post, finds it in Favourites, opened from the main navigation, and takes the favourite back
// there: the post leaves the list, and stays gone after a reload (#3366).

import { favourites, Frame, homeTimeline } from "./screens";
import { expect, test } from "./test";

test("a post favourited shows in Favourites, opened from the navigation, and a favourite taken back there leaves it", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const post = await fresh.post(author, { text: "The bread came out of the oven at noon." });
  await signIn(await fresh.person({ follows: [author] }));
  const home = homeTimeline(page).post(author.displayName, post.text);
  await home.favourite();
  await expect(home.favourited).toBeVisible();

  // From a tablet's width the main navigation holds Favourites, as it holds Bookmarks; the Menu holds them on a phone.
  const frame = new Frame(page);
  await frame.goTo("Favourites");
  await expect(frame.heading("Favourites")).toBeVisible();
  const listed = favourites(page).post(author.displayName, post.text);
  await expect(listed.favourited).toBeVisible();
  await expect(listed.article).toHaveAttribute("aria-posinset", "1");

  await listed.favourite();
  await expect(listed.article).toBeHidden();
  // The reader favourited nothing else: once read again, the list is empty.
  await page.reload();
  await expect(page.getByRole("main").getByText("No favourites yet.", { exact: true })).toBeVisible();
  await expect(listed.article).toBeHidden();
});
