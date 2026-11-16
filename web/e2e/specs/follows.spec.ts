// SPDX-License-Identifier: AGPL-3.0-only

// A person opens another person's lists of follows and followers from the counts of their profile, follows an account
// from its row, and meets a list its owner keeps from them.

import { countLink, Follows, Profile } from "./screens";
import { expect, test } from "./test";

test("a person opens another's followers from their profile, and follows one of them from its row", async ({
  page,
  signIn,
  fresh,
}) => {
  const follower = await fresh.person();
  const owner = await fresh.person({ followedBy: [follower] });
  await signIn(await fresh.person());
  const profile = new Profile(page, owner);
  await profile.open();
  await countLink(profile.card, "Followers").click();
  const follows = new Follows(page, owner);
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
});

test("a tab of the lists replaces the address, and Back returns to the profile", async ({ page, signIn, fresh }) => {
  const followee = await fresh.person();
  const owner = await fresh.person({ follows: [followee] });
  await signIn(await fresh.person());
  const profile = new Profile(page, owner);
  await profile.open();
  await countLink(profile.card, "Followers").click();
  const follows = new Follows(page, owner);
  await expect(follows.tab("Followers")).toHaveAttribute("aria-selected", "true");
  await follows.tab("Following").click();
  await expect(page).toHaveURL(follows.address("Following"));
  await expect(follows.row(followee)).toBeVisible();
  // The tab replaced the list's address, so the browser's back leaves the lists for the profile.
  await page.goBack();
  await expect(profile.heading).toBeVisible();
  await countLink(profile.card, "Following").click();
  await expect(follows.row(followee)).toBeVisible();
  await follows.back.click();
  await expect(profile.heading).toBeVisible();
});

for (const visibility of ["followers", "private"] as const)
  test(`a list its owner shows to ${visibility === "private" ? "nobody" : "their followers"} says so to another`, async ({
    page,
    signIn,
    fresh,
  }) => {
    const follower = await fresh.person();
    const owner = await fresh.person({ followedBy: [follower], followsVisibility: visibility });
    await signIn(await fresh.person());
    const follows = new Follows(page, owner);
    await page.goto(`/@${owner.username}/followers`);
    await expect(follows.hidden).toBeVisible();
    await expect(follows.row(follower)).toHaveCount(0);
    await follows.tab("Following").click();
    await expect(follows.hidden).toBeVisible();
  });
