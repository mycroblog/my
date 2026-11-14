// SPDX-License-Identifier: AGPL-3.0-only

// A person reads another person's profile, and follows and unfollows them.

import { Frame, homeTimeline, Profile } from "./screens";
import { expect, person, test } from "./test";

test("another person's profile shows their note, fields and posts", async ({ page, signIn }) => {
  const sam = person("sam");
  await signIn(person("marco"));
  await homeTimeline(page).post(sam.displayName, "Day 3 of the sourdough").openAuthor(sam.displayName);
  const profile = new Profile(page, sam);
  await expect(profile.heading).toBeVisible();
  await expect(profile.paragraph(sam.note)).toBeVisible();
  await expect(profile.fieldName("Blog")).toBeVisible();
  await expect(profile.followingButton).toBeVisible();
  await expect(profile.posts.post(sam.displayName, "Day 3 of the sourdough").article).toBeVisible();
});

test("a person follows another and unfollows them, and each stays after a reload", async ({ page, signIn }) => {
  await signIn(person("marco"));
  const profile = new Profile(page, person("layla"));
  await profile.open();
  await profile.follow();
  await expect(profile.followingButton).toBeVisible();
  await page.reload();
  await expect(profile.followingButton).toBeVisible();
  await profile.unfollow();
  await expect(profile.followButton).toBeVisible();
  await page.reload();
  await expect(profile.followButton).toBeVisible();
});

test("the profile of a person who writes in Japanese keeps the reader's language", async ({ page, signIn }) => {
  test.fail(true, "the client shows its whole UI in the profile owner's locale (ja) to a reader whose locale is en");
  await signIn(person("marco"));
  const profile = new Profile(page, person("yuki"));
  await profile.open();
  await expect(profile.heading).toBeVisible();
  await expect(new Frame(page).navigation).toBeVisible({ timeout: 5_000 });
  await expect(profile.followButton).toBeVisible({ timeout: 5_000 });
});
