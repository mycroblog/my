// SPDX-License-Identifier: AGPL-3.0-only

// Moving to another screen moves the focus to the new screen's heading, so a screen reader announces where the person
// is and the keyboard starts from the new screen.

import { Frame, homeTimeline, Thread } from "./screens";
import { expect, person, test } from "./test";

test("the main navigation moves the focus to the heading of the screen it opens", async ({ page, signIn }) => {
  await signIn(person("marco"));
  const frame = new Frame(page);
  await frame.goTo("Notifications");
  await expect(frame.heading("Notifications")).toBeFocused();
  await frame.goTo("Profile");
  await expect(frame.heading(/^Marco \(@marco@/)).toBeFocused();
  await frame.goTo("Home");
  await expect(frame.heading("Home")).toBeFocused();
});

test("opening a post and going back move the focus to each screen's heading", async ({ page, signIn }) => {
  await signIn(person("marco"));
  await homeTimeline(page).post(person("sam").displayName, "Day 3 of the sourdough").open();
  const thread = new Thread(page);
  await expect(thread.heading).toBeFocused();
  await thread.back();
  await expect(new Frame(page).heading("Home")).toBeFocused();
});
