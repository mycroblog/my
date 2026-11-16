// SPDX-License-Identifier: AGPL-3.0-only

// Search, against the server: an account of this server by a part in the middle of its name (FR-307); a hashtag by its
// start, with the count of its posts, which opens its timeline (FR-306).

import { randomUUID } from "node:crypto";
import { Hashtag, Search } from "./screens";
import { expect, test } from "./test";

/** A hashtag no other test or run uses, so that the search finds this test's post alone. */
const freshTag = () => `crumb${randomUUID().replaceAll("-", "").slice(0, 12)}`;

test("the search finds an account of this server by a part in the middle of its name", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const reader = await fresh.person();

  await signIn(reader);
  const search = new Search(page);
  await search.goTo();
  // "Fresh 1a2b3c4d": the words "a2b3c", which start no word of the name.
  await search.find(author.displayName.slice("Fresh ".length + 1, -1));
  await expect(search.account(author.displayName).row).toBeVisible();
});

test("the search finds a hashtag by its start with the count of its posts, and the hashtag opens its timeline", async ({
  page,
  signIn,
  fresh,
}) => {
  const tag = freshTag();
  const writer = await fresh.person();
  const { text } = await fresh.post(writer, { text: `Crumb shot of the rye. #${tag}` });
  const reader = await fresh.person();

  await signIn(reader);
  const search = new Search(page);
  await search.goTo();
  await search.find(tag.slice(0, -3));
  const found = search.hashtag(tag, "1 post");
  await expect(found).toBeVisible();
  await found.click();

  const hashtag = new Hashtag(page, tag);
  await expect(hashtag.heading).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/tags/${tag}$`));
  await expect(hashtag.post(writer.displayName, text).article).toBeVisible();
});
