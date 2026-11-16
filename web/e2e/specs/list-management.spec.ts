// SPDX-License-Identifier: AGPL-3.0-only

// The reader makes a list on its index, adds an account they follow from its profile's Add to list, reads the list's
// timeline with that account's posts, then renames and deletes it from the list's menu (ADR-0013 §11–§15).

import { ListIndex, Profile } from "./screens";
import { expect, test } from "./test";

test("a list is made, given a followed account from its profile, read with its posts, renamed and deleted", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const post = await fresh.post(author, { text: "Sourdough starter is alive again." });
  const reader = await fresh.person({ follows: [author] });
  const name = `Bakers ${author.displayName.slice(-8)}`;

  await signIn(reader);
  const lists = new ListIndex(page);
  await lists.goTo();
  await lists.create(name);
  await expect(lists.list(name)).toHaveAccessibleName(`${name} 0 members`);

  const profile = new Profile(page, author);
  await profile.open();
  await expect(profile.heading).toBeVisible();
  await profile.addToList(name);
  await expect(profile.listChoice(name)).toBeChecked();
  await page.keyboard.press("Escape");

  await lists.goTo();
  await expect(lists.list(name)).toHaveAccessibleName(`${name} 1 member`);
  const list = await lists.open(name);
  await expect(list.heading).toBeVisible();
  await expect(list.post(author.displayName, post.text).article).toBeVisible();

  const renamed = `${name} and friends`;
  const again = await list.rename(renamed);
  await expect(again.heading).toBeVisible();
  await expect(list.heading).toBeHidden();

  await again.delete();
  await expect(lists.heading).toBeVisible();
  await expect(lists.list(renamed)).toBeHidden();
  await page.reload();
  await expect(lists.heading).toBeVisible();
  await expect(lists.list(renamed)).toBeHidden();
});
