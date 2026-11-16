// SPDX-License-Identifier: AGPL-3.0-only

// A writer keeps a post as a draft on the server and schedules one from the composer (ADR-0156): the draft reopens from
// the list and is posted, a scheduled post's time is set, changed and taken back, a draft is deleted, a save from an
// older copy is refused for the newer one, a scheduled post is published at its time, and one that failed says why.

import { Drafts, Frame, homeTimeline } from "./screens";
import { expect, person, test } from "./test";

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
  await expect(
    after.dialog.getByText(
      "No drafts or scheduled posts yet. Save a post as a draft or schedule it, and it shows here.",
      { exact: true },
    ),
  ).toBeVisible();
  await after.close();
  await page.reload();
  await expect(homeTimeline(page).post(writer.displayName, text).article).toBeVisible();
});

test("the list goes back to the post being written, and once closed, New post opens the composer", async ({
  page,
  signIn,
  fresh,
}) => {
  const writer = await fresh.person();
  const text = "Back check: a post still being written.";
  await signIn(writer);
  const frame = new Frame(page);
  const composer = await frame.newPost();
  await composer.write(text);
  const drafts = await composer.openDrafts();
  await drafts.dialog.getByRole("button", { name: "Back to the post", exact: true }).click();
  await composer.holds(text);
  await composer.replace("");
  await (await composer.openDrafts()).close();
  const next = await frame.newPost();
  await expect(next.text).toBeVisible();
  await next.holds("");
});

test("a writer schedules a post, changes its time from the list, and returns it to the drafts", async ({
  page,
  signIn,
  fresh,
}) => {
  const writer = await fresh.person();
  const text = "Schedule check: the reading group meets at the library.";
  await signIn(writer);
  const frame = new Frame(page);
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
});

test("a writer deletes a draft and a scheduled post after confirming", async ({ page, signIn, fresh }) => {
  const writer = await fresh.person();
  const draft = await fresh.draft(writer, { text: "Delete check: a note about the ferry times." });
  const scheduled = await fresh.draft(writer, { text: "Delete check: a reminder for next month.", inSeconds: 86_400 });
  await signIn(writer);
  const frame = new Frame(page);
  const drafts = await (await frame.newPost()).openDrafts();
  await expect(drafts.heading("Scheduled 1")).toBeVisible();
  await expect(drafts.heading("Drafts 1")).toBeVisible();
  await drafts.delete(draft.text);
  await expect(drafts.opener(draft.text)).toBeHidden();
  await expect(drafts.heading("Drafts 0")).toBeVisible();
  await drafts.delete(scheduled.text, drafts.scheduled);
  await expect(
    drafts.dialog.getByText(
      "No drafts or scheduled posts yet. Save a post as a draft or schedule it, and it shows here.",
      { exact: true },
    ),
  ).toBeVisible();
  await drafts.close();
  await page.reload();
  const again = await (await frame.newPost()).openDrafts();
  await expect(
    again.dialog.getByText(
      "No drafts or scheduled posts yet. Save a post as a draft or schedule it, and it shows here.",
      { exact: true },
    ),
  ).toBeVisible();
});

test("a save from an older copy of a draft is refused, and the composer opens the newer one", async ({
  page,
  context,
  signIn,
  fresh,
}) => {
  const writer = await fresh.person();
  const draft = await fresh.draft(writer, { text: "Conflict check: first version." });
  await signIn(writer);
  // This page opens the draft first.
  const older = await (await new Frame(page).newPost()).openDrafts();
  const stale = await older.open(draft.text);
  await stale.holds(draft.text);

  // Another page of the same writer opens it too, and saves a newer version.
  const other = await context.newPage();
  await other.goto("/");
  const otherFrame = new Frame(other);
  const newer = await (await otherFrame.newPost()).openDrafts();
  const fresher = await newer.open(draft.text);
  const newerText = `${draft.text} Second version from another device.`;
  await fresher.replace(newerText);
  await fresher.saveAsDraft();

  // The first page's save is refused: it says so, and opens the newer one.
  await stale.replace(`${draft.text} Third version, written on the older copy.`);
  await stale.saveAsDraftRefused();
  await expect(stale.alert).toHaveText(
    "This draft was saved later on another device, so your changes here were not saved. Open the newer one",
  );
  await stale.openNewer();
  await stale.holds(newerText);
  await expect(stale.alert).toBeHidden();
  await stale.close();
  // One draft, the newer one: the refused save made no copy.
  const list = await (await new Frame(page).newPost()).openDrafts();
  await expect(list.heading("Drafts 1")).toBeVisible();
  await expect(list.opener(newerText)).toBeVisible();
});

test("a scheduled post is published at its time and leaves the list", async ({ page, signIn, fresh }) => {
  // The nearest time the demo server takes is a minute away (scenario.ts), past the default test timeout.
  test.slow();
  const writer = await fresh.person();
  const scheduled = await fresh.draft(writer, { text: "Publish check: the market opens early today.", inSeconds: 65 });
  await signIn(writer);
  const frame = new Frame(page);
  const drafts = await (await frame.newPost()).openDrafts();
  await expect(drafts.opener(scheduled.text, drafts.scheduled)).toBeVisible();
  await drafts.close();
  // The home timeline is read again until the server has published it.
  await expect(async () => {
    await page.reload();
    await expect(homeTimeline(page).post(writer.displayName, scheduled.text).article).toBeVisible();
  }).toPass();
  const after = await (await frame.newPost()).openDrafts();
  await expect(after.opener(scheduled.text, after.scheduled)).toBeHidden();
  await expect(after.opener(scheduled.text)).toBeHidden();
});

test("a scheduled post that failed says why on its draft and in the composer", async ({ page, signIn }) => {
  // The seed's scheduled reply fails about a minute after the seed (its post is deleted), past the default timeout.
  test.slow();
  const text = "Booked two seats. See you at the pier!";
  const failure = /^Scheduled for .+, not posted: the post it replies to is no longer available$/;
  await signIn(person("yuki"));
  const frame = new Frame(page);
  // The page is loaded and the list read again until the server has tried to publish it.
  const shown: { drafts?: Drafts } = {};
  await expect(async () => {
    await page.reload();
    shown.drafts = await (await frame.newPost()).openDrafts();
    await expect(shown.drafts.row(text).getByText(failure)).toBeVisible();
  }).toPass();
  const drafts = shown.drafts ?? new Drafts(page);
  const composer = await drafts.open(text, drafts.drafts, "Reply");
  await expect(composer.alert).toHaveText(
    /^Scheduled for .+, not posted: the post it replies to is no longer available\. Edit it if needed, then post it or schedule it again\.$/,
  );
  await composer.close();
});
