// SPDX-License-Identifier: AGPL-3.0-only

// A reader reacts to a post with the emoji picker (FR-116): adds, changes and takes back their one reaction by pointer
// and by keyboard, finds a custom emoji by its alias, and the server keeps each change (a reload shows it). A reaction
// takes the place of the reader's favourite. The list of who reacted with an emoji leaves once the post is deleted.
// The composer's Add emoji button writes the emoji at the caret.

import { Frame, homeTimeline, Reactors, Thread } from "./screens";
import { expect, test } from "./test";

test("a reader adds, changes and takes back a reaction by pointer, and the server keeps each", async ({
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

  // The picker knocks out the reader's own reaction, and picking it again takes it back.
  picker = await post.react();
  await post.pick(picker.emoji("Custom emoji", ":teacup:, your reaction"));
  await expect(picker.dialog).toBeHidden();
  await expect(post.reactions).toBeHidden();
  await page.reload();
  await expect(post.favouriteButton).toBeVisible();
  await expect(post.reactions).toBeHidden();
});

test("a reader adds, changes and takes back a reaction by keyboard, and the server keeps each", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const { text } = await fresh.post(author, { text: "Soup for the whole street tonight." });
  await signIn(await fresh.person({ follows: [author] }));
  const post = homeTimeline(page).post(author.displayName, text);

  // The Tab order: the search, the skin tone, the categories, the grid. A category takes the focus to the first emoji
  // of its section, and the arrows, Home and End move it through the grid.
  let picker = await post.react("Enter");
  await expect(picker.search).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(picker.tone).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(picker.category("Frequently used")).toBeFocused();
  await page.keyboard.press("ArrowRight");
  await expect(picker.category("Custom emoji")).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(picker.emoji("Custom emoji", ":sparkle:")).toBeFocused();
  await page.keyboard.press("ArrowRight");
  await expect(picker.emoji("Custom emoji", ":wave:")).toBeFocused();
  // Down from the second column of a row to a shorter row lands on its last emoji, past the heading "Sunday tea".
  await page.keyboard.press("ArrowDown");
  await expect(picker.emoji("Custom emoji", ":teacup:")).toBeFocused();
  await page.keyboard.press("ArrowUp");
  await expect(picker.emoji("Custom emoji", ":sparkle:")).toBeFocused();
  await page.keyboard.press("End");
  await expect(picker.emoji("Custom emoji", ":wave:")).toBeFocused();
  await page.keyboard.press("Home");
  await expect(picker.emoji("Custom emoji", ":sparkle:")).toBeFocused();
  await page.keyboard.press("ArrowRight");
  await post.reacting(async () => {
    await page.keyboard.press("Enter");
  });
  await expect(picker.dialog).toBeHidden();
  await expect(post.reactButton).toBeFocused();
  await expect(post.reactionChips).toHaveCount(1);
  await expect(post.ownReaction).toHaveAccessibleName(":wave:, 1, including yours");

  // While a search is on, the categories are off: the Tab after the skin tone reaches the results.
  picker = await post.react("Enter");
  await page.keyboard.type("party popper");
  await expect(picker.found).toHaveText("1 emoji found");
  await page.keyboard.press("Tab");
  await expect(picker.tone).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(picker.emoji("Search results", "party popper")).toBeFocused();
  await post.reacting(async () => {
    await page.keyboard.press("Enter");
  });
  await expect(picker.dialog).toBeHidden();
  await expect(post.reactionChips).toHaveCount(1);
  await expect(post.ownReaction).toHaveAccessibleName("🎉, 1, including yours");

  picker = await post.react("Space");
  await page.keyboard.type("party popper");
  await expect(picker.found).toHaveText("1 emoji found");
  await page.keyboard.press("Tab");
  await page.keyboard.press("Tab");
  await expect(picker.emoji("Search results", "party popper, your reaction")).toBeFocused();
  await post.reacting(async () => {
    await page.keyboard.press("Space");
  });
  await expect(picker.dialog).toBeHidden();
  await expect(post.reactions).toBeHidden();
  await page.reload();
  await expect(post.favouriteButton).toBeVisible();
  await expect(post.reactions).toBeHidden();
});

