// SPDX-License-Identifier: AGPL-3.0-only

// A visitor without a session reads a person's profile and a post's thread, as the server shows them to everyone;
// Follow and a post's Favourite send them to the sign-in, which comes back to the profile or the post.

import { Profile, SignIn, Thread } from "./screens";
import { expect, test } from "./test";

test("a visitor reads a profile and a thread signed out, and Follow sends them through the sign-in back to the profile", async ({
  page,
  fresh,
}) => {
  const writer = await fresh.person();
  const post = await fresh.post(writer, { text: "Rye starter, day four: it doubled overnight." });
  const replier = await fresh.person();
  const reply = await fresh.post(replier, { text: "Mine took a week to get there.", replyTo: post.id });

  const profile = new Profile(page, writer);
  await profile.open();
  await expect(profile.heading).toBeVisible();
  await profile.posts.post(writer.displayName, post.text).open();

  const thread = new Thread(page);
  await expect(thread.heading).toBeVisible();
  await expect(thread.post(writer.displayName, post.text).article).toBeVisible();
  await expect(thread.post(replier.displayName, reply.text).article).toBeVisible();
  await thread.back();

  await expect(profile.heading).toBeVisible();
  await profile.followButton.click();
  const signIn = new SignIn(page);
  await expect(signIn.heading).toBeVisible();
  const reader = await fresh.person();
  await signIn.withPassword(reader.email, reader.password);
  await expect(profile.heading).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/@${writer.username}$`));
  // Signed in now, the profile offers the reader's own follow.
  await expect(profile.followButton).toBeVisible();
});

test("a post's Favourite sends a visitor through the sign-in back to the post", async ({ page, fresh }) => {
  const writer = await fresh.person();
  const post = await fresh.post(writer, { text: "The crumb is open at last." });
  const profile = new Profile(page, writer);
  await profile.open();
  await profile.posts.post(writer.displayName, post.text).open();
  const thread = new Thread(page);
  await expect(thread.heading).toBeVisible();
  const opened = new URL(page.url()).pathname;

  await thread.post(writer.displayName, post.text).favouriteButton.click();
  const signIn = new SignIn(page);
  await expect(signIn.heading).toBeVisible();
  const reader = await fresh.person();
  await signIn.withPassword(reader.email, reader.password);
  await expect(thread.heading).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`${opened}$`));
  await expect(thread.post(writer.displayName, post.text).favouriteButton).toBeVisible();
});
