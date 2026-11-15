// SPDX-License-Identifier: AGPL-3.0-only

// A post the reader bookmarks shows in their bookmarks, the newest bookmark first; one taken back there leaves them.

import { bookmarks, Frame, homeTimeline } from "./screens";
import { expect, test } from "./test";

test("a bookmarked post shows first in Bookmarks, and leaves them once its bookmark is taken back there", async ({
  page,
  signIn,
  fresh,
}) => {
  const writer = await fresh.person();
  const older = await fresh.post(writer, { text: "Notes on the starter, to read again." });
  const newer = await fresh.post(writer, { text: "The meetup's address, to keep." });
  await signIn(writer);
  const home = homeTimeline(page);
  // The newer post bookmarked first: the bookmarks go by when each was bookmarked, not by when it was posted.
  await home.post(writer.displayName, newer.text).bookmark();
  await home.post(writer.displayName, older.text).bookmark();

  await new Frame(page).goTo("Bookmarks");
  const saved = bookmarks(page);
  const first = saved.post(writer.displayName, older.text);
  const second = saved.post(writer.displayName, newer.text);
  await expect(first.article).toHaveAttribute("aria-posinset", "1");
  await expect(second.article).toHaveAttribute("aria-posinset", "2");

  await first.removeBookmark();
  await expect(first.article).toBeHidden();
  await expect(second.article).toHaveAttribute("aria-posinset", "1");
  await page.reload();
  await expect(second.article).toBeVisible();
  await expect(first.article).toBeHidden();
});
