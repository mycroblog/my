// SPDX-License-Identifier: AGPL-3.0-only

// The demo server serves the web client, and a person of the scenario signs in and reads their home timeline, with
// the posts of the people they follow.

import { homeTimeline } from "./screens";
import { expect, person, test } from "./test";

test("a person of the scenario signs in and sees the home timeline", async ({ page, signIn }) => {
  await signIn(person("yuki"));
  await expect(homeTimeline(page).post(person("layla").displayName, "صباح الخير").article).toBeVisible();
});
