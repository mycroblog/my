// SPDX-License-Identifier: AGPL-3.0-only

// On a phone the main navigation is the tab bar at the bottom: moving between its items moves the focus to the new
// screen's heading, and its last item, Menu, opens a sheet with the places the tab bar does not hold, which closes.

import { Frame } from "./screens";
import { expect, person, test } from "./test";

// The narrowest screen the client supports (ADR-0110 §10).
test.use({ viewport: { width: 320, height: 640 } });

test("the tab bar moves the focus to the heading of the screen it opens", async ({ page, signIn }) => {
  await signIn(person("yuki"));
  const frame = new Frame(page);
  await frame.goTo("Notifications");
  await expect(frame.heading("Notifications")).toBeFocused();
  await frame.goTo("Home");
  await expect(frame.heading("Home")).toBeFocused();
});

test("the Menu opens with the places the tab bar does not hold and the announcements, and closes", async ({
  page,
  signIn,
}) => {
  await signIn(person("yuki"));
  const frame = new Frame(page);
  const menu = await frame.openMenu();
  for (const place of ["Profile", "Bookmarks", "Settings", "Announcements", "Events"] as const)
    await expect(menu.link(place)).toBeVisible();
  await menu.close();
  await expect(menu.dialog).toBeHidden();
  await expect(frame.menuButton).toBeFocused();
});

test("a place chosen in the Menu closes it and moves the focus to the heading of its screen", async ({
  page,
  signIn,
}) => {
  const yuki = person("yuki");
  await signIn(yuki);
  const frame = new Frame(page);
  const menu = await frame.openMenu();
  await menu.goTo("Profile");
  await expect(menu.dialog).toBeHidden();
  await expect(frame.heading(new RegExp(`^${yuki.displayName} \\(@yuki@`))).toBeFocused();
});
