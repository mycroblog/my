// SPDX-License-Identifier: AGPL-3.0-only

// An author approves replies and boosts by hand (ADR-0155): a reply of someone the post does not admit at once says
// it waits, before and after it is sent, and is seen by its author alone until the post's author approves it from the
// screen of what waits, after which it shows under the post without a reload; a boost says it waits until it is
// approved, and then counts.

import { Frame, localTimeline, Notifications, signInAs, Thread } from "./screens";
import { expect, test } from "./test";

test("a reply waits for the author's approval, seen by its writer alone, and joins the thread once approved", async ({
  page,
  browser,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const replier = await fresh.person();
  const other = await fresh.person();
  const post = await fresh.post(author, {
    text: "Approval check: replies from my followers show at once.",
    policy: { reply: { automatic: "followers", manual: "everyone" } },
  });
  const text = "Approval check: may I reply to this?";
  // A post of their own each, which their home shows once they sign in.
  await fresh.post(replier, { text: "Approval check: the replier's own post." });
  await fresh.post(other, { text: "Approval check: someone else's own post." });

  // The author opens the post first, so that its thread is on the screen they come back to.
  await signIn(author);
  await page.goto("/local");
  await localTimeline(page).post(author.displayName, post.text).open();
  const authorThread = new Thread(page);
  await expect(authorThread.heading).toBeVisible();

  const replierPage = await browser.newPage();
  const otherPage = await browser.newPage();
  try {
    await signInAs(replierPage, replier);
    await replierPage.goto("/local");
    await localTimeline(replierPage).post(author.displayName, post.text).open();
    const thread = new Thread(replierPage);
    const composer = await thread.post(author.displayName, post.text).reply();
    await expect(composer.replyWaits(`@${author.username}`)).toBeVisible();
    await composer.write(text);
    await composer.post();
    const reply = thread.post(replier.displayName, text);
    await expect(reply.approvalLine("Waiting for approval")).toBeVisible();
    await replierPage.reload();
    await expect(reply.approvalLine("Waiting for approval")).toBeVisible();

    // Neither another person nor the author sees the reply in the thread while it waits.
    await signInAs(otherPage, other);
    await otherPage.goto("/local");
    await localTimeline(otherPage).post(author.displayName, post.text).open();
    const otherThread = new Thread(otherPage);
    await expect(otherThread.post(author.displayName, post.text).article).toBeVisible();
    await expect(otherThread.post(replier.displayName, text).article).toBeHidden();
    await expect(authorThread.post(replier.displayName, text).article).toBeHidden();

    // The author approves it from the screen of what waits, and goes back to the thread, which shows it.
    await new Frame(page).goTo("Notifications");
    const waiting = await new Notifications(page).openWaiting(1);
    const request = waiting.request(replier.displayName, "reply to");
    await waiting.answer(request, "Approve");
    await expect(request.getByRole("button")).toHaveCount(0);
    await page.goBack();
    await page.goBack();
    await expect(authorThread.post(replier.displayName, text).article).toBeVisible();

    await replierPage.reload();
    await expect(reply.article).toBeVisible();
    await expect(reply.approvalLine("Waiting for approval")).toBeHidden();
    await otherPage.reload();
    await expect(otherThread.post(replier.displayName, text).article).toBeVisible();
  } finally {
    await replierPage.close();
    await otherPage.close();
  }
});

test("a boost waits for the author's approval, and counts once approved", async ({ page, browser, signIn, fresh }) => {
  const author = await fresh.person();
  const booster = await fresh.person();
  const post = await fresh.post(author, {
    text: "Approval check: ask me before you boost this.",
    policy: { announce: { automatic: "nobody", manual: "everyone" } },
  });

  await signIn(booster);
  await page.goto("/local");
  await localTimeline(page).post(author.displayName, post.text).open();
  const thread = new Thread(page);
  const boosted = thread.post(author.displayName, post.text);
  await boosted.boost();
  await expect(boosted.approvalLine("Boost waiting for approval")).toBeVisible();
  await page.reload();
  await expect(boosted.approvalLine("Boost waiting for approval")).toBeVisible();
  await expect(boosted.boosted).toHaveCount(0);

  const authorPage = await browser.newPage();
  try {
    await signInAs(authorPage, author);
    await new Frame(authorPage).goTo("Notifications");
    const waiting = await new Notifications(authorPage).openWaiting(1);
    const request = waiting.request(booster.displayName, "boost");
    await waiting.answer(request, "Approve");
    await expect(request.getByRole("button")).toHaveCount(0);
  } finally {
    await authorPage.close();
  }

  await page.reload();
  await expect(boosted.approvalLine("Boost waiting for approval")).toBeHidden();
  await expect(boosted.boosted).toHaveAccessibleName("Boost, 1");
});
