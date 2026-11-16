// SPDX-License-Identifier: AGPL-3.0-only

// A person opens the profile of an account that moved, and follows its link to the new account.

import { Moved, Profile } from "./screens";
import { expect, test } from "./test";

test("a person opens a moved account, follows its link to the new one, and follows that", async ({
  page,
  signIn,
  fresh,
}) => {
  const from = await fresh.person();
  const to = await fresh.person();
  await fresh.move(from, to);
  await signIn(await fresh.person());
  const old = new Profile(page, from);
  await old.open();
  await expect(old.heading).toBeVisible();
  const moved = new Moved(old.card, to);
  await expect(moved.line).toBeVisible();
  await expect(moved.goToNew).toBeVisible();
  await expect(moved.follows).toHaveCount(0);
  await moved.goToNew.click();
  const now = new Profile(page, to);
  await expect(page).toHaveURL(new RegExp(`/@${to.username}$`));
  await expect(now.heading).toBeVisible();
  await now.follow();
  await expect(now.followingButton).toBeVisible();
});

test("the line of a moved account links to the new one", async ({ page, signIn, fresh }) => {
  const from = await fresh.person();
  const to = await fresh.person();
  await fresh.move(from, to);
  await signIn(await fresh.person());
  const old = new Profile(page, from);
  await old.open();
  const moved = new Moved(old.card, to);
  await moved.link.click();
  await expect(new Profile(page, to).heading).toBeVisible();
});
