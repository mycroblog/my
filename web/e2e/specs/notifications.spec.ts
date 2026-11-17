// SPDX-License-Identifier: AGPL-3.0-only

// The notifications screen: a fresh person opens what a follow, a favourite and a mention tell of, and a notification
// they dismiss stays gone; the link of one notification, as an email or a push gives it, leads to what it tells of.

import { Frame, Notifications } from "./screens";
import { expect, test } from "./test";

test("a person opens a follow, a favourite and a mention from their notifications, and dismisses one for good", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  const follower = await fresh.person({ follows: [reader] });
  const fan = await fresh.person();
  const writer = await fresh.person();
  const post = await fresh.post(reader, { text: "The first loaf from the new oven." });
  await fresh.favourite(fan, post);
  const mention = await fresh.post(writer, { text: `@${reader.username} how long did it proof?` });

  await signIn(reader);
  const frame = new Frame(page);
  const notifications = new Notifications(page);
  await frame.goTo("Notifications");
  await notifications.open(`${follower.displayName} followed you`);
  await expect(page).toHaveURL(new RegExp(`/@${follower.username}$`));

  await frame.goTo("Notifications");
  await notifications.open(`${fan.displayName} favourited your post`);
  await expect(page).toHaveURL(new RegExp(`/@${reader.username}/status/${post.id}$`));

  await frame.goTo("Notifications");
  await notifications.openMention(writer.displayName, mention.text);
  await expect(page).toHaveURL(new RegExp(`/@${writer.username}/status/${mention.id}$`));

  await frame.goTo("Notifications");
  await notifications.dismiss(`${follower.displayName} followed you`);
  await expect(notifications.notification(`${follower.displayName} followed you`)).toBeHidden();
  await page.reload();
  await expect(notifications.notification(`${fan.displayName} favourited your post`)).toBeVisible();
  await expect(notifications.notification(`${follower.displayName} followed you`)).toBeHidden();
});

test("the link of one notification, as an email or a push gives it, leads to what it tells of, and says so of one gone", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  const fan = await fresh.person();
  const post = await fresh.post(reader, { text: "Rye loaves on the cooling rack." });
  await fresh.favourite(fan, post);
  const id = await fresh.notificationOf(reader, "favourite");

  await signIn(reader);
  await page.goto(`/notifications/${id}`);
  await expect(page).toHaveURL(new RegExp(`/@${reader.username}/status/${post.id}$`));

  const notifications = new Notifications(page);
  await page.goto("/notifications/0190a5d0-0000-7000-8000-000000000999");
  await expect(notifications.gone).toBeVisible();
});
