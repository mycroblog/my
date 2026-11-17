// SPDX-License-Identifier: AGPL-3.0-only

// Search, against the server: a reader writes a hashtag in the field, the hashtag found shows the count of its posts,
// and it opens its timeline (FR-306). Which words find which hashtag or account is the server's tests'
// (TestTimelineFlows, TestTagSearchAPI, TestAccountSearchFlows); the accounts found, and following one from them, are
// find.spec.ts.

import { randomUUID } from "node:crypto";
import { Hashtag, Search } from "./screens";
import { expect, test } from "./test";

/** A hashtag no other test or run uses, so that the search finds this test's post alone. */
const freshTag = () => `crumb${randomUUID().replaceAll("-", "").slice(0, 12)}`;

test("the search finds a hashtag with the count of its posts, and the hashtag opens its timeline", async ({
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
  await search.find(tag);
  const found = search.hashtag(tag, "1 post");
  await expect(found).toBeVisible();
  await found.click();

  const hashtag = new Hashtag(page, tag);
  await expect(hashtag.heading).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/tags/${tag}$`));
  await expect(hashtag.post(writer.displayName, text).article).toBeVisible();
});
