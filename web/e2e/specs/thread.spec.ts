// SPDX-License-Identifier: AGPL-3.0-only

// A person opens a post from the home timeline and replies in its thread.

import { homeTimeline, Thread } from "./screens";
import { expect, test } from "./test";

test("a person replies in a thread and the reply stays there after a reload", async ({ page, signIn, fresh }) => {
  const author = await fresh.person();
  const post = await fresh.post(author, { text: "Day 3 of the rye starter." });
  const replier = await fresh.person({ follows: [author] });
  const text = "Thread check: that crumb looks right to me.";
  await signIn(replier);
  await homeTimeline(page).post(author.displayName, post.text).open();
  const thread = new Thread(page);
  const composer = await thread.post(author.displayName, post.text).reply();
  await composer.holds(new RegExp(`^@${author.username}\\b`));
  await composer.write(text);
  await composer.post();
  await expect(thread.post(replier.displayName, text).article).toBeVisible();
  await page.reload();
  await expect(thread.post(replier.displayName, text).paragraph(text)).toContainText(`@${author.username}`);
});
