// SPDX-License-Identifier: AGPL-3.0-only

// A visitor at the server's address meets its landing page, and goes on from it to an account: a person without one
// signs up from the account's part, and a member signs in from the page's head. The demo server is open to everyone,
// so the landing offers Create an account and keeps Sign in at its head.

import { Frame, Landing, SignIn, SignUp } from "./screens";
import { expect, person, test } from "./test";

test("a person without an account signs up from the landing page", async ({ page, fresh }) => {
  const who = fresh.newcomer();
  const landing = new Landing(page);
  await landing.open();
  await landing.signUp.click();

  const screen = new SignUp(page);
  await expect(screen.heading).toBeVisible();
  await screen.as(who);
  await expect(screen.notReady).toBeVisible();
  await expect(screen.asksToConfirm(who.email)).toBeVisible();
});

test("a member signs in from the landing page and lands on the home timeline", async ({ page }) => {
  const yuki = person("yuki");
  const landing = new Landing(page);
  await landing.open();
  await landing.signIn.click();

  const screen = new SignIn(page);
  await expect(screen.heading).toBeVisible();
  await screen.withPassword(yuki.email, yuki.password);
  await expect(new Frame(page).heading("Home")).toBeVisible();
});
