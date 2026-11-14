// SPDX-License-Identifier: AGPL-3.0-only

// A follow of a locked account waits for its owner: the owner sees the request and accepts it, and the follower then
// follows them and reads their followers-only post.

import { Frame, homeTimeline, Notifications, signInAs } from "./screens";
import { expect, test } from "./test";

test("a locked person accepts a pending follow request, and the follower then reads their posts", async ({
  page,
  signIn,
  browser,
  fresh,
}) => {
  const owner = await fresh.person({ locked: true });
  const follower = await fresh.person();
  const post = await fresh.post(owner, {
    text: "Followers only: we meet upstairs at the café.",
    visibility: "private",
  });
  await fresh.followRequest(follower, owner);

  await signIn(owner);
  await new Frame(page).goTo("Notifications");
  const notifications = new Notifications(page);
  await expect(notifications.acceptButton(follower.displayName)).toBeVisible();
  await notifications.accept(follower.displayName);
  await expect(notifications.acceptButton(follower.displayName)).toBeHidden();
  await page.reload();
  await expect(notifications.followRequest(follower.displayName)).toBeVisible();
  await expect(notifications.acceptButton(follower.displayName)).toBeHidden();

  const followerPage = await browser.newPage();
  try {
    await signInAs(followerPage, follower);
    await expect(homeTimeline(followerPage).post(owner.displayName, post.text).article).toBeVisible();
    await new Frame(followerPage).goTo("Notifications");
    await expect(
      new Notifications(followerPage).notification(`${owner.displayName} accepted your follow request`),
    ).toBeVisible();
  } finally {
    await followerPage.close();
  }
});
