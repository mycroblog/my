// SPDX-License-Identifier: AGPL-3.0-only

// A hashtag in a post opens the hashtag's timeline in the app, where the reader follows it and stops following it; a
// visitor reads it too, and Follow sends them to the sign-in and back.

import { randomUUID } from "node:crypto";
import { Hashtag, homeTimeline, SignIn } from "./screens";
import { expect, test } from "./test";

/** A hashtag no other test or run uses, so that its timeline holds this test's post alone. */
const freshTag = () => `bake${randomUUID().replaceAll("-", "").slice(0, 12)}`;

test("a hashtag in a post opens its timeline in the app, where the reader follows and unfollows it", async ({
  page,
  signIn,
  fresh,
}) => {
  const tag = freshTag();
  const writer = await fresh.person();
  const { text } = await fresh.post(writer, { text: `The dough is proofing. #${tag}` });
  await signIn(writer);
  await homeTimeline(page).post(writer.displayName, text).hashtag(tag).click();

  // In place: the same tab, at the app's address of the hashtag.
  const hashtag = new Hashtag(page, tag);
  await expect(hashtag.heading).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/tags/${tag}$`));
  expect(page.context().pages()).toHaveLength(1);
  await expect(hashtag.post(writer.displayName, text).article).toBeVisible();

  await hashtag.follow();
  await expect(hashtag.followingButton).toBeVisible();
  await page.reload();
  await expect(hashtag.followingButton).toBeVisible();

  await hashtag.unfollow();
  await expect(hashtag.followButton).toBeVisible();
  await page.reload();
  await expect(hashtag.followButton).toBeVisible();
});

test("a visitor reads a hashtag's posts, and Follow sends them to the sign-in, which comes back to the hashtag", async ({
  page,
  fresh,
}) => {
  const tag = freshTag();
  const writer = await fresh.person();
  const { text } = await fresh.post(writer, { text: `Rye for the weekend. #${tag}` });
  const hashtag = new Hashtag(page, tag);
  await hashtag.open();
  await expect(hashtag.post(writer.displayName, text).article).toBeVisible();

  await hashtag.followButton.click();
  const signIn = new SignIn(page);
  await expect(signIn.heading).toBeVisible();
  const reader = await fresh.person();
  await signIn.withPassword(reader.email, reader.password);
  await expect(hashtag.heading).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/tags/${tag}$`));
});
