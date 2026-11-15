// SPDX-License-Identifier: AGPL-3.0-only

// A person writes a post in the composer, and it is on their home timeline and their profile, also after a reload. While
// it is sent, the composer is not left.

import type { Route } from "@playwright/test";
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

test("while a post is sent, Close and Escape leave the composer open, and it closes once posted", async ({
  page,
  signIn,
  fresh,
}) => {
  const writer = await fresh.person();
  const text = "In-flight check: the ferry leaves at noon.";
  await signIn(writer);
  const composer = await new Frame(page).newPost();
  await composer.write(text);
  const held = Promise.withResolvers<Route>();
  await page.route("**/api/v1/statuses", (route) => {
    if (route.request().method() === "POST") held.resolve(route);
    else void route.fallback();
  });
  await composer.postButton.click();
  const route = await held.promise;
  const close = composer.dialog.getByRole("button", { name: "Close", exact: true });
  // Close stays in its place, disabled, so that it cannot be pressed; Post alone shows it is sending.
  await expect(close).toBeVisible();
  await expect(close).toBeDisabled();
  await expect(composer.postButton).toHaveAttribute("aria-busy", "true");
  await page.keyboard.press("Escape");
  await expect(composer.dialog).toBeVisible();
  await composer.holds(text);
  await route.continue();
  await expect(composer.dialog).toBeHidden();
  await expect(homeTimeline(page).post(writer.displayName, text).article).toHaveCount(1);
  // Nothing was discarded: no Undo is offered.
  await expect(page.getByRole("button", { name: "Undo", exact: true })).toBeHidden();
});
