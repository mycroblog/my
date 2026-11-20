// SPDX-License-Identifier: AGPL-3.0-only

// A person blocks another from their profile, finds them among the blocked accounts of the settings' Blocks, mutes, and
// hashtags (#3202), and unblocks them there: the server keeps the block and then its end, so after a reload the list is
// without them and their profile offers to follow and to block again. Each person is a fresh one.
// What the lists show, how a long one is cut and paged, and what an undo says and where it leaves the focus are
// component tests on the fake (app-features, settings/relationships.dom.test.tsx and relationships.jsdom.test.tsx).

import { Profile, Relationships } from "./screens";
import { expect, test } from "./test";

test("a person blocks another from their profile, unblocks them from the settings' Blocks, mutes, and hashtags, and after a reload the list is without them and the profile offers Block again", async ({
  page,
  signIn,
  fresh,
}) => {
  const other = await fresh.person();
  await signIn(await fresh.person());
  const profile = new Profile(page, other);
  await profile.open();
  await expect(profile.followButton).toBeVisible();
  await profile.block();
  await expect(profile.unblockButton).toBeVisible();

  // The settings list the block, read from the server.
  const relationships = new Relationships(page);
  await relationships.goTo();
  const row = relationships.account("Blocked accounts", other.displayName);
  await expect(row).toBeVisible();
  await expect(relationships.undoButton("Blocked accounts", `@${other.username}`)).toBeVisible();

  await relationships.undo("Blocked accounts", `@${other.username}`);
  // The row stays until the reader leaves, saying what was done in place of its button.
  await expect(relationships.done(row, "Blocked accounts")).toBeVisible();
  await expect(relationships.undoButton("Blocked accounts", `@${other.username}`)).toHaveCount(0);

  await page.reload();
  await relationships.read();
  await expect(
    relationships.empty(
      "Blocked accounts",
      "You have not blocked anyone. To block an account, choose “Block…” in the menu of its profile or of one of its posts.",
    ),
  ).toBeVisible();
  await expect(row).toHaveCount(0);

  // The profile is as before the block: it can be followed, and blocked again.
  await profile.open();
  await expect(profile.followButton).toBeVisible();
  await expect(profile.unblockButton).toHaveCount(0);
  const blockAgain = await profile.moreItem("Block…");
  await expect(blockAgain).toBeVisible();
});
