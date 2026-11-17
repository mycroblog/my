// SPDX-License-Identifier: AGPL-3.0-only

// A person deletes, redrafts and edits their own post from its more menu, reads its edit history, and copies its link;
// the menu offers each reader what they may do to a post, the thread's mute to one with an account.

import { homeTimeline, Landing, localTimeline, Notice, SignIn, Thread } from "./screens";
import { expect, test } from "./test";

test("a deleted post leaves the timeline at once, and stays gone after a reload", async ({ page, signIn, fresh }) => {
  const writer = await fresh.person();
  const kept = await fresh.post(writer, { text: "A post to keep." });
  const { text } = await fresh.post(writer, { text: "A post to take back." });
  await signIn(writer);
  const timeline = homeTimeline(page);
  const post = timeline.post(writer.displayName, text);
  await expect(post.article).toBeVisible();
  await post.delete();
  await expect(post.article).toBeHidden();
  await expect(new Notice(page).text("Post deleted")).toBeVisible();
  await page.reload();
  // The post kept shows once the timeline has loaded again, and the deleted one with it would show by then.
  await expect(timeline.post(writer.displayName, kept.text).article).toBeVisible();
  await expect(post.article).toBeHidden();
});

test("an edit shows its new text and the edited mark, and the history lists both versions", async ({
  page,
  signIn,
  fresh,
}) => {
  const writer = await fresh.person();
  const { text } = await fresh.post(writer, { text: "Bread at nine." });
  const changed = `${text} Make it ten.`;
  await signIn(writer);
  const timeline = homeTimeline(page);
  const before = timeline.post(writer.displayName, text);
  await expect(before.edited).toBeHidden();
  const composer = await before.edit();
  await composer.holds(text);
  await composer.replace(changed);
  await composer.save();
  await expect(new Notice(page).text("Edit saved")).toBeVisible();
  const after = timeline.post(writer.displayName, changed);
  await expect(after.paragraph(changed)).toBeVisible();
  await expect(after.edited).toBeVisible();
  const versions = await after.history();
  // Newest first: the current text, then the original without the words the edit added.
  await expect(versions).toHaveText([/^Current.*Make it ten\./, /^Original(?:(?!Make it ten).)*$/]);
  await page.reload();
  await expect(timeline.post(writer.displayName, changed).edited).toBeVisible();
});

test("delete and redraft takes the post away and opens it as a new post", async ({ page, signIn, fresh }) => {
  const writer = await fresh.person();
  const { text } = await fresh.post(writer, { text: "A post to write again." });
  await signIn(writer);
  const post = homeTimeline(page).post(writer.displayName, text);
  const composer = await post.redraft();
  await expect(composer.dialog).toBeVisible();
  await composer.holds(text);
  await expect(post.article).toBeHidden();
});

test("Copy link copies the post's address and says so", async ({ page, context, signIn, fresh }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  const writer = await fresh.person();
  const { id, text } = await fresh.post(writer, { text: "A post to hand on." });
  await signIn(writer);
  await homeTimeline(page).post(writer.displayName, text).menuItem("Copy link");
  await expect(new Notice(page).text("Link copied")).toBeVisible();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toMatch(new RegExp(`/${id}$`));
});

test.describe("the more menu", () => {
  test("of one's own post: its link, sharing, the bookmark, the thread's mute, edit, redraft and delete", async ({
    page,
    signIn,
    fresh,
  }) => {
    const writer = await fresh.person();
    const { text } = await fresh.post(writer, { text: "A post of one's own." });
    await signIn(writer);
    const items = await homeTimeline(page).post(writer.displayName, text).more();
    await expect(items).toHaveText([
      "Copy link",
      "Share",
      "Bookmark",
      "Mute thread",
      "Edit",
      "Delete and redraft…",
      "Delete…",
    ]);
  });

  test("of one's own post with a poll: no edit, which the server refuses", async ({ page, signIn, fresh }) => {
    const writer = await fresh.person();
    const { text } = await fresh.post(writer, {
      text: "Rye or spelt?",
      poll: { options: ["Rye", "Spelt"], days: 1 },
    });
    await signIn(writer);
    const items = await homeTimeline(page).post(writer.displayName, text).more();
    await expect(items).toHaveText(["Copy link", "Share", "Bookmark", "Mute thread", "Delete and redraft…", "Delete…"]);
  });

  test("of another's post: its link, sharing, the bookmark and the thread's mute, then muting and blocking its author", async ({
    page,
    signIn,
    fresh,
  }) => {
    // An author of this server: no server of its own to mute.
    const author = await fresh.person();
    const { text } = await fresh.post(author, { text: "A post of someone else's." });
    await signIn(await fresh.person({ follows: [author] }));
    const items = await homeTimeline(page).post(author.displayName, text).more();
    await expect(items).toHaveText(["Copy link", "Share", "Bookmark", "Mute thread", "Mute", "Block…"]);
  });

  test("of a visitor: the link and sharing work, and the bookmark leads to the sign-in", async ({ page, fresh }) => {
    // A post of its own, the newest of the local timeline: the posts the other tests write would push a seeded one
    // off the first page.
    const writer = await fresh.person();
    const { text } = await fresh.post(writer, { text: "A post a visitor reads." });
    const landing = new Landing(page);
    await landing.open();
    await landing.place("Read the local timeline").click();
    const post = localTimeline(page).post(writer.displayName, text);
    const items = await post.more();
    await expect(items).toHaveText(["Copy link", "Share", "Bookmark"]);
    await post.menu.getByRole("menuitem", { name: "Bookmark", exact: true }).click();
    await expect(new SignIn(page).heading).toBeVisible();
  });
});

test("deleting the post open on its own page leaves the page without it", async ({ page, signIn, fresh }) => {
  const writer = await fresh.person();
  const { text } = await fresh.post(writer, { text: "A post deleted from its own page." });
  await signIn(writer);
  await homeTimeline(page).post(writer.displayName, text).open();
  const thread = new Thread(page);
  await expect(thread.heading).toBeVisible();
  const post = thread.post(writer.displayName, text);
  await post.delete();
  await expect(new Notice(page).text("Post deleted")).toBeVisible();
  await expect(post.article).toBeHidden();
  // The reader deleted it: the page does not say it failed to load, nor offer to load it again.
  await expect(thread.error).toBeHidden();
});
