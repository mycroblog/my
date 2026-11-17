// SPDX-License-Identifier: AGPL-3.0-only

// A person follows and unfollows another from their profile, and follows one of that person's followers from its row
// in the list the profile's counts open; the server keeps each.

import { countLink, Follows, Profile } from "./screens";
import { expect, test } from "./test";

test("a person follows another from their profile and one of their followers from its row, then unfollows, and each stays after a reload", async ({
  page,
  signIn,
  fresh,
}) => {
  const follower = await fresh.person();
  const followee = await fresh.person({ followedBy: [follower] });
  await signIn(await fresh.person());
  const profile = new Profile(page, followee);
  await profile.open();
  await profile.follow();
  await expect(profile.followingButton).toBeVisible();
  await page.reload();
  await expect(profile.followingButton).toBeVisible();

  // The followers: the one who followed before, whose row offers a follow, and the reader, whose row offers none.
  await countLink(profile.card, "Followers").click();
  const follows = new Follows(page, followee);
  await expect(page).toHaveURL(follows.address("Followers"));
  await expect(follows.tab("Followers")).toHaveAttribute("aria-selected", "true");
  await expect(follows.row(follower)).toBeVisible();
  await expect(follows.followButtons).toHaveCount(1);
  await follows.followTheOne();
  await expect(follows.followButtons).toHaveCount(0);
  await page.reload();
  await expect(follows.row(follower)).toBeVisible();
  await expect(follows.followButtons).toHaveCount(0);
  await follows.row(follower).click();
  await expect(new Profile(page, follower).followingButton).toBeVisible();

  await profile.open();
  await profile.unfollow();
  await expect(profile.followButton).toBeVisible();
  await page.reload();
  await expect(profile.followButton).toBeVisible();
});
