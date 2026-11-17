// SPDX-License-Identifier: AGPL-3.0-only

// A person opens another person's followers from the counts of their profile, and follows an account from its row.

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
