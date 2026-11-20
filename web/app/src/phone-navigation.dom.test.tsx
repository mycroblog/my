// SPDX-License-Identifier: AGPL-3.0-only

// On a phone the main navigation is the tab bar: moving between its items moves the focus to the new screen's heading,
// and its last item, Menu, opens a sheet with the places the tab bar does not hold, which closes; each place opens
// its page by a press, by Enter and as a link. That the tab bar, and not the rail, is what a phone shows is the frame's
// layout, which a browser tests.

import { expect, test } from "bun:test";
import { screen, within } from "@testing-library/react";
import { renderApp, type TestApp } from "./test/app";
import { onAPhone, tabBar } from "./test/viewport";

/** The heading of the screen in place, once it is named `name`. */
const heading = (name: string | RegExp) => within(screen.getByRole("main")).findByRole("heading", { level: 1, name });

/** The tab bar's Menu, whose name says the unread direct messages it holds while there are any. */
const menuButton = () => within(tabBar()).getByRole("button", { name: /^Menu(?: \d+ unread in direct messages)?$/ });

/** Opens the Menu from the tab bar, and returns it once it is open. */
async function openMenu(app: TestApp): Promise<HTMLElement> {
  await app.user.click(menuButton());
  return screen.findByRole("menu", { name: "Menu" });
}

test("the tab bar moves the focus to the heading of the screen it opens", async () => {
  onAPhone();
  const app = await renderApp("/", { viewer: "yuki" });

  await app.user.click(within(tabBar()).getByRole("link", { name: /^Notifications(?: \d+ unread)?$/ }));
  expect(app.focused()).toBe(await heading("Notifications"));
  await app.user.click(within(tabBar()).getByRole("link", { name: "Home" }));
  expect(app.focused()).toBe(await heading("Home"));
});

test("the Menu opens with the places the tab bar does not hold and the announcements, and closes", async () => {
  onAPhone();
  const app = await renderApp("/lists", { viewer: "yuki" });
  const button = menuButton();
  const bar = within(tabBar());

  const menu = within(await openMenu(app));
  for (const place of ["Profile", "Bookmarks", "Settings", "Announcements", "Events"]) {
    expect(menu.getByRole("menuitem", { name: place })).toBeDefined();
    expect(bar.queryByRole("link", { name: place })).toBeNull();
  }

  // On a phone the Menu is a sheet, whose head ends with Close.
  await app.user.click(screen.getByRole("button", { name: "Close" }));
  expect(screen.queryByRole("menu")).toBeNull();
  expect(app.focused()).toBe(button);
});

test("a place chosen in the Menu closes it and moves the focus to the heading of its screen", async () => {
  onAPhone();
  const app = await renderApp("/lists", { viewer: "yuki" });

  await app.user.click(within(await openMenu(app)).getByRole("menuitem", { name: "Profile" }));

  expect(app.focused()).toBe(await heading(/^Yuki 雪 \(@yuki@/));
  expect(screen.queryByRole("menu")).toBeNull();
});

test("a place of the Menu opens by Enter", async () => {
  onAPhone();
  const app = await renderApp("/lists", { viewer: "yuki" });
  const profile = within(await openMenu(app)).getByRole("menuitem", { name: "Profile" });

  profile.focus();
  await app.user.keyboard("{Enter}");

  expect(app.focused()).toBe(await heading(/^Yuki 雪 \(@yuki@/));
  expect(screen.queryByRole("menu")).toBeNull();
});

test("a place of the Menu is a link to its page, which opens as a link does, and the page under it offers none", async () => {
  onAPhone();
  const app = await renderApp("/lists", { viewer: "yuki" });
  const menu = within(await openMenu(app));
  const settings = menu.getByRole("menuitem", { name: "Settings" });
  expect(settings.getAttribute("href")).toBe("/settings");
  // The page under the sheet takes no press: its own links to the same places are out of reach while it is open.
  for (const link of screen.queryAllByRole("link", { name: /^(?:Profile|Settings)$/ }))
    expect(link.closest("[inert]")).not.toBeNull();

  // A link's activation as a script or assistive technology sends it: a click with no pointer.
  settings.click();

  expect(await heading("Settings")).toBeDefined();
  expect(screen.queryByRole("menu")).toBeNull();
});
