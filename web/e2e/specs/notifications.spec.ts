// SPDX-License-Identifier: AGPL-3.0-only

// The notifications screen shows what the scenario's people did to a person: a follow, a reply that mentions them, a
// favourite and a boost of their post.

import { Frame, Notifications } from "./screens";
import { expect, person, test } from "./test";

test("a person's notifications show the follows, replies, favourites and boosts of others", async ({
  page,
  signIn,
}) => {
  const yuki = person("yuki").displayName;
  const hana = person("hana").displayName;
  await signIn(person("sam"));
  await new Frame(page).goTo("Notifications");
  const notifications = new Notifications(page);
  await expect(notifications.notification(`${yuki} followed you`)).toBeVisible();
  await expect(notifications.mention(yuki, "北側の尾根道です")).toBeVisible();
  await expect(notifications.notification(`${yuki} favourited your post`)).toContainText("Day 3 of the sourdough");
  await expect(notifications.notification(`${hana} boosted your post`)).toContainText("Day 3 of the sourdough");
});
