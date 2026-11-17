// SPDX-License-Identifier: AGPL-3.0-only

// A writer keeps a post as a draft on the server and schedules one from the composer (ADR-0156): the draft reopens from
// the list and is posted, a scheduled post's time is set, changed and taken back, and a draft and a scheduled post are
// deleted. That the server publishes a scheduled post at its time is the server's tests'
// (TestScheduledDraftBeyondTheQueueDelayLimit).

import { Frame, homeTimeline } from "./screens";
import { expect, test } from "./test";

/** What the list says of a writer with neither a draft nor a scheduled post. */
const nothing = "No drafts or scheduled posts yet. Save a post as a draft or schedule it, and it shows here.";

test("a writer saves a draft, reopens it from the list and posts it", async ({ page, signIn, fresh }) => {
  const writer = await fresh.person();
  const text = "Draft check: the bakery on the corner reopens next week.";
  await signIn(writer);
  const frame = new Frame(page);
  const composer = await frame.newPost();
  await composer.write(text);
  await composer.saveAsDraft();
  // The post is no longer being written: the composer opens empty, and the draft is in the list.
  const next = await frame.newPost();
  await next.holds("");
  const drafts = await next.openDrafts();
  await expect(drafts.heading("Drafts 1")).toBeVisible();
  await expect(drafts.opener(text)).toBeVisible();
  const reopened = await drafts.open(text);
  await reopened.holds(text);
  await reopened.postDraft();
  await expect(homeTimeline(page).post(writer.displayName, text).article).toBeVisible();
  // Posted, it leaves the list.
  const after = await (await frame.newPost()).openDrafts();
  await expect(after.dialog.getByText(nothing, { exact: true })).toBeVisible();
  await after.close();
  await page.reload();
  await expect(homeTimeline(page).post(writer.displayName, text).article).toBeVisible();
});

test("a writer schedules a post, changes its time from the list and returns it to the drafts, and deletes it and a scheduled post", async ({
  page,
  signIn,
  fresh,
}) => {
  const writer = await fresh.person();
  const reminder = await fresh.draft(writer, { text: "Delete check: a reminder for next month.", inSeconds: 86_400 });
  const text = "Schedule check: the reading group meets at the library.";
  await signIn(writer);
  const frame = new Frame(page);
  // A scheduled post is deleted after confirming, which leaves the writer nothing.
  const first = await (await frame.newPost()).openDrafts();
  await expect(first.heading("Scheduled 1")).toBeVisible();
  await first.delete(reminder.text, first.scheduled);
  await expect(first.dialog.getByText(nothing, { exact: true })).toBeVisible();
  await first.close();

  const composer = await frame.newPost();
  await composer.write(text);
  await composer.setTime();
  // A time set: the tool is on, the line over the row says when, and Schedule takes Post's place. The form starts at a
  // whole hour.
  await expect(composer.scheduleTool("Change the scheduled time")).toHaveAttribute("aria-pressed", "true");
  await expect(composer.timeLine).toHaveText(/^Scheduled for .+:00\s[AP]M \(.+\)\s*Change time$/);
  await expect(composer.dialog.getByRole("button", { name: "Post", exact: true })).toBeHidden();
  await composer.schedule();

  const drafts = await (await frame.newPost()).openDrafts();
  await expect(drafts.heading("Scheduled 1")).toBeVisible();
  await expect(drafts.dialog.getByText(/^Times in .+$/)).toBeVisible();
  await expect(drafts.opener(text, drafts.scheduled)).toHaveText(/:00\s[AP]M/);
  await expect(drafts.drafts.getByRole("listitem")).toHaveCount(0);

  // Change time… opens it with its time's form; a minute later is its new time.
  const changing = await drafts.changeTime(text);
  await changing.holds(text);
  await changing.nextMinute();
  await expect(changing.timeLine).toHaveText(/^Scheduled for .+:01\s[AP]M \(.+\)\s*Change time$/);
  await changing.schedule();

  const moved = await (await frame.newPost()).openDrafts();
  await expect(moved.opener(text, moved.scheduled)).toHaveText(/:01\s[AP]M/);
  // Returned to the drafts, it is a draft again, and nothing is scheduled.
  await moved.unschedule(text);
  await expect(moved.scheduled).toBeHidden();
  await expect(moved.heading("Drafts 1")).toBeVisible();
  await expect(moved.opener(text)).toBeVisible();
  const draft = await moved.open(text);
  await expect(draft.timeLine).toBeHidden();
  await expect(draft.scheduleTool("Schedule")).toHaveAttribute("aria-pressed", "false");
  await expect(draft.dialog.getByRole("button", { name: "Post", exact: true })).toBeVisible();
  await draft.close();
  // Closed without a word, the draft stays on the server as it was saved.
  const kept = await (await frame.newPost()).openDrafts();
  await expect(kept.opener(text)).toBeVisible();
  // Deleted after confirming, it is gone, and so is the scheduled post deleted first: the list read again holds
  // neither.
  await kept.delete(text);
  await expect(kept.dialog.getByText(nothing, { exact: true })).toBeVisible();
  await kept.close();
  await page.reload();
  const again = await (await frame.newPost()).openDrafts();
  await expect(again.dialog.getByText(nothing, { exact: true })).toBeVisible();
});
