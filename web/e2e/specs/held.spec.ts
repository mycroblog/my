// SPDX-License-Identifier: AGPL-3.0-only

// The notifications a person's notification policy holds back (ADR-0158 §7): with those of people they do not follow
// held back, a stranger's favourite waits under the row over the notifications, on the screen of the held-back
// notifications and on the stranger's own; accepting moves it into the notifications, and the row goes.

import { Frame, Notifications } from "./screens";
import { expect, test } from "./test";

test("a person accepts the held-back notifications of a stranger, which then join their notifications", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  const stranger = await fresh.person();
  await fresh.notificationPolicy(reader, { not_following: "filter" });
  const post = await fresh.post(reader, { text: "Notes from the morning market." });
  await fresh.favourite(stranger, post);

  await signIn(reader);
  await new Frame(page).goTo("Notifications");
  const notifications = new Notifications(page);
  const favourited = `${stranger.displayName} favourited your post`;
  const held = await notifications.openHeld(1, 1);
  const theirs = await held.open(stranger.displayName);
  await expect(theirs.notification(favourited)).toBeVisible();
  await theirs.answer("Accept");
  await expect(theirs.said("Accepted")).toBeFocused();

  await new Frame(page).goTo("Notifications");
  await expect(notifications.notification(favourited)).toBeVisible();
  await expect(notifications.heldRow(1, 1)).toBeHidden();
});