test("a custom emoji is found by its alias, and the reaction with it reaches the server", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const { text } = await fresh.post(author, { text: "Kettle on, who is in?" });
  await signIn(await fresh.person({ follows: [author] }));
  const post = homeTimeline(page).post(author.displayName, text);
  const picker = await post.react();
  await picker.search.fill("ocha");
  await expect(picker.found).toHaveText("1 emoji found");
  await post.pick(picker.emoji("Search results", ":teacup:"));
  await expect(post.ownReaction).toHaveAccessibleName(":teacup:, 1, including yours");
  await page.reload();
  await expect(post.ownReaction).toHaveAccessibleName(":teacup:, 1, including yours");
});

test("a reaction takes the place of the reader's favourite, and a favourite that of their reaction", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const { text } = await fresh.post(author, { text: "The library keeps late hours this week." });
  await signIn(await fresh.person({ follows: [author] }));
  const post = homeTimeline(page).post(author.displayName, text);
  await post.favourite();
  await expect(post.favourited).toHaveAccessibleName("Favourite, 1");

  const picker = await post.react();
  await post.pick(picker.emoji("Smileys & emotion", "grinning face"));
  await expect(post.ownReaction).toHaveAccessibleName("😀, 1, including yours");
  await expect(post.favourited).toBeHidden();
  await expect(post.favouriteButton).toHaveAccessibleName("Favourite");
  await page.reload();
  await expect(post.ownReaction).toHaveAccessibleName("😀, 1, including yours");
  await expect(post.favouriteButton).toHaveAccessibleName("Favourite");
  await expect(post.favourited).toBeHidden();

  await post.favourite();
  await expect(post.favourited).toHaveAccessibleName("Favourite, 1");
  await expect(post.reactions).toBeHidden();
  await page.reload();
  await expect(post.favourited).toHaveAccessibleName("Favourite, 1");
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

test("the composer's Add emoji button writes the emoji at the caret, and gives the text the focus back", async ({
  page,
  signIn,
  fresh,
}) => {
  await signIn(await fresh.person());
  const composer = await new Frame(page).newPost();
  await composer.write("Good morning");

  // A custom emoji gets a space on each side that has none.
  let picker = await composer.emoji();
  await expect(picker.search).toBeFocused();
  await picker.emoji("Custom emoji", ":teacup:").click();
  await expect(picker.dialog).toBeHidden();
  await expect(composer.text).toBeFocused();
  await composer.holds("Good morning :teacup: ");

  // A Unicode emoji goes at the caret as it is, and the caret follows it.
  await composer.text.press("ControlOrMeta+Home");
  for (const _ of "Good") await composer.text.press("ArrowRight");
  picker = await composer.emoji();
  await picker.emoji("Smileys & emotion", "grinning face").click();
  await expect(composer.text).toBeFocused();
  await page.keyboard.type("!");
  await composer.holds("Good😀! morning :teacup: ");

  // With Shift, the picker stays open for the next emoji, which follows the last at the caret.
  picker = await composer.emoji();
  await picker.emoji("Custom emoji", ":wave:").click({ modifiers: ["Shift"] });
  await composer.holds("Good😀! :wave: morning :teacup: ");
  await expect(picker.dialog).toBeVisible();
  await picker.emoji("Smileys & emotion", "grinning face").click();
  await expect(picker.dialog).toBeHidden();
  await composer.holds("Good😀! :wave:😀 morning :teacup: ");
  await expect(composer.text).toBeFocused();

  // An emoji takes the place of the selection.
  await composer.text.press("ControlOrMeta+a");
  picker = await composer.emoji();
  await picker.emoji("Smileys & emotion", "grinning face").click();
  await composer.holds("😀");
});
