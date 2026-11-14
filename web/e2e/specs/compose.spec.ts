// SPDX-License-Identifier: AGPL-3.0-only

// A person writes a post in the composer, and it is on their home timeline and their profile, also after a reload.

import { Frame, homeTimeline, Profile } from "./screens";
import { expect, test } from "./test";

test("a person posts from the composer and finds the post on home and on their profile", async ({
  page,
  signIn,
  fresh,
}) => {
  const writer = await fresh.person();
  const text = "Composer check: the reading room opens at nine on Sunday.";
  await signIn(writer);
  const composer = await new Frame(page).newPost();
  await expect(composer.text).toBeFocused();
  await composer.write(text);
  await composer.post();
  await expect(homeTimeline(page).post(writer.displayName, text).article).toBeVisible();
  await page.reload();
  await expect(homeTimeline(page).post(writer.displayName, text).article).toBeVisible();
  const profile = new Profile(page, writer);
  await profile.open();
  await expect(profile.posts.post(writer.displayName, text).article).toBeVisible();
});
