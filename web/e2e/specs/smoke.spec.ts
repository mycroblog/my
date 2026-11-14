// SPDX-License-Identifier: AGPL-3.0-only

// The demo server serves the web client, and a person of the scenario signs in and reads their home timeline, with
// the posts of the people they follow.

import { expect, person, test } from "./test";

test("a person of the scenario signs in and sees the home timeline", async ({ page, signIn }) => {
  await signIn(person("yuki"));
  const home = page.getByRole("feed", { name: "Home timeline" });
  await expect(home.getByRole("article", { name: "ليلى Layla" }).filter({ hasText: "صباح الخير" })).toBeVisible();
});
