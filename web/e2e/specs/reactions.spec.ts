// SPDX-License-Identifier: AGPL-3.0-only

// A reader reacts to a post with the emoji picker (FR-116), and the server keeps each change (a reload shows it): a
// reaction added, changed and taken back, a favourite that takes the place of the reader's reaction, and a reaction
// that of their favourite. The list of who reacted with an emoji leaves once the post is deleted. The picker's keys and
// its search are its own tests' (web/design-system/src/emoji-picker.dom.test.tsx) and the features'
// (web/app-features/src/reactions.dom.test.tsx); the emoji the composer writes at the caret is a test in a browser on
// its story (catalog/specs/browser/composer.spec.ts).

import { homeTimeline, Reactors, Thread } from "./screens";
import { expect, test } from "./test";

test("a reader adds and changes a reaction, which a favourite replaces and which replaces a favourite, and takes it back, and the server keeps each", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const { text } = await fresh.post(author, { text: "The first frost on the allotment." });
  await signIn(await fresh.person({ follows: [author] }));
  const post = homeTimeline(page).post(author.displayName, text);
  await expect(post.favouriteButton).toBeVisible();
  await expect(post.reactions).toBeHidden();

  let picker = await post.react();
  await expect(picker.search).toBeFocused();
  await post.pick(picker.emoji("Smileys & emotion", "grinning face"));
  await expect(picker.dialog).toBeHidden();
  await expect(post.reactionChips).toHaveCount(1);
  await expect(post.ownReaction).toHaveAccessibleName("😀, 1, including yours");
  await page.reload();
  await expect(post.ownReaction).toHaveAccessibleName("😀, 1, including yours");

  picker = await post.react();
  await post.pick(picker.emoji("Custom emoji", ":teacup:"));
  await expect(post.reactionChips).toHaveCount(1);
  await expect(post.ownReaction).toHaveAccessibleName(":teacup:, 1, including yours");
  await page.reload();
  await expect(post.reactionChips).toHaveCount(1);
  await expect(post.ownReaction).toHaveAccessibleName(":teacup:, 1, including yours");

  // A favourite takes the place of the reader's reaction.
  await post.favourite();
  await expect(post.favourited).toHaveAccessibleName("Favourite, 1");
  await expect(post.reactions).toBeHidden();
  await page.reload();
  await expect(post.favourited).toHaveAccessibleName("Favourite, 1");
  await expect(post.reactions).toBeHidden();

  // And a reaction that of their favourite.
  picker = await post.react();
  await post.pick(picker.emoji("Custom emoji", ":teacup:"));
  await expect(post.ownReaction).toHaveAccessibleName(":teacup:, 1, including yours");
  await expect(post.favourited).toBeHidden();
  await expect(post.favouriteButton).toHaveAccessibleName("Favourite");
  await page.reload();
  await expect(post.ownReaction).toHaveAccessibleName(":teacup:, 1, including yours");
  await expect(post.favouriteButton).toHaveAccessibleName("Favourite");
  await expect(post.favourited).toBeHidden();

  // The picker knocks out the reader's own reaction, and picking it again takes it back.
  picker = await post.react();
  await post.pick(picker.emoji("Custom emoji", ":teacup:, your reaction"));
  await expect(picker.dialog).toBeHidden();
  await expect(post.reactions).toBeHidden();
  await page.reload();
  await expect(post.favouriteButton).toBeVisible();
  await expect(post.reactions).toBeHidden();
});

test("the list of who reacted to a post leaves once its author deletes the post", async ({ page, signIn, fresh }) => {
  const author = await fresh.person();
  const other = await fresh.person();
  const shared = await fresh.post(author, { text: "Our bakery closes for the winter." });
  await fresh.reaction(other, shared, "teacup");
  await signIn(await fresh.person({ follows: [author] }));
  await homeTimeline(page).post(author.displayName, shared.text).open();
  const thread = new Thread(page);
  await thread.post(author.displayName, shared.text).whoReacted(":teacup: 1 person reacted").click();
  const reactors = new Reactors(page);
  await expect(reactors.account(other.displayName)).toBeVisible();

  await fresh.deletion(author, shared);
  // The stream says the post is gone: the list goes back, and so does the post's own page, with nothing to show.
  await expect(reactors.heading).toBeHidden();
  await expect(thread.heading).toBeHidden();
  await expect(page).not.toHaveURL(/\/status\//);
});
