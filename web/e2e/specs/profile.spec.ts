// SPDX-License-Identifier: AGPL-3.0-only

// A person follows and unfollows another from their profile.

import { Profile } from "./screens";
import { expect, test } from "./test";

test("a person follows another and unfollows them, and each stays after a reload", async ({ page, signIn, fresh }) => {
  const followee = await fresh.person();
  await signIn(await fresh.person());
  const profile = new Profile(page, followee);
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
