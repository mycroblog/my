// SPDX-License-Identifier: AGPL-3.0-only

// A follow of a locked account waits for its owner: ren sees sam's request and accepts it, and sam then follows ren
// and reads ren's followers-only post.

import { Frame, homeTimeline, Notifications, signInAs } from "./screens";
import { expect, person, test } from "./test";

test("a locked person accepts a pending follow request, and the follower then reads their posts", async ({
  page,
  signIn,
  browser,
}) => {
  const ren = person("ren");
  const sam = person("sam");
  await signIn(ren);
  await new Frame(page).goTo("Notifications");
  const notifications = new Notifications(page);
  await expect(notifications.acceptButton(sam.displayName)).toBeVisible();
  await notifications.accept(sam.displayName);
  await expect(notifications.acceptButton(sam.displayName)).toBeHidden();
  await page.reload();
  await expect(notifications.followRequest(sam.displayName)).toBeVisible();
  await expect(notifications.acceptButton(sam.displayName)).toBeHidden();

  const samPage = await browser.newPage();
  try {
    await signInAs(samPage, sam);
    await expect(homeTimeline(samPage).post(ren.displayName, "フォロワー限定で").article).toBeVisible();
    await new Frame(samPage).goTo("Notifications");
    await expect(
      new Notifications(samPage).notification(`${ren.displayName} accepted your follow request`),
    ).toBeVisible();
  } finally {
    await samPage.close();
  }
});
