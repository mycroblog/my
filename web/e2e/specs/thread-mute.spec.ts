// SPDX-License-Identifier: AGPL-3.0-only

// A person mutes the thread of their post from its more menu (ADR-0158 §8): a reply in it then brings them no
// notification, while what comes from elsewhere still does; the menu then offers to unmute it.

import { Frame, homeTimeline, Notifications } from "./screens";
import { expect, test } from "./test";

test("after a person mutes the thread of their post, a reply in it brings no notification", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  const replier = await fresh.person();
  const post = await fresh.post(reader, { text: "Which trail should we take on Sunday?" });

  await signIn(reader);
  const mine = homeTimeline(page).post(reader.displayName, post.text);
  await mine.muteThread();
  await mine.more();
  await expect(mine.menu.getByRole("menuitem", { name: "Unmute thread", exact: true })).toBeVisible();
  await page.keyboard.press("Escape");

  await fresh.post(replier, { text: `@${reader.username} the ridge, if it is dry.`, replyTo: post.id });
  // A follow after the reply: once its notification shows, the reply had its chance to notify too.
  const follower = await fresh.person({ follows: [reader] });

  await new Frame(page).goTo("Notifications");
  const notifications = new Notifications(page);
  await expect(notifications.notification(`${follower.displayName} followed you`)).toBeVisible();
  await expect(notifications.feed.getByRole("article", { name: replier.displayName, exact: true })).toHaveCount(0);
});
