// SPDX-License-Identifier: AGPL-3.0-only

// The composer's audience is a menu with a submenu: on a viewport narrower than a tablet's the app's menus are sheets,
// headed by their name, and from a tablet's width popovers, headed inside a submenu alone. What a sheet, a popover and
// a submenu do by themselves is the design system's (menu.dom.test.tsx, menu.jsdom.test.tsx): these tests hold which
// of them the app shows, what the audience's menus hold, and that closing one leaves the composer open.

import { expect, test } from "bun:test";
import { screen, within } from "@testing-library/react";
import { renderApp, type TestApp } from "./test/app";
import { newPost } from "./test/composer";
import { onAPhone } from "./test/viewport";

/** Where the composer is opened: it opens the same over any screen, and a screen that shows little keeps a test quick. */
const light = "/lists";

/** The headings over the page that are named `name`: the head of an open menu is one. */
const headings = (name: string) => screen.queryAllByRole("heading", { name });

/** Opens the audience's menu of a new post's composer, and returns the composer's dialog, its button and the menu. */
async function openAudience(app: TestApp) {
  const { dialog } = await newPost(app);
  const button = within(dialog).getByRole("button", { name: /^Who can see it: .+$/ });
  await app.user.click(button);
  return { dialog, button, top: await screen.findByRole("menu", { name: "Who can see and reply" }) };
}

/** Enters the submenu of who can see the post from `top`, and returns it. */
async function openVisibility(app: TestApp, top: HTMLElement): Promise<HTMLElement> {
  await app.user.click(within(top).getByRole("menuitem", { name: /^Who can see it\s*\S.*$/ }));
  return screen.findByRole("menu", { name: "Who can see it" });
}

test("on a phone the composer's audience opens a submenu in the same sheet, under Back and its title; Back returns, Escape closes the menu alone", async () => {
  onAPhone();
  const app = await renderApp(light, { viewer: "yuki" });
  const { dialog, button, top } = await openAudience(app);
  // A sheet is headed by its menu's name.
  expect(headings("Who can see and reply")).toHaveLength(1);

  const sub = await openVisibility(app, top);
  // Each audience is a value of one setting: a radio, the chosen one checked.
  expect(within(sub).getAllByRole("menuitemradio", { checked: true })).toHaveLength(1);
  expect(within(sub).queryAllByRole("menuitem")).toHaveLength(0);
  expect(screen.queryByRole("menu", { name: "Who can see and reply" })).toBeNull();
  expect(headings("Who can see it")).toHaveLength(1);

  await app.user.click(screen.getByRole("button", { name: "Back" }));
  expect(await screen.findByRole("menu", { name: "Who can see and reply" })).toBeDefined();
  expect(screen.queryByRole("menu", { name: "Who can see it" })).toBeNull();
  expect(headings("Who can see and reply")).toHaveLength(1);

  // Escape closes the menu alone, not the composer under it.
  await app.user.keyboard("{Escape}");
  expect(screen.queryByRole("menu")).toBeNull();
  expect(screen.getByRole("dialog", { name: "New post" })).toBe(dialog);
  expect(app.focused()).toBe(button);
});

test("from a tablet's width the composer's audience opens its submenu in the popover, headed by Back and its title", async () => {
  const app = await renderApp(light, { viewer: "yuki" });
  const { top } = await openAudience(app);
  // A popover has no head of its own.
  expect(headings("Who can see and reply")).toHaveLength(0);

  const sub = await openVisibility(app, top);
  expect(within(sub).getAllByRole("menuitemradio", { checked: true })).toHaveLength(1);
  expect(headings("Who can see it")).toHaveLength(1);

  await app.user.click(screen.getByRole("button", { name: "Back" }));
  expect(await screen.findByRole("menu", { name: "Who can see and reply" })).toBeDefined();
  expect(headings("Who can see and reply")).toHaveLength(0);
});
