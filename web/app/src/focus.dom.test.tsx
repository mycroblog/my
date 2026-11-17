// SPDX-License-Identifier: AGPL-3.0-only

// Moving to another screen moves the focus to the new screen's heading, so that a screen reader announces where the
// reader is and the keyboard starts from the new screen.

import { expect, test } from "bun:test";
import { screen, within } from "@testing-library/react";
import { renderApp, type TestApp } from "./test/app";
import { feedOf, postOf } from "./test/posts";
import { rail } from "./test/viewport";

/** The heading of the screen in place, once it is named `name`. */
const heading = (name: string | RegExp) => within(screen.getByRole("main")).findByRole("heading", { level: 1, name });

/** Follows the rail's link to the place named `place`. */
async function goTo(app: TestApp, place: string | RegExp): Promise<void> {
  await app.user.click(within(rail()).getByRole("link", { name: place }));
}

test("the main navigation moves the focus to the heading of the screen it opens", async () => {
  const app = await renderApp("/", { viewer: "marco" });

  await goTo(app, /^Notifications(?: \d+ unread)?$/);
  expect(app.focused()).toBe(await heading("Notifications"));
  expect(app.path()).toBe("/notifications");

  await goTo(app, "Profile");
  expect(app.focused()).toBe(await heading(/^Marco \(@marco@/));

  await goTo(app, "Home");
  expect(app.focused()).toBe(await heading("Home"));
  expect(app.path()).toBe("/");
});

test("opening a post and going back move the focus to each screen's heading", async () => {
  const app = await renderApp("/", { viewer: "marco" });
  const post = postOf(await feedOf("Home timeline"), "Sam Rivera", "Day 3 of the sourdough");

  // The post's time is the link to its own page.
  await app.user.click(within(within(post).getByRole("time").closest("a") ?? post).getByRole("time"));
  expect(app.focused()).toBe(await heading("Post"));
  expect(app.path()).toMatch(/^\/@sam\/status\/[^/]+$/);

  await app.user.click(within(screen.getByRole("main")).getByRole("button", { name: "Back" }));
  expect(app.focused()).toBe(await heading("Home"));
  expect(app.path()).toBe("/");
});
