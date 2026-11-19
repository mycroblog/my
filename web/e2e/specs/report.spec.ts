// SPDX-License-Identifier: AGPL-3.0-only

// A person reports an account from one of its posts (FR-701): Report… of the post's menu opens the report with that
// post chosen; they add an older post of the account, say why and write to the moderators, and send it; the panel
// says it was sent and offers to mute the account, which leaves its line in the post's place once the panel closes.

import { homeTimeline, ReportPanel } from "./screens";
import { expect, test } from "./test";

test("a person reports an account from its post, with another of its posts, and mutes it from the report", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const reader = await fresh.person({ follows: [author] });
  const older = await fresh.post(author, { text: "Cheap followers, link in my profile." });
  const post = await fresh.post(author, { text: "Cheap followers again, link in my profile!" });

  await signIn(reader);
  const shown = homeTimeline(page).post(author.displayName, post.text);
  const report = await ReportPanel.fromPost(shown);
  await expect(report.post(post.text)).toBeChecked();
  await expect(report.post(older.text)).not.toBeChecked();
  await report.include(older.text);
  await report.choose("Spam");
  await report.comment.fill("The same advertisement under every post of mine.");
  await report.submit();

  await expect(report.sent).toBeVisible();
  await report.after("Mute");
  await report.done();
  await expect(page.getByText(new RegExp(`^You muted @${author.username}(?:@\\S+)?\\.$`))).toBeVisible();
});
