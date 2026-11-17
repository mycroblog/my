// SPDX-License-Identifier: AGPL-3.0-only

// A person deletes and edits their own post from its more menu, and both are kept by the server: a reload shows the
// timeline without the one and with the other's new text and history.

import { homeTimeline, Notice } from "./screens";
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
