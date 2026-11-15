// SPDX-License-Identifier: AGPL-3.0-only

// Below the width where the column beside the main one shows them, one line above the home timeline counts the
// server's unread announcements and leads to their list; one the reader opens there is read, and leaves the count.

import { Frame, HomeAnnouncements } from "./screens";
import { announcements, expect, test } from "./test";

// A tablet in landscape: the navigation's rail, and no column beside the main one.
test.use({ viewport: { width: 1024, height: 768 } });

test("a line above the home timeline counts the unread announcements and leads to the list, where one opened is read", async ({
  page,
  signIn,
  fresh,
}) => {
  // A fresh person, who has read none, and whose read the other runs do not see.
  await signIn(await fresh.person());
  const home = new HomeAnnouncements(page);
  await expect(home.unread(announcements.length)).toBeVisible();

  const [opened] = announcements;
  if (opened === undefined) throw new Error("the scenario has no announcement");
  const list = await home.openList();
  const frame = new Frame(page);
  await expect(frame.heading("Announcements")).toBeVisible();
  await expect(list.unread(opened.title)).toBeVisible();
  await list.open(opened.title);
  await expect(frame.heading(opened.title)).toBeVisible();

  await frame.goTo("Home");
  await expect(home.unread(announcements.length - 1)).toBeVisible();
});
