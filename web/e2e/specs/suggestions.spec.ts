// SPDX-License-Identifier: AGPL-3.0-only

// The suggested people, against the server (FR-308): the home of a reader who follows no one suggests people to
// follow, over its posts.

import { homeSuggestions, homeTimeline } from "./screens";
import { expect, test } from "./test";

test("the home of a reader who follows no one suggests people to follow, over the reader's posts", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  await fresh.post(author, { text: "Fresh loaves at the market this morning." });
  const reader = await fresh.person();
  const { text } = await fresh.post(reader, { text: "Hello, this is my first post." });

  await signIn(reader);
  const suggested = homeSuggestions(page);
  await expect(suggested).toBeVisible();
  // Others write in parallel and the latest writers come first: which people are suggested is the server's choice.
  await expect(suggested.getByRole("button", { name: "Follow", exact: true })).not.toHaveCount(0);
  const post = homeTimeline(page).post(reader.displayName, text).article;
  await expect(post).toBeVisible();
  await expect
    .poll(async () => {
      const [over, under] = await Promise.all([suggested.boundingBox(), post.boundingBox()]);
      return over !== null && under !== null && over.y + over.height <= under.y;
    })
    .toBe(true);
});
