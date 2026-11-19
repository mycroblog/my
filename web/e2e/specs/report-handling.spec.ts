// SPDX-License-Identifier: AGPL-3.0-only

// A report from its filing to its resolution, on the real server (#3259, FR-701, FR-709–FR-712): a person reports an
// account from one of its posts; a moderator finds the open report through Moderation in the navigation and its list
// of screens, opens it from the queue, takes it, writes a note, and resolves it with no action needed; the person who
// filed it is then told that it was resolved, and nothing of how. The moderator reopens it, and it waits in the queue
// again. What each screen shows in every state is the stories' and the route tests' on the fake (web/app,
// moderation.stories.tsx).

import { randomUUID } from "node:crypto";
import type { Page } from "@playwright/test";
import { Frame, homeTimeline, Moderation, Notifications, ReportPanel, SignIn } from "./screens";
import { expect, test, type FreshPerson } from "./test";

/** Signs in as `p` on `page`, a browser of its own, and waits for the home screen. */
async function signInOn(page: Page, p: FreshPerson): Promise<void> {
  const screen = new SignIn(page);
  await screen.open();
  await screen.withPassword(p.email, p.password);
  await expect(new Frame(page).heading("Home")).toBeVisible();
}

test("a person reports an account, a moderator takes the report, writes a note and resolves it, and the person is told it was resolved", async ({
  page,
  browser,
  baseURL,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const reporter = await fresh.person({ follows: [author] });
  const moderator = await fresh.person({ admin: true });
  const post = await fresh.post(author, { text: "Cheap followers, link in my profile." });
  const handle = `@${author.username}`;

  // The person files the report from the post's menu.
  await signIn(reporter);
  const report = await ReportPanel.fromPost(homeTimeline(page).post(author.displayName, post.text));
  await report.choose("Spam");
  await report.comment.fill("The same advertisement under every post of mine.");
  await report.submit();
  await expect(report.sent).toBeVisible();
  await report.done();

  // The moderator, in a browser of their own, finds it under Moderation.
  const theirs = await browser.newContext({
    ...(baseURL === undefined ? {} : { baseURL }),
    extraHTTPHeaders: { "X-Forwarded-For": "198.19.251.1" },
  });
  try {
    const staff = await theirs.newPage();
    await signInOn(staff, moderator);
    const moderation = new Moderation(staff);
    await moderation.place.click();
    await moderation.reportsLink.click();
    await expect(moderation.row(handle)).toBeVisible();
    await moderation.open(handle);
    await expect(moderation.reportHeading(handle)).toBeVisible();
    await expect(moderation.facts).toContainText("Spam");
    await expect(staff.getByRole("main").getByText(post.text)).toBeVisible();

    await moderation.assignToMe();
    await expect(moderation.said("You handle this report now.")).toBeVisible();
    const note = `Checked the profile ${randomUUID().slice(0, 8)}`;
    await moderation.addNote(note);
    await expect(moderation.note(note)).toBeVisible();
    await moderation.resolve("No action needed");
    await expect(moderation.said("The report is resolved.")).toBeVisible();
    await expect(moderation.facts).toContainText("Resolved, no action needed");

    // Reopened, it waits in the queue of open reports again.
    await moderation.reopen();
    await expect(moderation.said("The report is open again.")).toBeVisible();
    await moderation.back();
    await expect(moderation.row(handle)).toBeVisible();
  } finally {
    await theirs.close();
  }

  // The person who filed it is told it was resolved, once, and nothing of how.
  await page.goto("/notifications");
  const notifications = new Notifications(page);
  await expect(
    notifications.feed.getByRole("article", { name: /^A report you filed on .+ has been resolved$/ }),
  ).toHaveCount(1);
});
